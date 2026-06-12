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

// CreateOutput inserts a new output package submitted by a worker.
func (s *Store) CreateOutput(ctx context.Context, o *domain.OutputPackage) error {
	artifactsJSON, err := json.Marshal(o.Artifacts)
	if err != nil {
		return fmt.Errorf("outputs: marshal artifacts: %w", err)
	}
	metaJSON, err := json.Marshal(o.Metadata)
	if err != nil {
		return fmt.Errorf("outputs: marshal metadata: %w", err)
	}

	const q = `
		INSERT INTO outputs (
			id, task_id, worker_id, hmac_sha256,
			artifacts, metadata, review_status,
			reviewer_id, submitted_at, reviewed_at
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	_, err = s.pool.Exec(ctx, q,
		o.ID, o.TaskID, o.WorkerID, o.HMACSha256,
		artifactsJSON, metaJSON, string(o.ReviewStatus),
		o.ReviewerID, o.SubmittedAt, o.ReviewedAt,
	)
	if err != nil {
		return fmt.Errorf("outputs: create: %w", err)
	}
	return nil
}

// GetOutput retrieves a single output package by primary key.
func (s *Store) GetOutput(ctx context.Context, id uuid.UUID) (*domain.OutputPackage, error) {
	const q = `
		SELECT id, task_id, worker_id, hmac_sha256,
		       artifacts, metadata, review_status,
		       reviewer_id, submitted_at, reviewed_at
		FROM outputs
		WHERE id = $1
	`
	row := s.pool.QueryRow(ctx, q, id)
	o, err := scanOutput(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("outputs: not found: %s", id)
		}
		return nil, fmt.Errorf("outputs: get: %w", err)
	}
	return o, nil
}

// UpdateOutputReview sets the review_status, reviewer_id, and reviewed_at for
// an output package.  Enforces the DB constraint that approved outputs must
// have a non-nil reviewer.
func (s *Store) UpdateOutputReview(ctx context.Context, id uuid.UUID, status domain.ReviewStatus, reviewerID uuid.UUID) error {
	now := time.Now().UTC()
	const q = `
		UPDATE outputs
		SET review_status = $1,
		    reviewer_id   = $2,
		    reviewed_at   = $3
		WHERE id = $4
	`
	tag, err := s.pool.Exec(ctx, q, string(status), reviewerID, now, id)
	if err != nil {
		return fmt.Errorf("outputs: update review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("outputs: not found: %s", id)
	}
	return nil
}

// scanOutput reads one outputs row into an OutputPackage.
func scanOutput(row pgx.Row) (*domain.OutputPackage, error) {
	var o domain.OutputPackage
	var artifactsRaw, metaRaw []byte
	var reviewStatus string

	if err := row.Scan(
		&o.ID, &o.TaskID, &o.WorkerID, &o.HMACSha256,
		&artifactsRaw, &metaRaw, &reviewStatus,
		&o.ReviewerID, &o.SubmittedAt, &o.ReviewedAt,
	); err != nil {
		return nil, err
	}

	o.ReviewStatus = domain.ReviewStatus(reviewStatus)

	if len(artifactsRaw) > 0 {
		if err := json.Unmarshal(artifactsRaw, &o.Artifacts); err != nil {
			return nil, fmt.Errorf("outputs: unmarshal artifacts: %w", err)
		}
	}
	if len(metaRaw) > 0 {
		if err := json.Unmarshal(metaRaw, &o.Metadata); err != nil {
			return nil, fmt.Errorf("outputs: unmarshal metadata: %w", err)
		}
	}
	return &o, nil
}
