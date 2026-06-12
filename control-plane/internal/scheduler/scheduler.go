// Package scheduler provides the task dispatch and worker reaper goroutines.
package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/domain"
)

// SchedulerTaskStore is the Postgres task interface used by the scheduler.
type SchedulerTaskStore interface {
	ClaimNextTask(ctx context.Context, tx pgx.Tx, workerID uuid.UUID, agents []string) (*domain.Task, error)
	UpdateTaskState(ctx context.Context, id uuid.UUID, state domain.TaskState, workerID *uuid.UUID) error
}

// SchedulerWorkerStore is the Postgres worker interface used by the scheduler.
type SchedulerWorkerStore interface {
	ListIdleWorkers(ctx context.Context) ([]*domain.Worker, error)
	UpdateWorkerState(ctx context.Context, workerID uuid.UUID, state domain.WorkerState) error
}

// TxBeginner can start a database transaction.
type TxBeginner interface {
	WithTx(ctx context.Context, fn func(pgx.Tx) error) error
}

// QueueDepth gives the scheduler visibility into queue depth.
type QueueDepth interface {
	QueueLength(ctx context.Context) (int64, error)
}

// Scheduler dispatches queued tasks to idle workers on a regular tick.
type Scheduler struct {
	tasks       SchedulerTaskStore
	workers     SchedulerWorkerStore
	txBeginner  TxBeginner
	queueLen    QueueDepth
	tickInterval time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewScheduler constructs a Scheduler.
func NewScheduler(
	tasks SchedulerTaskStore,
	workers SchedulerWorkerStore,
	txBeginner TxBeginner,
	queueLen QueueDepth,
	tickIntervalSec int,
) *Scheduler {
	return &Scheduler{
		tasks:        tasks,
		workers:      workers,
		txBeginner:   txBeginner,
		queueLen:     queueLen,
		tickInterval: time.Duration(tickIntervalSec) * time.Second,
	}
}

// Start launches the scheduler loop in the background.
func (s *Scheduler) Start(ctx context.Context) {
	ctx, s.cancel = context.WithCancel(ctx)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		ticker := time.NewTicker(s.tickInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.tick(ctx)
			}
		}
	}()
	log.Info().Dur("interval", s.tickInterval).Msg("scheduler: started")
}

// Stop signals the scheduler goroutine to exit and waits for it.
func (s *Scheduler) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	s.wg.Wait()
	log.Info().Msg("scheduler: stopped")
}

// tick performs one scheduling round: match idle workers to queued tasks.
func (s *Scheduler) tick(ctx context.Context) {
	// Quick bail-out if the queue is empty.
	qLen, err := s.queueLen.QueueLength(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("scheduler: queue length")
		return
	}
	if qLen == 0 {
		return
	}

	workers, err := s.workers.ListIdleWorkers(ctx)
	if err != nil {
		log.Error().Err(err).Msg("scheduler: list idle workers")
		return
	}
	if len(workers) == 0 {
		return
	}

	log.Debug().
		Int64("queue_depth", qLen).
		Int("idle_workers", len(workers)).
		Msg("scheduler: tick")

	for _, w := range workers {
		if qLen <= 0 {
			break
		}

		worker := w // capture for closure
		var claimed *domain.Task

		err := s.txBeginner.WithTx(ctx, func(tx pgx.Tx) error {
			var err error
			claimed, err = s.tasks.ClaimNextTask(ctx, tx, worker.ID, worker.Agents)
			return err
		})
		if err != nil {
			log.Error().Err(err).
				Str("worker_id", worker.ID.String()).
				Msg("scheduler: claim task")
			continue
		}
		if claimed == nil {
			// No task matched this worker's capabilities.
			continue
		}

		// Mark worker as busy.
		if err := s.workers.UpdateWorkerState(ctx, worker.ID, domain.WorkerStateBusy); err != nil {
			log.Warn().Err(err).
				Str("worker_id", worker.ID.String()).
				Msg("scheduler: mark worker busy")
		}

		qLen--

		log.Info().
			Str("task_id", claimed.ID.String()).
			Str("worker_id", worker.ID.String()).
			Str("agent", claimed.AIAgent).
			Msg("scheduler: task assigned")
	}
}
