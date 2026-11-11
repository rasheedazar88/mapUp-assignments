package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"geofence/internal/ids"
	"geofence/internal/tracking"
)

// FormatTime is the single timestamp format used in API responses and alerts:
// ISO 8601 / RFC 3339 in UTC, e.g. "2025-01-15T10:35:00Z".
func FormatTime(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }

type LocationUpdate struct {
	VehicleID string
	Latitude  float64
	Longitude float64
	Timestamp time.Time
}

// AlertMessage is EXACTLY the WebSocket message format from the README.
// The same JSON is stored in alerts.payload and broadcast on /ws/alerts.
type AlertMessage struct {
	EventID   string        `json:"event_id"`
	EventType string        `json:"event_type"`
	Timestamp string        `json:"timestamp"`
	Vehicle   AlertVehicle  `json:"vehicle"`
	Geofence  AlertGeofence `json:"geofence"`
	Location  LatLng        `json:"location"`
}

type AlertVehicle struct {
	VehicleID     string `json:"vehicle_id"`
	VehicleNumber string `json:"vehicle_number"`
	DriverName    string `json:"driver_name"`
}

type AlertGeofence struct {
	GeofenceID   string `json:"geofence_id"`
	GeofenceName string `json:"geofence_name"`
	Category     string `json:"category"`
}

type GeofenceEvent struct {
	ID        string
	Geofence  GeofenceRef
	EventType string // entry | exit
}

type LocationResult struct {
	CurrentGeofences []GeofenceRef   // geofences containing the new point
	Events           []GeofenceEvent // entry/exit transitions detected
	Alerts           []AlertMessage  // persisted alerts to broadcast (subset of Events with a matching rule)
	Stale            bool            // timestamp older than the last known one: stored, but no state change
}

// RecordLocation is the heart of the system (POST /vehicles/location).
// Everything runs in ONE transaction, so either all of it is saved or none of it:
//
//  1. Lock the vehicle row (SELECT … FOR UPDATE). Two concurrent updates for the
//     same vehicle now run one after the other, so both cannot read the same
//     "before" state and both emit an "entry" event.
//  2. Store the point in vehicle_locations (movement history).
//  3. Ask PostGIS which active geofences cover the point.
//  4. Diff with the previous membership → entry / exit events.
//  5. Save events, matching alerts, the new membership, and the vehicle's last position.
//
// Broadcasting the alerts is NOT done here. The caller does it after COMMIT,
// asynchronously, so clients never see an alert that was later rolled back.
func (s *Store) RecordLocation(ctx context.Context, in LocationUpdate) (LocationResult, error) {
	var res LocationResult

	err := pgx.BeginFunc(ctx, s.pool, func(tx pgx.Tx) error {
		// 1. Lock the vehicle row.
		var vehicleNumber, driverName string
		var lastSeen *time.Time
		err := tx.QueryRow(ctx,
			`SELECT vehicle_number, driver_name, last_seen_at FROM vehicles WHERE id = $1 FOR UPDATE`,
			in.VehicleID,
		).Scan(&vehicleNumber, &driverName, &lastSeen)
		if errors.Is(err, pgx.ErrNoRows) {
			return &NotFoundError{Resource: "vehicle", ID: in.VehicleID}
		}
		if err != nil {
			return fmt.Errorf("lock vehicle: %w", err)
		}

		// 2. Movement history. ST_MakePoint takes (x, y) = (longitude, latitude).
		var locationID int64
		if err := tx.QueryRow(ctx,
			`INSERT INTO vehicle_locations (vehicle_id, location, recorded_at)
			 VALUES ($1, ST_SetSRID(ST_MakePoint($2, $3), 4326), $4)
			 RETURNING id`,
			in.VehicleID, in.Longitude, in.Latitude, in.Timestamp,
		).Scan(&locationID); err != nil {
			return fmt.Errorf("insert location: %w", err)
		}

		// 3. Point-in-polygon.
		current, err := geofencesCovering(ctx, tx, in.Latitude, in.Longitude)
		if err != nil {
			return err
		}
		res.CurrentGeofences = current

		// Out-of-order (delayed) GPS packet: keep it in history, but don't let an
		// old position rewind the current state and create false exit/entry pairs.
		if lastSeen != nil && in.Timestamp.Before(*lastSeen) {
			res.Stale = true
			return nil
		}

		// 4. Previous membership → diff.
		before, err := membership(ctx, tx, in.VehicleID)
		if err != nil {
			return err
		}
		refs := make(map[string]GeofenceRef, len(current))
		currentIDs := make([]string, 0, len(current))
		for _, g := range current {
			refs[g.ID] = g
			currentIDs = append(currentIDs, g.ID)
		}
		entered, exited := tracking.Diff(before, currentIDs)

		// Exited geofences are (by definition) not in `current`; load their names.
		if len(exited) > 0 {
			exitedRefs, err := geofenceRefs(ctx, tx, exited)
			if err != nil {
				return err
			}
			for id, ref := range exitedRefs {
				refs[id] = ref
			}
		}

		vehicle := AlertVehicle{VehicleID: in.VehicleID, VehicleNumber: vehicleNumber, DriverName: driverName}

		// 5. Apply transitions: exits first, then entries (chronologically sensible order).
		for _, id := range exited {
			if _, err := tx.Exec(ctx,
				`DELETE FROM vehicle_geofence_state WHERE vehicle_id = $1 AND geofence_id = $2`,
				in.VehicleID, id); err != nil {
				return fmt.Errorf("delete membership: %w", err)
			}
			if err := s.recordEvent(ctx, tx, &res, in, locationID, vehicle, refs[id], "exit"); err != nil {
				return err
			}
		}
		for _, id := range entered {
			if _, err := tx.Exec(ctx,
				`INSERT INTO vehicle_geofence_state (vehicle_id, geofence_id, entered_at) VALUES ($1, $2, $3)`,
				in.VehicleID, id, in.Timestamp); err != nil {
				return fmt.Errorf("insert membership: %w", err)
			}
			if err := s.recordEvent(ctx, tx, &res, in, locationID, vehicle, refs[id], "entry"); err != nil {
				return err
			}
		}

		if _, err := tx.Exec(ctx,
			`UPDATE vehicles
			    SET last_location = ST_SetSRID(ST_MakePoint($2, $3), 4326), last_seen_at = $4
			  WHERE id = $1`,
			in.VehicleID, in.Longitude, in.Latitude, in.Timestamp); err != nil {
			return fmt.Errorf("update vehicle position: %w", err)
		}
		return nil
	})
	if err != nil {
		return LocationResult{}, err
	}
	return res, nil
}

