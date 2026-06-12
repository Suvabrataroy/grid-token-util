// Package brownie implements the Brownie Points incentive system for workers.
package brownie

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/grid-computing/control-plane/internal/domain"
)

// Point values for standard events.
const (
	PointsTaskCompleted  = 10
	PointsTaskAbandoned  = -5
	PointsOutputApproved = 20
	PointsOutputRejected = -10
	PointsSecretFound    = -50
)

// LedgerWriter is the persistence interface for brownie events.
type LedgerWriter interface {
	WriteLedgerEntry(ctx context.Context, ev *domain.BrownieEvent) error
}

// Engine applies the brownie point rules and persists the results.
type Engine struct {
	ledger LedgerWriter
}

// NewEngine creates a brownie Engine backed by the given ledger writer.
func NewEngine(ledger LedgerWriter) *Engine {
	return &Engine{ledger: ledger}
}

// Award adds positive brownie points to a worker's ledger.
func (e *Engine) Award(
	ctx context.Context,
	workerID uuid.UUID,
	orgUnitID uuid.UUID,
	points int,
	reason string,
	refID *uuid.UUID,
) error {
	if points <= 0 {
		return fmt.Errorf("brownie: Award called with non-positive points %d; use Deduct for negative values", points)
	}
	return e.write(ctx, workerID, orgUnitID, points, reason, refID)
}

// Deduct subtracts brownie points from a worker's ledger (stores a negative
// entry).
func (e *Engine) Deduct(
	ctx context.Context,
	workerID uuid.UUID,
	orgUnitID uuid.UUID,
	points int,
	reason string,
	refID *uuid.UUID,
) error {
	if points <= 0 {
		return fmt.Errorf("brownie: Deduct called with non-positive points %d", points)
	}
	return e.write(ctx, workerID, orgUnitID, -points, reason, refID)
}

func (e *Engine) write(
	ctx context.Context,
	workerID uuid.UUID,
	orgUnitID uuid.UUID,
	points int,
	reason string,
	refID *uuid.UUID,
) error {
	ev := &domain.BrownieEvent{
		ID:          uuid.New(),
		WorkerID:    workerID,
		OrgUnitID:   orgUnitID,
		Points:      points,
		Reason:      reason,
		ReferenceID: refID,
	}

	if err := e.ledger.WriteLedgerEntry(ctx, ev); err != nil {
		return fmt.Errorf("brownie: write ledger: %w", err)
	}

	log.Info().
		Str("worker_id", workerID.String()).
		Int("points", points).
		Str("reason", reason).
		Msg("brownie: points recorded")

	return nil
}
