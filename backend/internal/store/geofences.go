package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"geofence/internal/geo"
	"geofence/internal/ids"
)

type Geofence struct {
	ID          string
	Name        string
	Description string
	Category    string
	Status      string
	Coordinates [][]float64 // [lat, lng] pairs, closed ring
	CreatedAt   time.Time
}

type CreateGeofenceInput struct {
	Name        string
	Description string
	Category    string
	Coordinates [][]float64 // already validated by geo.ValidatePolygon
}

// CreateGeofence asks PostGIS to check validity first (self-intersections etc.),
// so a bad polygon gets a clear 400 instead of a CHECK-constraint 500.
func (s *Store) CreateGeofence(ctx context.Context, in CreateGeofenceInput) (Geofence, error) {
	wkt := geo.PolygonWKT(in.Coordinates)

	var valid bool
	var reason string
	err := s.pool.QueryRow(ctx,
		`SELECT ST_IsValid(g), ST_IsValidReason(g)
		   FROM (SELECT ST_GeomFromText($1, 4326) AS g) AS t`, wkt,
	).Scan(&valid, &reason)
	if err != nil {
		return Geofence{}, fmt.Errorf("validate polygon: %w", err)
	}
	if !valid {
		return Geofence{}, &ValidationError{Field: "coordinates", Message: "polygon is not valid (" + reason + "); edges must not cross each other"}
	}

	g := Geofence{
		ID:          ids.New(ids.Geofence),
		Name:        in.Name,
		Description: in.Description,
		Category:    in.Category,
		Status:      "active",
		Coordinates: in.Coordinates,
	}
	err = s.pool.QueryRow(ctx,
		`INSERT INTO geofences (id, name, description, category, boundary)
		 VALUES ($1, $2, $3, $4, ST_GeomFromText($5, 4326))
		 RETURNING created_at`,
		g.ID, g.Name, g.Description, g.Category, wkt,
	).Scan(&g.CreatedAt)
	if err != nil {
		return Geofence{}, fmt.Errorf("insert geofence: %w", err)
	}
	g.CreatedAt = utc(g.CreatedAt)
	return g, nil
}

const geofenceColumns = `id, name, description, category, status, ST_AsGeoJSON(boundary, 15)::text, created_at`

func scanGeofence(row pgx.Row) (Geofence, error) {
	var g Geofence
	var geoJSON string
	if err := row.Scan(&g.ID, &g.Name, &g.Description, &g.Category, &g.Status, &geoJSON, &g.CreatedAt); err != nil {
		return Geofence{}, err
	}
	coords, err := geo.PolygonFromGeoJSON([]byte(geoJSON))
	if err != nil {
		return Geofence{}, err
	}
	g.Coordinates = coords
	g.CreatedAt = utc(g.CreatedAt)
	return g, nil
}

// ListGeofences returns geofences, newest first. category "" means all categories.
func (s *Store) ListGeofences(ctx context.Context, category string, page Page) ([]Geofence, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+geofenceColumns+`
		   FROM geofences
		  WHERE ($1 = '' OR category = $1)
		  ORDER BY created_at DESC, id
		  LIMIT $2 OFFSET $3`,
		category, page.limitArg(), page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list geofences: %w", err)
	}
	defer rows.Close()

	out := []Geofence{} // non-nil so JSON encodes [] rather than null
	for rows.Next() {
		g, err := scanGeofence(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GetGeofence(ctx context.Context, id string) (Geofence, error) {
	g, err := scanGeofence(s.pool.QueryRow(ctx, `SELECT `+geofenceColumns+` FROM geofences WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Geofence{}, &NotFoundError{Resource: "geofence", ID: id}
	}
	return g, err
}

func (s *Store) geofenceExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM geofences WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}
