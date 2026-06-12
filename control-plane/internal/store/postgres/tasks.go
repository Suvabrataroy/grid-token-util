package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grid-computing/control-plane/internal/domain"
)

// CreateTask inserts a new task record.
func (s *Store) CreateTask(ctx context.Context, t *domain.Task) error {
	optJSON, err := json.Marshal(t.Options)
	if err != nil {
		return fmt.Errorf("tasks: marshal options: %w", err)
	}

	const q = `
		INSERT INTO tasks (
			id, org_unit_id, submitter_id, title, description,
			task_type, priority, ai_agent, state,
			assigned_worker_id, queued_at, assigned_at, started_at,
			completed_at, failed_at, error_message,
			retry_count, max_retries, options
		) VALUES (
			$1,$2,$3,$4,$5,
			$6,$7,$8,$9,
			$10,$11,$12,$13,
			$14,$15,$16,
			$17,$18,$19
		)
	`
	_, err = s.pool.Exec(ctx, q,
		t.ID, t.OrgUnitID, t.SubmitterID, t.Title, t.Description,
		t.TaskType, t.Priority, t.AIAgent, t.State,
		t.AssignedWorkerID, t.QueuedAt, t.AssignedAt, t.StartedAt,
		t.CompletedAt, t.FailedAt, t.ErrorMessage,
		t.RetryCount, t.MaxRetries, optJSON,
	)
	if err != nil {
		return fmt.Errorf("tasks: create: %w", err)
	}
	return nil
}

// GetTask retrieves a task by primary key.
func (s *Store) GetTask(ctx context.Context, id uuid.UUID) (*domain.Task, error) {
	const q = `
		SELECT id, org_unit_id, submitter_id, title, description,
		       task_type, priority, ai_agent, state,
		       assigned_worker_id, queued_at, assigned_at, started_at,
		       completed_at, failed_at, error_message,
		       retry_count, max_retries, options
		FROM tasks
		WHERE id = $1
		LIMIT 1
	`
	row := s.pool.QueryRow(ctx, q, id)
	task, err := scanTask(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("tasks: not found: %s", id)
		}
		return nil, fmt.Errorf("tasks: get: %w", err)
	}
	return task, nil
}