// recordEvent stores one entry/exit event and, if an active alert rule matches,
// a fired alert with the exact WebSocket payload.
func (s *Store) recordEvent(ctx context.Context, tx pgx.Tx, res *LocationResult, in LocationUpdate,
	locationID int64, vehicle AlertVehicle, g GeofenceRef, eventType string) error {

	ev := GeofenceEvent{ID: ids.New(ids.Event), Geofence: g, EventType: eventType}
	if _, err := tx.Exec(ctx,
		`INSERT INTO geofence_events (id, vehicle_id, geofence_id, location_id, event_type, location, occurred_at)
		 VALUES ($1, $2, $3, $4, $5, ST_SetSRID(ST_MakePoint($6, $7), 4326), $8)`,
		ev.ID, in.VehicleID, g.ID, locationID, eventType, in.Longitude, in.Latitude, in.Timestamp); err != nil {
		return fmt.Errorf("insert geofence event: %w", err)
	}
	res.Events = append(res.Events, ev)

	// Rule matching:
	//   same geofence, active,
	//   for THIS vehicle or for all vehicles (vehicle_id IS NULL),
	//   for THIS event type or for 'both'.
	// Several rules can match one event (e.g. a vehicle rule and an all-vehicles
	// rule). We still create exactly ONE alert, preferring the vehicle-specific rule.
	var ruleID string
	err := tx.QueryRow(ctx,
		`SELECT id FROM alert_configurations
		  WHERE geofence_id = $1
		    AND status = 'active'
		    AND (vehicle_id = $2 OR vehicle_id IS NULL)
		    AND event_type IN ($3, 'both')
		  ORDER BY (vehicle_id IS NULL), created_at
		  LIMIT 1`,
		g.ID, in.VehicleID, eventType).Scan(&ruleID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // no rule → event is in history, but nobody is alerted
	}
	if err != nil {
		return fmt.Errorf("match alert rule: %w", err)
	}

	msg := AlertMessage{
		EventID:   ids.New(ids.Fired),
		EventType: eventType,
		Timestamp: FormatTime(in.Timestamp),
		Vehicle:   vehicle,
		Geofence:  AlertGeofence{GeofenceID: g.ID, GeofenceName: g.Name, Category: g.Category},
		Location:  LatLng{Latitude: in.Latitude, Longitude: in.Longitude},
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx,
		`INSERT INTO alerts (id, geofence_event_id, alert_config_id, payload) VALUES ($1, $2, $3, $4::jsonb)`,
		msg.EventID, ev.ID, ruleID, string(payload)); err != nil {
		return fmt.Errorf("insert alert: %w", err)
	}
	res.Alerts = append(res.Alerts, msg)
	return nil
}

