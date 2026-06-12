// Package brownie implements the Brownie Points incentive system for workers.
// This file defines the leaderboard types and the LedgerStore interface.
package brownie

import (
	"context"

	"github.com/google/uuid"
)

// LeaderboardEntry represents one row in the brownie leaderboard.
type LeaderboardEntry struct {
	WorkerID    uuid.UUID `json:"worker_id"`
	OrgUnitID   uuid.UUID `json:"org_unit_id"`
	TotalPoints int       `json:"total_points"`
	EventCount  int       `json:"event_count"`
}

// LedgerStore defines the full persistence interface for the brownie sub-system.
// The Postgres store implements this interface.
type LedgerStore interface {
	LedgerWriter
	GetBalance(ctx context.Context, workerID uuid.UUID) (int, error)
	GetLeaderboard(ctx context.Context, orgUnitID uuid.UUID, limit int) ([]LeaderboardEntry, error)
}