// UpdateTaskState changes a task's lifecycle state and optionally sets the
// assigned worker.  The appropriate timestamp column is also updated based on
// the new state.
func (s *Store) UpdateTaskState(ctx context.Context, id uuid.UUID, state domain.TaskState, workerID *uuid.UUID) error {
	const q = `
		UPDATE tasks
		SET state              = $1,
		    assigned_worker_id = COALESCE($2, assigned_worker_id),
		    assigned_at  = CASE WHEN $1 = 'assigned'  THEN NOW() ELSE assigned_at  END,
		    started_at   = CASE WHEN $1 = 'running'   THEN NOW() ELSE started_at   END,
		    completed_at = CASE WHEN $1 = 'completed' THEN NOW() ELSE completed_at END,
		    failed_at    = CASE WHEN $1 = 'failed'    THEN NOW() ELSE failed_at    END
		WHERE id = $3
	`
	tag, err := s.pool.Exec(ctx, q, state, workerID, id)
	if err != nil {
		return fmt.Errorf("tasks: update state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("tasks: not found: %s", id)
	}
	return nil
}

// ListTasksByOrg returns paginated tasks for an org unit ordered by queued_at DESC.
func (s *Store) ListTasksByOrg(ctx context.Context, orgUnitID uuid.UUID, limit, offset int) ([]*domain.Task, error) {
	const q = `
		SELECT id, org_unit_id, submitter_id, title, description,
		       task_type, priority, ai_agent, state,
		       assigned_worker_id, queued_at, assigned_at, started_at,
		       completed_at, failed_at, error_message,
		       retry_count, max_retries, options
		FROM tasks
		WHERE org_unit_id = $1
		ORDER BY queued_at DESC
		LIMIT $2 OFFSET $3
	`
	rows, err := s.pool.Query(ctx, q, orgUnitID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("tasks: list query: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("tasks: list scan: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: list rows: %w", err)
	}
	return tasks, nil
}

// ClaimNextTask atomically finds the highest-priority queued task that the
// worker can handle (matching agent capability), claims it using
// pg_try_advisory_xact_lock to prevent double-assignment under concurrent
// schedulers, and returns it.  Must be called within an existing transaction.
func (s *Store) ClaimNextTask(ctx context.Context, tx pgx.Tx, workerID uuid.UUID, agents []string) (*domain.Task, error) {
	const q = `
		SELECT id, org_unit_id, submitter_id, title, description,
		       task_type, priority, ai_agent, state,
		       assigned_worker_id, queued_at, assigned_at, started_at,
		       completed_at, failed_at, error_message,
		       retry_count, max_retries, options
		FROM tasks
		WHERE state    = 'queued'
		  AND ai_agent = ANY($1)
		ORDER BY priority DESC, queued_at ASC
		LIMIT 1
		FOR UPDATE SKIP LOCKED
	`
	row := tx.QueryRow(ctx, q, agents)
	task, err := scanTask(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // no work available
		}
		return nil, fmt.Errorf("tasks: claim query: %w", err)
	}

	// Obtain a lightweight advisory lock scoped to this transaction.
	// The lock key is derived from the lower 63 bits of the task UUID.
	lockKey := int64(task.ID[0])<<56 | int64(task.ID[1])<<48 | int64(task.ID[2])<<40 |
		int64(task.ID[3])<<32 | int64(task.ID[4])<<24 | int64(task.ID[5])<<16 |
		int64(task.ID[6])<<8 | int64(task.ID[7])

	var locked bool
	if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock($1)`, lockKey).Scan(&locked); err != nil {
		return nil, fmt.Errorf("tasks: advisory lock: %w", err)
	}
	if !locked {
		return nil, nil // another scheduler beat us to it
	}

	const update = `
		UPDATE tasks
		SET state              = 'assigned',
		    assigned_worker_id = $1,
		    assigned_at        = NOW()
		WHERE id = $2
	`
	if _, err := tx.Exec(ctx, update, workerID, task.ID); err != nil {
		return nil, fmt.Errorf("tasks: claim update: %w", err)
	}

	task.State = domain.TaskStateAssigned
	task.AssignedWorkerID = &workerID
	now := time.Now().UTC()
	task.AssignedAt = &now

	return task, nil
}

// GetAssignedTaskForWorker returns the first task in "assigned" state for the
// given worker, or nil if none is pending.  Used by the heartbeat handler to
// push the assignment back to the worker.
func (s *Store) GetAssignedTaskForWorker(ctx context.Context, workerID uuid.UUID) (*domain.Task, error) {
	const q = `
		SELECT id, org_unit_id, submitter_id, title, description,
		       task_type, priority, ai_agent, state,
		       assigned_worker_id, queued_at, assigned_at, started_at,
		       completed_at, failed_at, error_message,
		       retry_count, max_retries, options
		FROM tasks
		WHERE assigned_worker_id = $1
		  AND state = 'assigned'
		ORDER BY assigned_at ASC
		LIMIT 1
	`
	row := s.pool.QueryRow(ctx, q, workerID)
	task, err := scanTask(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("tasks: get assigned for worker: %w", err)
	}
	return task, nil
}

// GetStaleTasks returns tasks in an assigned or running state whose worker
// has not sent a heartbeat since heartbeatDeadline.
func (s *Store) GetStaleTasks(ctx context.Context, heartbeatDeadline time.Time) ([]*domain.Task, error) {
	const q = `
		SELECT t.id, t.org_unit_id, t.submitter_id, t.title, t.description,
		       t.task_type, t.priority, t.ai_agent, t.state,
		       t.assigned_worker_id, t.queued_at, t.assigned_at, t.started_at,
		       t.completed_at, t.failed_at, t.error_message,
		       t.retry_count, t.max_retries, t.options
		FROM tasks t
		JOIN workers w ON w.id = t.assigned_worker_id
		WHERE t.state IN ('assigned', 'running')
		  AND (w.last_heartbeat_at IS NULL OR w.last_heartbeat_at < $1)
	`
	rows, err := s.pool.Query(ctx, q, heartbeatDeadline)
	if err != nil {
		return nil, fmt.Errorf("tasks: stale query: %w", err)
	}
	defer rows.Close()

	var tasks []*domain.Task
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return nil, fmt.Errorf("tasks: stale scan: %w", err)
		}
		tasks = append(tasks, task)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("tasks: stale rows: %w", err)
	}
	return tasks, nil
}

func scanTask(row pgx.Row) (*domain.Task, error) {
	var t domain.Task
	var optRaw []byte

	if err := row.Scan(
		&t.ID, &t.OrgUnitID, &t.SubmitterID, &t.Title, &t.Description,
		&t.TaskType, &t.Priority, &t.AIAgent, &t.State,
		&t.AssignedWorkerID, &t.QueuedAt, &t.AssignedAt, &t.StartedAt,
		&t.CompletedAt, &t.FailedAt, &t.ErrorMessage,
		&t.RetryCount, &t.MaxRetries, &optRaw,
	); err != nil {
		return nil, err
	}

	if len(optRaw) > 0 {
		if err := json.Unmarshal(optRaw, &t.Options); err != nil {
			return nil, fmt.Errorf("unmarshal options: %w", err)
		}
	}
	return &t, nil
}
