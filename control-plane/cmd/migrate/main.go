package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func main() {
	log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()

	dsn := os.Getenv("GRID_DATABASE_DSN")
	if dsn == "" {
		log.Fatal().Msg("GRID_DATABASE_DSN environment variable is required")
	}

	migrationsDir := os.Getenv("MIGRATIONS_DIR")
	if migrationsDir == "" {
		migrationsDir = "migrations"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatal().Err(err).Msg("failed to connect to database")
	}
	defer pool.Close()

	if err := pool.Ping(ctx); err != nil {
		log.Fatal().Err(err).Msg("database ping failed")
	}

	if err := runMigrations(ctx, pool, migrationsDir); err != nil {
		log.Fatal().Err(err).Msg("migration failed")
	}

	log.Info().Msg("all migrations applied successfully")
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool, dir string) error {
	// Create migrations tracking table
	_, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			id          SERIAL PRIMARY KEY,
			filename    TEXT NOT NULL UNIQUE,
			applied_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	// Get already-applied migrations
	rows, err := pool.Query(ctx, "SELECT filename FROM schema_migrations ORDER BY id")
	if err != nil {
		return fmt.Errorf("query applied migrations: %w", err)
	}
	applied := make(map[string]bool)
	for rows.Next() {
		var filename string
		if err := rows.Scan(&filename); err != nil {
			return fmt.Errorf("scan migration filename: %w", err)
		}
		applied[filename] = true
	}
	rows.Close()

	// Read migration files
	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("read migrations dir %q: %w", dir, err)
	}

	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".sql") {
			files = append(files, e.Name())
		}
	}
	sort.Strings(files)

	// Apply pending migrations
	for _, filename := range files {
		if applied[filename] {
			log.Info().Str("file", filename).Msg("already applied, skipping")
			continue
		}

		content, err := os.ReadFile(filepath.Join(dir, filename))
		if err != nil {
			return fmt.Errorf("read %s: %w", filename, err)
		}

		log.Info().Str("file", filename).Msg("applying migration")

		tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
		if err != nil {
			return fmt.Errorf("begin tx for %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx, string(content)); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("execute %s: %w", filename, err)
		}

		if _, err := tx.Exec(ctx,
			"INSERT INTO schema_migrations (filename) VALUES ($1)",
			filename,
		); err != nil {
			tx.Rollback(ctx)
			return fmt.Errorf("record migration %s: %w", filename, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit %s: %w", filename, err)
		}

		log.Info().Str("file", filename).Msg("migration applied")
	}

	return nil
}
