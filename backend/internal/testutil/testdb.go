// Package testutil gives integration tests an isolated, migrated database.
//
// Each test package gets its own PostgreSQL schema (test_xxxxxxxx) inside the
// database named by TEST_DATABASE_URL, so tests never touch development data
// and can run in parallel. The schema is dropped when the test finishes.
// If TEST_DATABASE_URL is not set, integration tests are skipped.
package testutil

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"geofence/internal/db"
	"geofence/internal/ids"
)

func NewTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set; skipping integration test")
	}
	ctx := context.Background()
	schema := "test_" + ids.New("")[:8]

	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}

	pool, err := db.Connect(ctx, url, func(cfg *pgxpool.Config) {
		// Tables go in the test schema; PostGIS functions are found in public.
		cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), `DROP SCHEMA `+schema+` CASCADE`)
		admin.Close()
	})
	return pool
}
