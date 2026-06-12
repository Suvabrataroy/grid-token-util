// Package postgres provides the PostgreSQL-backed persistence layer.
package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/grid-computing/control-plane/internal/config"
)

// Store wraps a pgxpool.Pool and exposes the persistence operations used by
// the control plane.
type Store struct {
	pool *pgxpool.Pool
}

// New creates and validates a connection pool using the supplied configuration.
func New(ctx context.Context, cfg config.DatabaseConfig) (*Store, error) {
	poolCfg, err := pgxpool.ParseConfig(cfg.DSN)
	if err != nil {
		return nil, fmt.Errorf("postgres: parse dsn: %w", err)
	}

	poolCfg.MaxConns = int32(cfg.MaxOpenConns)
	poolCfg.MinConns = int32(cfg.MaxIdleConns)
	poolCfg.ConnConfig.ConnectTimeout = time.Duration(cfg.ConnTimeoutSec) * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("postgres: create pool: %w", err)
	}

	// Eagerly validate connectivity.
	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(cfg.ConnTimeoutSec)*time.Second)
	defer cancel()

	if err := pool.Ping(connectCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("postgres: ping: %w", err)
	}

	return &Store{pool: pool}, nil
}

// Close releases all connections in the pool.
func (s *Store) Close() {
	s.pool.Close()
}

// Ping verifies that the database is reachable.
func (s *Store) Ping(ctx context.Context) error {
	if err := s.pool.Ping(ctx); err != nil {
		return fmt.Errorf("postgres: ping: %w", err)
	}
	return nil
}

// Pool exposes the underlying pgxpool.Pool for use by sub-packages that need
// direct query access (e.g. the brownie ledger).
func (s *Store) Pool() *pgxpool.Pool {
	return s.pool
}

// WithTx executes fn inside a serialisable transaction.  If fn returns an
// error the transaction is rolled back; otherwise it is committed.
func (s *Store) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{
		IsoLevel:   pgx.Serializable,
		AccessMode: pgx.ReadWrite,
	})
	if err != nil {
		return fmt.Errorf("postgres: begin tx: %w", err)
	}

	defer func() {
		// Best-effort rollback; the error from fn is what callers care about.
		if err != nil {
			_ = tx.Rollback(ctx)
		}
	}()

	if err = fn(tx); err != nil {
		return err
	}

	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("postgres: commit tx: %w", err)
	}
	return nil
}