// querier is satisfied by both the pool and a transaction (pgx.Tx).
type querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// geofencesCovering is the point-in-polygon query.
//
// ST_Covers(polygon, point) is true when the point is inside OR exactly on the
// boundary. (ST_Contains would say "outside" for boundary points, and the
// README example puts the vehicle exactly on a polygon corner and expects "inside".)
// The GiST index on boundary makes this a fast bounding-box lookup followed by
// an exact test on the few candidates.
func geofencesCovering(ctx context.Context, q querier, lat, lng float64) ([]GeofenceRef, error) {
	rows, err := q.Query(ctx,
		`SELECT id, name, category
		   FROM geofences
		  WHERE status = 'active'
		    AND ST_Covers(boundary, ST_SetSRID(ST_MakePoint($1, $2), 4326))
		  ORDER BY name, id`,
		lng, lat)
	if err != nil {
		return nil, fmt.Errorf("point in polygon: %w", err)
	}
	return collectRefs(rows)
}

func membership(ctx context.Context, q querier, vehicleID string) ([]string, error) {
	rows, err := q.Query(ctx, `SELECT geofence_id FROM vehicle_geofence_state WHERE vehicle_id = $1`, vehicleID)
	if err != nil {
		return nil, fmt.Errorf("load membership: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

func geofenceRefs(ctx context.Context, q querier, idList []string) (map[string]GeofenceRef, error) {
	rows, err := q.Query(ctx, `SELECT id, name, category FROM geofences WHERE id = ANY($1)`, idList)
	if err != nil {
		return nil, fmt.Errorf("load geofences: %w", err)
	}
	refs, err := collectRefs(rows)
	if err != nil {
		return nil, err
	}
	out := make(map[string]GeofenceRef, len(refs))
	for _, r := range refs {
		out[r.ID] = r
	}
	return out, nil
}

func collectRefs(rows pgx.Rows) ([]GeofenceRef, error) {
	defer rows.Close()
	out := []GeofenceRef{}
	for rows.Next() {
		var r GeofenceRef
		if err := rows.Scan(&r.ID, &r.Name, &r.Category); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// VehicleStatus is a vehicle with its last known position and the geofences it is inside.
type VehicleStatus struct {
	Vehicle
	CurrentGeofences []GeofenceRef
}

// GetVehicleLocation backs GET /vehicles/location/{vehicle_id}.
// current_geofences comes from vehicle_geofence_state, the same state that
// drives entry/exit detection, so the two endpoints never disagree.
func (s *Store) GetVehicleLocation(ctx context.Context, vehicleID string) (VehicleStatus, error) {
	v, err := s.GetVehicle(ctx, vehicleID)
	if err != nil {
		return VehicleStatus{}, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT g.id, g.name, g.category
		   FROM vehicle_geofence_state st
		   JOIN geofences g ON g.id = st.geofence_id
		  WHERE st.vehicle_id = $1
		  ORDER BY g.name, g.id`, vehicleID)
	if err != nil {
		return VehicleStatus{}, fmt.Errorf("load vehicle geofences: %w", err)
	}
	refs, err := collectRefs(rows)
	if err != nil {
		return VehicleStatus{}, err
	}
	return VehicleStatus{Vehicle: v, CurrentGeofences: refs}, nil
}

// ListVehicleLocations backs the extra bulk endpoint GET /vehicles/locations
// (2 queries in total instead of 1 + N calls to the per-vehicle endpoint).
func (s *Store) ListVehicleLocations(ctx context.Context) ([]VehicleStatus, error) {
	vehicles, err := s.ListVehicles(ctx, Page{})
	if err != nil {
		return nil, err
	}
	rows, err := s.pool.Query(ctx,
		`SELECT st.vehicle_id, g.id, g.name, g.category
		   FROM vehicle_geofence_state st
		   JOIN geofences g ON g.id = st.geofence_id
		  ORDER BY g.name, g.id`)
	if err != nil {
		return nil, fmt.Errorf("load memberships: %w", err)
	}
	defer rows.Close()
	byVehicle := map[string][]GeofenceRef{}
	for rows.Next() {
		var vehicleID string
		var r GeofenceRef
		if err := rows.Scan(&vehicleID, &r.ID, &r.Name, &r.Category); err != nil {
			return nil, err
		}
		byVehicle[vehicleID] = append(byVehicle[vehicleID], r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]VehicleStatus, 0, len(vehicles))
	for _, v := range vehicles {
		refs := byVehicle[v.ID]
		if refs == nil {
			refs = []GeofenceRef{}
		}
		out = append(out, VehicleStatus{Vehicle: v, CurrentGeofences: refs})
	}
	return out, nil
}
