package store

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Violation struct {
	ID            string
	VehicleID     string
	VehicleNumber string
	GeofenceID    string
	GeofenceName  string
	Category      string
	EventType     string
	Latitude      float64
	Longitude     float64
	Timestamp     time.Time
}

type ViolationFilter struct {
	VehicleID  string     // "" = any
	GeofenceID string     // "" = any
	StartDate  *time.Time // inclusive
	EndDate    *time.Time // inclusive
	Limit      int
	Offset     int
}

// ListViolations backs GET /violations/history. It returns one page of events
// plus total_count, the number of ALL rows matching the filters (ignoring
// limit/offset), which the frontend needs to render pagination.
//
// The WHERE clause is built dynamically, but only from fixed SQL fragments;
// every user value goes in as a $n bind parameter.
func (s *Store) ListViolations(ctx context.Context, f ViolationFilter) ([]Violation, int, error) {
	var conds []string
	var args []any
	add := func(cond string, val any) {
		args = append(args, val)
		conds = append(conds, fmt.Sprintf(cond, len(args)))
	}
	if f.VehicleID != "" {
		add("e.vehicle_id = $%d", f.VehicleID)
	}
	if f.GeofenceID != "" {
		add("e.geofence_id = $%d", f.GeofenceID)
	}
	if f.StartDate != nil {
		add("e.occurred_at >= $%d", *f.StartDate)
	}
	if f.EndDate != nil {
		add("e.occurred_at <= $%d", *f.EndDate)
	}
	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}

	var total int
	if err := s.pool.QueryRow(ctx, `SELECT count(*) FROM geofence_events e `+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count violations: %w", err)
	}

	pageArgs := append(append([]any{}, args...), f.Limit, f.Offset)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT e.id, e.vehicle_id, v.vehicle_number, e.geofence_id, g.name, g.category,
		       e.event_type, ST_Y(e.location), ST_X(e.location), e.occurred_at
		  FROM geofence_events e
		  JOIN vehicles  v ON v.id = e.vehicle_id
		  JOIN geofences g ON g.id = e.geofence_id
		  %s
		 ORDER BY e.occurred_at DESC, e.id DESC
		 LIMIT $%d OFFSET $%d`, where, len(args)+1, len(args)+2), pageArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("list violations: %w", err)
	}
	defer rows.Close()

	out := []Violation{}
	for rows.Next() {
		var v Violation
		if err := rows.Scan(&v.ID, &v.VehicleID, &v.VehicleNumber, &v.GeofenceID, &v.GeofenceName, &v.Category,
			&v.EventType, &v.Latitude, &v.Longitude, &v.Timestamp); err != nil {
			return nil, 0, err
		}
		v.Timestamp = utc(v.Timestamp)
		out = append(out, v)
	}
	return out, total, rows.Err()
}
