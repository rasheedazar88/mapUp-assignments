package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"geofence/internal/store"
)

const (
	defaultHistoryLimit = 50
	maxHistoryLimit     = 500
)

type violationJSON struct {
	ID            string  `json:"id"`
	VehicleID     string  `json:"vehicle_id"`
	VehicleNumber string  `json:"vehicle_number"`
	GeofenceID    string  `json:"geofence_id"`
	GeofenceName  string  `json:"geofence_name"`
	Category      string  `json:"category"`
	EventType     string  `json:"event_type"`
	Latitude      float64 `json:"latitude"`
	Longitude     float64 `json:"longitude"`
	Timestamp     string  `json:"timestamp"`
}

type violationHistoryResponse struct {
	Violations []violationJSON `json:"violations"`
	TotalCount int             `json:"total_count"`
	Timing
}

// GET /violations/history?vehicle_id=&geofence_id=&start_date=&end_date=&limit=&offset=
func (a *API) violationHistory(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := r.URL.Query()
	fe := fieldErrors{}

	f := store.ViolationFilter{
		VehicleID:  strings.TrimSpace(q.Get("vehicle_id")),
		GeofenceID: strings.TrimSpace(q.Get("geofence_id")),
		Limit:      defaultHistoryLimit,
	}
	if s := q.Get("start_date"); s != "" {
		if t, ok := parseDateParam(s, false); ok {
			f.StartDate = &t
		} else {
			fe.add("start_date", "must be ISO 8601, e.g. 2025-01-01T00:00:00Z")
		}
	}
	if s := q.Get("end_date"); s != "" {
		if t, ok := parseDateParam(s, true); ok {
			f.EndDate = &t
		} else {
			fe.add("end_date", "must be ISO 8601, e.g. 2025-01-31T23:59:59Z")
		}
	}
	if f.StartDate != nil && f.EndDate != nil && f.StartDate.After(*f.EndDate) {
		fe.add("end_date", "must not be before start_date")
	}
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		switch {
		case err != nil || n < 1:
			fe.add("limit", "must be a positive integer")
		case n > maxHistoryLimit:
			f.Limit = maxHistoryLimit // README: "max: 500" → clamp
		default:
			f.Limit = n
		}
	}
	f.Offset = parseOffset(q, fe) // extra: offset-based pagination
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	list, total, err := a.store.ListViolations(r.Context(), f)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	out := make([]violationJSON, len(list))
	for i, v := range list {
		out[i] = violationJSON{
			ID: v.ID, VehicleID: v.VehicleID, VehicleNumber: v.VehicleNumber, GeofenceID: v.GeofenceID,
			GeofenceName: v.GeofenceName, Category: v.Category, EventType: v.EventType,
			Latitude: v.Latitude, Longitude: v.Longitude, Timestamp: store.FormatTime(v.Timestamp),
		}
	}
	writeJSON(w, http.StatusOK, start, &violationHistoryResponse{Violations: out, TotalCount: total})
}
