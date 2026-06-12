package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/grid-computing/control-plane/internal/domain"
)

// CreateAPIKey persists a new API key record.  The plain-text value must never
// be stored; only the Argon2id hash is written here.
func (s *Store) CreateAPIKey(ctx context.Context, k *domain.APIKey) error {
	const q = `
		INSERT INTO api_keys (id, org_unit_id, worker_id, key_hash, key_prefix, name, scopes, created_at, expires_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`
	_, err := s.pool.Exec(ctx, q,
		k.ID, k.OrgUnitID, k.WorkerID, k.KeyHash, k.KeyPrefix,
		k.Name, k.Scopes, k.CreatedAt, k.ExpiresAt,
	)
	if err != nil {
		return fmt.Errorf("apikeys: create: %w", err)
	}
	return nil
}

// GetAPIKeyByPrefix looks up a non-revoked, non-expired key by its prefix token.
func (s *Store) GetAPIKeyByPrefix(ctx context.Context, prefix string) (*domain.APIKey, error) {
	const q = `
		SELECT id, org_unit_id, worker_id, key_hash, key_prefix, name, scopes,
		       created_at, expires_at, revoked_at, last_used_at
		FROM api_keys
		WHERE key_prefix  = $1
		  AND revoked_at  IS NULL
		  AND (expires_at IS NULL OR expires_at > NOW())
		LIMIT 1
	`
	row := s.pool.QueryRow(ctx, q, prefix)
	k, err := scanAPIKey(row)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("apikeys: not found for prefix: %s", prefix)
		}
		return nil, fmt.Errorf("apikeys: get by prefix: %w", err)
	}
	return k, nil
}

// RevokeAPIKey marks an API key as revoked at the current time.
// orgUnitID is required and the revoke is silently a no-op (returns error) if
// the key does not belong to that org, preventing cross-org revocation.
func (s *Store) RevokeAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID) error {
	const q = `UPDATE api_keys SET revoked_at = NOW() WHERE id = $1 AND org_unit_id = $2 AND revoked_at IS NULL`
	tag, err := s.pool.Exec(ctx, q, id, orgUnitID)
	if err != nil {
		return fmt.Errorf("apikeys: revoke: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("apikeys: not found or access denied: %s", id)
	}
	return nil
}

// UpdateAPIKeyLastUsed records the current timestamp as the last use time.
func (s *Store) UpdateAPIKeyLastUsed(ctx context.Context, id uuid.UUID) error {
	const q = `UPDATE api_keys SET last_used_at = NOW() WHERE id = $1`
	_, err := s.pool.Exec(ctx, q, id)
	if err != nil {
		return fmt.Errorf("apikeys: update last used: %w", err)
	}
	return nil
}

// RotateAPIKey atomically replaces the hash and prefix for a key that belongs
// to orgUnitID.  Returns an error if the key does not exist or belongs to a
// different org, preventing cross-org rotation.
func (s *Store) RotateAPIKey(ctx context.Context, id uuid.UUID, orgUnitID uuid.UUID, newHash, newPrefix string) (*domain.APIKey, error) {
	var k *domain.APIKey
	err := s.WithTx(ctx, func(tx pgx.Tx) error {
		const update = `
			UPDATE api_keys
			SET key_hash   = $1,
			    key_prefix = $2,
			    revoked_at = NULL,
			    created_at = NOW()
			WHERE id = $3
			  AND org_unit_id = $4
			RETURNING id, org_unit_id, worker_id, key_hash, key_prefix, name, scopes,
			          created_at, expires_at, revoked_at, last_used_at
		`
		row := tx.QueryRow(ctx, update, newHash, newPrefix, id, orgUnitID)
		var err error
		k, err = scanAPIKey(row)
		if err != nil {
			if err == pgx.ErrNoRows {
				return fmt.Errorf("apikeys: not found or access denied: %s", id)
			}
			return fmt.Errorf("apikeys: rotate update: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return k, nil
}

func scanAPIKey(row pgx.Row) (*domain.APIKey, error) {
	var k domain.APIKey
	var expiresAt, revokedAt, lastUsedAt *time.Time

	if err := row.Scan(
		&k.ID, &k.OrgUnitID, &k.WorkerID, &k.KeyHash, &k.KeyPrefix, &k.Name, &k.Scopes,
		&k.CreatedAt, &expiresAt, &revokedAt, &lastUsedAt,
	); err != nil {
		return nil, err
	}
	k.ExpiresAt = expiresAt
	k.RevokedAt = revokedAt
	k.LastUsedAt = lastUsedAt
	return &k, nil
}
