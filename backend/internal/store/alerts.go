package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"geofence/internal/ids"
)

// AlertRule is an alert configuration. The README exposes these as "alerts"
// with an "alert_id" (POST /alerts/configure, GET /alerts).
type AlertRule struct {
	ID            string
	GeofenceID    string
	GeofenceName  string
	VehicleID     *string // nil = applies to all vehicles
	VehicleNumber *string
	EventType     string // entry | exit | both
	Status        string // active | inactive
	CreatedAt     time.Time
}

type CreateAlertRuleInput struct {
	GeofenceID string
	VehicleID  *string
	EventType  string
}

func (s *Store) CreateAlertRule(ctx context.Context, in CreateAlertRuleInput) (AlertRule, error) {
	ok, err := s.geofenceExists(ctx, in.GeofenceID)
	if err != nil {
		return AlertRule{}, err
	}
	if !ok {
		return AlertRule{}, &NotFoundError{Resource: "geofence", ID: in.GeofenceID}
	}
	if in.VehicleID != nil {
		ok, err := s.vehicleExists(ctx, *in.VehicleID)
		if err != nil {
			return AlertRule{}, err
		}
		if !ok {
			return AlertRule{}, &NotFoundError{Resource: "vehicle", ID: *in.VehicleID}
		}
	}

	id := ids.New(ids.Alert)
	_, err = s.pool.Exec(ctx,
		`INSERT INTO alert_configurations (id, geofence_id, vehicle_id, event_type) VALUES ($1, $2, $3, $4)`,
		id, in.GeofenceID, in.VehicleID, in.EventType)
	if isUniqueViolation(err) {
		var existing string
		_ = s.pool.QueryRow(ctx,
			`SELECT id FROM alert_configurations
			  WHERE geofence_id = $1 AND vehicle_id IS NOT DISTINCT FROM $2 AND event_type = $3`,
			in.GeofenceID, in.VehicleID, in.EventType).Scan(&existing)
		return AlertRule{}, &ConflictError{
			Message: "an identical alert rule already exists",
			Details: map[string]any{"alert_id": existing},
		}
	}
	if err != nil {
		return AlertRule{}, fmt.Errorf("insert alert rule: %w", err)
	}
	return s.GetAlertRule(ctx, id)
}

const alertRuleSelect = `
	SELECT a.id, a.geofence_id, g.name, a.vehicle_id, v.vehicle_number, a.event_type, a.status, a.created_at
	  FROM alert_configurations a
	  JOIN geofences g ON g.id = a.geofence_id
	  LEFT JOIN vehicles v ON v.id = a.vehicle_id`

func scanAlertRule(row pgx.Row) (AlertRule, error) {
	var r AlertRule
	err := row.Scan(&r.ID, &r.GeofenceID, &r.GeofenceName, &r.VehicleID, &r.VehicleNumber, &r.EventType, &r.Status, &r.CreatedAt)
	r.CreatedAt = utc(r.CreatedAt)
	return r, err
}

func (s *Store) GetAlertRule(ctx context.Context, id string) (AlertRule, error) {
	r, err := scanAlertRule(s.pool.QueryRow(ctx, alertRuleSelect+` WHERE a.id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return AlertRule{}, &NotFoundError{Resource: "alert", ID: id}
	}
	return r, err
}

// ListAlertRules filters are optional ("" = no filter).
// Filtering by vehicle_id returns that vehicle's rules AND the all-vehicles
// rules, because both apply to that vehicle.
func (s *Store) ListAlertRules(ctx context.Context, geofenceID, vehicleID string, page Page) ([]AlertRule, error) {
	rows, err := s.pool.Query(ctx, alertRuleSelect+`
		WHERE ($1 = '' OR a.geofence_id = $1)
		  AND ($2 = '' OR a.vehicle_id = $2 OR a.vehicle_id IS NULL)
		ORDER BY a.created_at DESC, a.id
		LIMIT $3 OFFSET $4`,
		geofenceID, vehicleID, page.limitArg(), page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list alert rules: %w", err)
	}
	defer rows.Close()
	out := []AlertRule{}
	for rows.Next() {
		r, err := scanAlertRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// SetAlertRuleStatus enables/disables a rule without deleting it (extra endpoint).
func (s *Store) SetAlertRuleStatus(ctx context.Context, id, status string) (AlertRule, error) {
	tag, err := s.pool.Exec(ctx, `UPDATE alert_configurations SET status = $2 WHERE id = $1`, id, status)
	if err != nil {
		return AlertRule{}, fmt.Errorf("update alert rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return AlertRule{}, &NotFoundError{Resource: "alert", ID: id}
	}
	return s.GetAlertRule(ctx, id)
}

// DeleteAlertRule removes a rule (extra endpoint). Already-fired alerts are kept
// (alerts.alert_config_id is set to NULL by the foreign key).
func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM alert_configurations WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete alert rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &NotFoundError{Resource: "alert", ID: id}
	}
	return nil
}

// ListFiredAlerts returns persisted alert messages, newest first. The frontend
// uses it to fill the live feed on page load and after a WebSocket reconnect.
func (s *Store) ListFiredAlerts(ctx context.Context, limit int) ([]AlertMessage, error) {
	rows, err := s.pool.Query(ctx, `SELECT payload FROM alerts ORDER BY created_at DESC, id DESC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list fired alerts: %w", err)
	}
	defer rows.Close()
	out := []AlertMessage{}
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var m AlertMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}
