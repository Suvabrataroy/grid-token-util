package postgres

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/grid-computing/control-plane/internal/brownie"
	"github.com/grid-computing/control-plane/internal/domain"
)

// WriteLedgerEntry inserts a new brownie ledger row.
func (s *Store) WriteLedgerEntry(ctx context.Context, ev *domain.BrownieEvent) error {
	const q = `
		INSERT INTO brownie_points_ledger (id, worker_id, org_unit_id, points, reason, reference_id, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, NOW())
	`
	_, err := s.pool.Exec(ctx, q,
		ev.ID, ev.WorkerID, ev.OrgUnitID, ev.Points, ev.Reason, ev.ReferenceID,
	)
	if err != nil {
		return fmt.Errorf("brownie: write ledger: %w", err)
	}
	return nil
}

// GetBalance returns the summed brownie points for a worker.
func (s *Store) GetBalance(ctx context.Context, workerID uuid.UUID) (int, error) {
	const q = `
		SELECT COALESCE(SUM(points), 0)
		FROM brownie_points_ledger
		WHERE worker_id = $1
	`
	var total int
	if err := s.pool.QueryRow(ctx, q, workerID).Scan(&total); err != nil {
		return 0, fmt.Errorf("brownie: get balance: %w", err)
	}
	return total, nil
}

// GetLeaderboard returns the top-N workers for an org unit from the
// materialised view.
func (s *Store) GetLeaderboard(ctx context.Context, orgUnitID uuid.UUID, limit int) ([]brownie.LeaderboardEntry, error) {
	const q = `
		SELECT worker_id, org_unit_id, total_points, event_count
		FROM brownie_leaderboard
		WHERE org_unit_id = $1
		ORDER BY total_points DESC
		LIMIT $2
	`
	rows, err := s.pool.Query(ctx, q, orgUnitID, limit)
	if err != nil {
		return nil, fmt.Errorf("brownie: leaderboard query: %w", err)
	}
	defer rows.Close()

	var entries []brownie.LeaderboardEntry
	for rows.Next() {
		var e brownie.LeaderboardEntry
		if err := rows.Scan(&e.WorkerID, &e.OrgUnitID, &e.TotalPoints, &e.EventCount); err != nil {
			return nil, fmt.Errorf("brownie: leaderboard scan: %w", err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("brownie: leaderboard rows: %w", err)
	}
	return entries, nil
}
