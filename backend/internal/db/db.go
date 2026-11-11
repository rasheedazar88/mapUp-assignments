// Package db opens the PostgreSQL connection pool and applies schema migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// The SQL files are compiled INTO the Go binary (go:embed), so the Docker image
// needs no extra files and migrations always match the code version.
//
//go:embed migrations/*.sql
var migrationFiles embed.FS

// Connect creates a connection pool and verifies the database is reachable.
// A pool keeps several open connections and lends them to concurrent requests.
func Connect(ctx context.Context, databaseURL string, configure func(*pgxpool.Config)) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MaxConnIdleTime = 5 * time.Minute
	if configure != nil {
		configure(cfg)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	// Retry for up to ~30s: in Docker Compose the DB may still be starting.
	var pingErr error
	for attempt := 1; attempt <= 15; attempt++ {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		pingErr = pool.Ping(pingCtx)
		cancel()
		if pingErr == nil {
			return pool, nil
		}
		slog.Info("waiting for database", "attempt", attempt, "err", pingErr)
		select {
		case <-ctx.Done():
			pool.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	pool.Close()
	return nil, fmt.Errorf("database not reachable: %w", pingErr)
}

// Migrate applies every embedded migrations/*.sql file not yet recorded in
// schema_migrations, in filename order, each inside its own transaction.
// A PostgreSQL advisory lock guarantees only one process migrates at a time
// (e.g. two backend replicas starting together).
func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	const lockKey = 727274 // arbitrary app-wide constant
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck

	if _, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (
		version    TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := fs.Glob(migrationFiles, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")

		var exists bool
		if err := conn.QueryRow(ctx,
			`SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version = $1)`, version,
		).Scan(&exists); err != nil {
			return err
		}
		if exists {
			continue
		}

		sqlBytes, err := migrationFiles.ReadFile(name)
		if err != nil {
			return err
		}

		err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, string(sqlBytes)); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, `INSERT INTO schema_migrations (version) VALUES ($1)`, version)
			return err
		})
		if err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
		slog.Info("applied migration", "version", version)
	}
	return nil
}
