package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grid-computing/control-plane/internal/domain"
)

// RegisterWorker upserts a worker identified by (org_unit_id, hostname_hash).
// If the worker already exists its agents list and state are refreshed.
func (s *Store) RegisterWorker(ctx context.Context, w *domain.Worker) error {
	const q = `
		INSERT INTO workers (id, org_unit_id, hostname_hash, agents, state, capacity_score, last_heartbeat_at, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (org_unit_id, hostname_hash) DO UPDATE
		    SET agents            = EXCLUDED.agents,
		        state             = EXCLUDED.state,
		        capacity_score    = EXCLUDED.capacity_score,
		        last_heartbeat_at = EXCLUDED.last_heartbeat_at
		RETURNING id
	`
	return s.pool.QueryRow(ctx, q,
		w.ID,
		w.OrgUnitID,
		w.HostnameHash,
		w.Agents,
		w.State,
		w.CapacityScore,
		w.LastHeartbeatAt,
		w.CreatedAt,
	).Scan(&w.ID)
}

// UpdateWorkerState changes the operational state of a worker.
func (s *Store) UpdateWorkerState(ctx context.Context, workerID uuid.UUID, state domain.WorkerState) error {
	const q = `UPDATE workers SET state = $1 WHERE id = $2`
	tag, err := s.pool.Exec(ctx, q, state, workerID)
	if err != nil {
		return fmt.Errorf("workers: update state: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workers: not found: %s", workerID)
	}
	return nil
}

// UpdateWorkerHeartbeat records a fresh heartbeat timestamp and refreshes the
// agent capability list for the given worker.
func (s *Store) UpdateWorkerHeartbeat(ctx context.Context, workerID uuid.UUID, agents []string) error {
	const q = `
		UPDATE workers
		SET last_heartbeat_at = NOW(),
		    agents            = $1
		WHERE id = $2
	`
	tag, err := s.pool.Exec(ctx, q, agents, workerID)
	if err != nil {
		return fmt.Errorf("workers: update heartbeat: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("workers: not found: %s", workerID)
	}
	return nil
}

// GetWorker returns a worker by primary key.
func (s *Store) GetWorker(ctx context.Context, id uuid.UUID) (*domain.Worker, error) {
	const q = `
		SELECT id, org_unit_id, hostname_hash, agents, state, capacity_score, last_heartbeat_at, created_at
		FROM workers
		WHERE id = $1
	`
	row := s.pool.QueryRow(ctx, q, id)
	w, err := scanWorker(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("workers: not found: %s", id)
		}
		return nil, fmt.Errorf("workers: get: %w", err)
	}
	return w, nil
}

// ListWorkers returns all workers belonging to an org unit.
func (s *Store) ListWorkers(ctx context.Context, orgUnitID uuid.UUID) ([]*domain.Worker, error) {
	const q = `
		SELECT id, org_unit_id, hostname_hash, agents, state, capacity_score, last_heartbeat_at, created_at
		FROM workers
		WHERE org_unit_id = $1
		ORDER BY created_at ASC
	`
	return queryWorkers(ctx, s, q, orgUnitID)
}

// ListIdleWorkers returns all workers across all org units that are in the idle
// state and sent a heartbeat within the last 90 seconds.
func (s *Store) ListIdleWorkers(ctx context.Context) ([]*domain.Worker, error) {
	const q = `
		SELECT id, org_unit_id, hostname_hash, agents, state, capacity_score, last_heartbeat_at, created_at
		FROM workers
		WHERE state = 'idle'
		  AND last_heartbeat_at > NOW() - INTERVAL '90 seconds'
		ORDER BY capacity_score DESC
	`
	return queryWorkers(ctx, s, q)
}

// queryWorkers is a helper that executes a worker SELECT query with optional args.
func queryWorkers(ctx context.Context, s *Store, q string, args ...any) ([]*domain.Worker, error) {
	rows, err := s.pool.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("workers: query: %w", err)
	}
	defer rows.Close()

	var workers []*domain.Worker
	for rows.Next() {
		w, err := scanWorker(rows)
		if err != nil {
			return nil, fmt.Errorf("workers: scan: %w", err)
		}
		workers = append(workers, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workers: rows: %w", err)
	}
	return workers, nil
}

func scanWorker(row pgx.Row) (*domain.Worker, error) {
	var w domain.Worker
	var lastHeartbeat *time.Time

	if err := row.Scan(
		&w.ID,
		&w.OrgUnitID,
		&w.HostnameHash,
		&w.Agents,
		&w.State,
		&w.CapacityScore,
		&lastHeartbeat,
		&w.CreatedAt,
	); err != nil {
		return nil, err
	}
	w.LastHeartbeatAt = lastHeartbeat
	return &w, nil
}
