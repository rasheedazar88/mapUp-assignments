// Package store is the data-access layer: every SQL query lives here.
//
// All queries use bind parameters ($1, $2, …). Values are sent to PostgreSQL
// separately from the SQL text, so user input can never change the query
// (this is what prevents SQL injection).
package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

// Ping checks the database connection (used by GET /health).
func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

// ---- Errors the HTTP layer translates into status codes ----

// ErrNotFound → 404.
var ErrNotFound = errors.New("not found")

// NotFoundError → 404 with a resource-specific message ("vehicle not found").
type NotFoundError struct{ Resource, ID string }

func (e *NotFoundError) Error() string { return e.Resource + " not found: " + e.ID }
func (e *NotFoundError) Unwrap() error { return ErrNotFound }

// ConflictError → 409 (duplicate vehicle_number, duplicate alert rule).
type ConflictError struct {
	Message string
	Details map[string]any
}

func (e *ConflictError) Error() string { return e.Message }

// ValidationError → 400, for rules only the database can check
// (e.g. PostGIS says the polygon intersects itself).
type ValidationError struct{ Field, Message string }

func (e *ValidationError) Error() string { return e.Field + ": " + e.Message }

// isUniqueViolation reports whether err is PostgreSQL error 23505 (unique_violation).
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Page is optional limit/offset pagination. Limit 0 means "no limit".
type Page struct {
	Limit  int
	Offset int
}

// limitArg turns Limit 0 into SQL NULL, and "LIMIT NULL" means no limit.
func (p Page) limitArg() any {
	if p.Limit <= 0 {
		return nil
	}
	return p.Limit
}

// LatLng is a point in API order.
type LatLng struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GeofenceRef is the short geofence summary used in location responses.
type GeofenceRef struct {
	ID       string
	Name     string
	Category string
}

func utc(t time.Time) time.Time { return t.UTC() }
