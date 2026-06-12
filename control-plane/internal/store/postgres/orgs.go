package postgres

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grid-computing/control-plane/internal/domain"
)

// CreateOrg inserts a new org unit into the database.
func (s *Store) CreateOrg(ctx context.Context, org *domain.OrgUnit) error {
	policyJSON, err := json.Marshal(org.Policy)
	if err != nil {
		return fmt.Errorf("orgs: marshal policy: %w", err)
	}

	const q = `
		INSERT INTO orgs (id, name, plan_tier, policy, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6)
	`
	_, err = s.pool.Exec(ctx, q,
		org.ID,
		org.Name,
		org.PlanTier,
		policyJSON,
		org.CreatedAt,
		org.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("orgs: create: %w", err)
	}
	return nil
}

// GetOrg retrieves a single org unit by primary key.
func (s *Store) GetOrg(ctx context.Context, id uuid.UUID) (*domain.OrgUnit, error) {
	const q = `
		SELECT id, name, plan_tier, policy, created_at, updated_at
		FROM orgs
		WHERE id = $1
	`
	row := s.pool.QueryRow(ctx, q, id)
	org, err := scanOrg(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("orgs: not found: %s", id)
		}
		return nil, fmt.Errorf("orgs: get: %w", err)
	}
	return org, nil
}

// ListOrgs returns all org units ordered by creation time.
func (s *Store) ListOrgs(ctx context.Context) ([]*domain.OrgUnit, error) {
	const q = `
		SELECT id, name, plan_tier, policy, created_at, updated_at
		FROM orgs
		ORDER BY created_at ASC
	`
	rows, err := s.pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("orgs: list query: %w", err)
	}
	defer rows.Close()

	var orgs []*domain.OrgUnit
	for rows.Next() {
		org, err := scanOrg(rows)
		if err != nil {
			return nil, fmt.Errorf("orgs: list scan: %w", err)
		}
		orgs = append(orgs, org)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("orgs: list rows: %w", err)
	}
	return orgs, nil
}

// UpdateOrgPolicy replaces the policy JSONB for the given org unit.
func (s *Store) UpdateOrgPolicy(ctx context.Context, id uuid.UUID, policy domain.OrgPolicy) error {
	policyJSON, err := json.Marshal(policy)
	if err != nil {
		return fmt.Errorf("orgs: marshal policy: %w", err)
	}

	const q = `
		UPDATE orgs
		SET policy = $1, updated_at = NOW()
		WHERE id = $2
	`
	tag, err := s.pool.Exec(ctx, q, policyJSON, id)
	if err != nil {
		return fmt.Errorf("orgs: update policy: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("orgs: not found: %s", id)
	}
	return nil
}

// scanOrg reads one row into an OrgUnit.  It accepts both pgx.Row and pgx.Rows
// via the common RowScanner interface.
func scanOrg(row pgx.Row) (*domain.OrgUnit, error) {
	var org domain.OrgUnit
	var policyRaw []byte

	if err := row.Scan(
		&org.ID,
		&org.Name,
		&org.PlanTier,
		&policyRaw,
		&org.CreatedAt,
		&org.UpdatedAt,
	); err != nil {
		return nil, err
	}

	if err := json.Unmarshal(policyRaw, &org.Policy); err != nil {
		return nil, fmt.Errorf("unmarshal policy: %w", err)
	}
	return &org, nil
}
