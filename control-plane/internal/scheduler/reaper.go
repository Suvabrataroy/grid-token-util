package scheduler

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/domain"
)

// ReaperTaskStore is the Postgres interface used by the Reaper.
type ReaperTaskStore interface {
	GetStaleTasks(ctx context.Context, heartbeatDeadline time.Time) ([]*domain.Task, error)
	UpdateTaskState(ctx context.Context, id uuid.UUID, state domain.TaskState, workerID *uuid.UUID) error
}

// ReaperWorkerStore is the worker-state interface used by the Reaper.
type ReaperWorkerStore interface {
	UpdateWorkerState(ctx context.Context, workerID uuid.UUID, state domain.WorkerState) error
}

// ReaperHeartbeatStore checks worker liveness in Redis.
type ReaperHeartbeatStore interface {
	GetWorkerHeartbeat(ctx context.Context, workerID uuid.UUID) (bool, error)
}

// BrownieDeductor awards/deducts brownie points.
type BrownieDeductor interface {
	Deduct(ctx context.Context, workerID uuid.UUID, orgUnitID uuid.UUID, points int, reason string, refID *uuid.UUID) error
}

// Reaper finds tasks whose workers have gone silent and recycles them.
type Reaper struct {
	tasks      ReaperTaskStore
	workers    ReaperWorkerStore
	heartbeats ReaperHeartbeatStore
	brownie    BrownieDeductor

	heartbeatTTLSec  int
	reaperIntervalSec int

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewReaper constructs a Reaper.
func NewReaper(
	tasks ReaperTaskStore,
	workers ReaperWorkerStore,
	heartbeats ReaperHeartbeatStore,
	brownie BrownieDeductor,
	heartbeatTTLSec, reaperIntervalSec int,
) *Reaper {
	return &Reaper{
		tasks:             tasks,
		workers:           workers,
		heartbeats:        heartbeats,
		brownie:           brownie,
		heartbeatTTLSec:   heartbeatTTLSec,
		reaperIntervalSec: reaperIntervalSec,
	}
}

// Start launches the reaper loop in the background.
func (r *Reaper) Start(ctx context.Context) {
	ctx, r.cancel = context.WithCancel(ctx)
	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		interval := time.Duration(r.reaperIntervalSec) * time.Second
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				r.reap(ctx)
			}
		}
	}()
	log.Info().Int("interval_sec", r.reaperIntervalSec).Msg("reaper: started")
}

// Stop signals the reaper goroutine to exit and waits for it.
func (r *Reaper) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
	log.Info().Msg("reaper: stopped")
}

// reap performs one reaping cycle.
func (r *Reaper) reap(ctx context.Context) {
	deadline := time.Now().UTC().Add(-time.Duration(r.heartbeatTTLSec) * time.Second)

	staleTasks, err := r.tasks.GetStaleTasks(ctx, deadline)
	if err != nil {
		log.Error().Err(err).Msg("reaper: get stale tasks")
		return
	}

	if len(staleTasks) == 0 {
		return
	}

	log.Info().Int("count", len(staleTasks)).Msg("reaper: found stale tasks")

	for _, task := range staleTasks {
		t := task // capture

		// Double-check: is the heartbeat genuinely absent in Redis?
		if t.AssignedWorkerID != nil {
			alive, err := r.heartbeats.GetWorkerHeartbeat(ctx, *t.AssignedWorkerID)
			if err != nil {
				// Redis is unavailable; be conservative and skip this task to
				// avoid falsely abandoning a worker that may still be running.
				log.Warn().Err(err).
					Str("worker_id", t.AssignedWorkerID.String()).
					Msg("reaper: heartbeat check failed, skipping task to avoid false abandonment")
				continue
			}
			if alive {
				continue // worker came back, skip
			}
		}

		// Requeue the task.
		if err := r.tasks.UpdateTaskState(ctx, t.ID, domain.TaskStateQueued, nil); err != nil {
			log.Error().Err(err).
				Str("task_id", t.ID.String()).
				Msg("reaper: requeue task")
			continue
		}

		// Mark worker as offline.
		if t.AssignedWorkerID != nil {
			if err := r.workers.UpdateWorkerState(ctx, *t.AssignedWorkerID, domain.WorkerStateOffline); err != nil {
				log.Warn().Err(err).
					Str("worker_id", t.AssignedWorkerID.String()).
					Msg("reaper: mark worker offline")
			}

			// Deduct brownie points for abandonment.
			if r.brownie != nil && t.AssignedWorkerID != nil {
				taskID := t.ID
				if err := r.brownie.Deduct(ctx, *t.AssignedWorkerID, t.OrgUnitID, 5, "task_abandoned", &taskID); err != nil {
					log.Warn().Err(err).
						Str("worker_id", t.AssignedWorkerID.String()).
						Msg("reaper: deduct brownie points")
				}
			}
		}

		log.Info().
			Str("task_id", t.ID.String()).
			Msg("reaper: task requeued")
	}
}
