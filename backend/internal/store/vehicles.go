package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"geofence/internal/ids"
)

type Vehicle struct {
	ID            string
	VehicleNumber string
	DriverName    string
	VehicleType   string
	Phone         string
	Status        string
	CreatedAt     time.Time
	LastLocation  *LatLng    // nil until the first location update
	LastSeenAt    *time.Time // device timestamp of LastLocation
}

type CreateVehicleInput struct {
	VehicleNumber string
	DriverName    string
	VehicleType   string
	Phone         string
}

func (s *Store) CreateVehicle(ctx context.Context, in CreateVehicleInput) (Vehicle, error) {
	v := Vehicle{
		ID:            ids.New(ids.Vehicle),
		VehicleNumber: in.VehicleNumber,
		DriverName:    in.DriverName,
		VehicleType:   in.VehicleType,
		Phone:         in.Phone,
		Status:        "active",
	}
	err := s.pool.QueryRow(ctx,
		`INSERT INTO vehicles (id, vehicle_number, driver_name, vehicle_type, phone)
		 VALUES ($1, $2, $3, $4, $5)
		 RETURNING created_at`,
		v.ID, v.VehicleNumber, v.DriverName, v.VehicleType, v.Phone,
	).Scan(&v.CreatedAt)
	if isUniqueViolation(err) {
		return Vehicle{}, &ConflictError{
			Message: "a vehicle with vehicle_number " + in.VehicleNumber + " is already registered",
			Details: map[string]any{"field": "vehicle_number"},
		}
	}
	if err != nil {
		return Vehicle{}, fmt.Errorf("insert vehicle: %w", err)
	}
	v.CreatedAt = utc(v.CreatedAt)
	return v, nil
}

// ST_Y = latitude, ST_X = longitude (PostGIS points are x=lng, y=lat).
const vehicleColumns = `id, vehicle_number, driver_name, vehicle_type, phone, status, created_at,
	ST_Y(last_location), ST_X(last_location), last_seen_at`

func scanVehicle(row pgx.Row) (Vehicle, error) {
	var v Vehicle
	var lat, lng *float64
	if err := row.Scan(&v.ID, &v.VehicleNumber, &v.DriverName, &v.VehicleType, &v.Phone, &v.Status,
		&v.CreatedAt, &lat, &lng, &v.LastSeenAt); err != nil {
		return Vehicle{}, err
	}
	v.CreatedAt = utc(v.CreatedAt)
	if lat != nil && lng != nil {
		v.LastLocation = &LatLng{Latitude: *lat, Longitude: *lng}
	}
	if v.LastSeenAt != nil {
		t := utc(*v.LastSeenAt)
		v.LastSeenAt = &t
	}
	return v, nil
}

func (s *Store) ListVehicles(ctx context.Context, page Page) ([]Vehicle, error) {
	rows, err := s.pool.Query(ctx,
		`SELECT `+vehicleColumns+` FROM vehicles ORDER BY created_at DESC, id LIMIT $1 OFFSET $2`,
		page.limitArg(), page.Offset)
	if err != nil {
		return nil, fmt.Errorf("list vehicles: %w", err)
	}
	defer rows.Close()

	out := []Vehicle{}
	for rows.Next() {
		v, err := scanVehicle(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) GetVehicle(ctx context.Context, id string) (Vehicle, error) {
	v, err := scanVehicle(s.pool.QueryRow(ctx, `SELECT `+vehicleColumns+` FROM vehicles WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Vehicle{}, &NotFoundError{Resource: "vehicle", ID: id}
	}
	return v, err
}

func (s *Store) vehicleExists(ctx context.Context, id string) (bool, error) {
	var ok bool
	err := s.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM vehicles WHERE id = $1)`, id).Scan(&ok)
	return ok, err
}
