package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"geofence/internal/geo"
	"geofence/internal/store"
)

// Device clocks drift; allow small skew but reject timestamps far in the future,
// which would otherwise make every later (correct) update look "out of order".
const maxClockSkew = 5 * time.Minute

type updateLocationRequest struct {
	VehicleID string   `json:"vehicle_id"`
	Latitude  *float64 `json:"latitude"` // pointers: lets us tell "missing" from a real 0.0
	Longitude *float64 `json:"longitude"`
	Timestamp string   `json:"timestamp"`
}

type insideGeofenceJSON struct {
	GeofenceID   string `json:"geofence_id"`
	GeofenceName string `json:"geofence_name"`
	Status       string `json:"status"`
}

type updateLocationResponse struct {
	VehicleID        string               `json:"vehicle_id"`
	LocationUpdated  bool                 `json:"location_updated"`
	CurrentGeofences []insideGeofenceJSON `json:"current_geofences"`
	Timing
}

// POST /vehicles/location
func (a *API) updateLocation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req updateLocationRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadJSON(w, start, err)
		return
	}
	req.VehicleID = strings.TrimSpace(req.VehicleID)

	fe := fieldErrors{}
	if req.VehicleID == "" {
		fe.add("vehicle_id", "is required")
	}
	if req.Latitude == nil {
		fe.add("latitude", "is required")
	}
	if req.Longitude == nil {
		fe.add("longitude", "is required")
	}
	if req.Latitude != nil && req.Longitude != nil {
		if err := geo.ValidateLatLng(*req.Latitude, *req.Longitude); err != nil {
			field := "latitude"
			if strings.HasPrefix(err.Error(), "longitude") {
				field = "longitude"
			}
			fe.add(field, err.Error())
		}
	}
	var ts time.Time
	if req.Timestamp == "" {
		fe.add("timestamp", "is required")
	} else if t, ok := parseTimestamp(req.Timestamp); !ok {
		fe.add("timestamp", "must be an ISO 8601 timestamp, e.g. 2025-01-15T10:35:00Z")
	} else if t.After(time.Now().Add(maxClockSkew)) {
		fe.add("timestamp", "must not be in the future")
	} else {
		ts = t
	}
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	// Rate limit per vehicle (bonus). Checked after validation so bad requests
	// don't burn the vehicle's budget.
	if !a.limiter.Allow(req.VehicleID) {
		w.Header().Set("Retry-After", "1")
		writeError(w, http.StatusTooManyRequests, start, "rate_limited", "too many location updates for this vehicle; slow down", nil)
		return
	}

	res, err := a.store.RecordLocation(r.Context(), store.LocationUpdate{
		VehicleID: req.VehicleID, Latitude: *req.Latitude, Longitude: *req.Longitude, Timestamp: ts,
	})
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	if res.Stale {
		slog.Info("out-of-order location stored without state change", "vehicle_id", req.VehicleID, "timestamp", req.Timestamp)
	}

	// Async delivery: Publish only drops each message into the hub's buffered
	// channel and returns immediately. The actual network writes happen on the
	// hub/client goroutines, so this HTTP response is never delayed by them.
	// Alerts were already saved in the database inside RecordLocation's transaction.
	for _, msg := range res.Alerts {
		if payload, err := json.Marshal(msg); err == nil {
			a.hub.Publish(payload)
		}
	}

	inside := make([]insideGeofenceJSON, len(res.CurrentGeofences))
	for i, g := range res.CurrentGeofences {
		inside[i] = insideGeofenceJSON{GeofenceID: g.ID, GeofenceName: g.Name, Status: "inside"}
	}
	writeJSON(w, http.StatusOK, start, &updateLocationResponse{
		VehicleID: req.VehicleID, LocationUpdated: true, CurrentGeofences: inside,
	})
}

type currentLocationJSON struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp string  `json:"timestamp"`
}

type geofenceRefJSON struct {
	GeofenceID   string `json:"geofence_id"`
	GeofenceName string `json:"geofence_name"`
	Category     string `json:"category"`
}

type vehicleLocationResponse struct {
	VehicleID        string               `json:"vehicle_id"`
	VehicleNumber    string               `json:"vehicle_number"`
	CurrentLocation  *currentLocationJSON `json:"current_location"` // null until the first update
	CurrentGeofences []geofenceRefJSON    `json:"current_geofences"`
	Timing
}

func toCurrentLocation(v store.Vehicle) *currentLocationJSON {
	if v.LastLocation == nil || v.LastSeenAt == nil {
		return nil
	}
	return &currentLocationJSON{
		Latitude: v.LastLocation.Latitude, Longitude: v.LastLocation.Longitude, Timestamp: store.FormatTime(*v.LastSeenAt),
	}
}

func toRefsJSON(refs []store.GeofenceRef) []geofenceRefJSON {
	out := make([]geofenceRefJSON, len(refs))
	for i, g := range refs {
		out[i] = geofenceRefJSON{GeofenceID: g.ID, GeofenceName: g.Name, Category: g.Category}
	}
	return out
}

// GET /vehicles/location/{vehicle_id}
func (a *API) getVehicleLocation(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	st, err := a.store.GetVehicleLocation(r.Context(), r.PathValue("vehicle_id"))
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusOK, start, &vehicleLocationResponse{
		VehicleID:        st.ID,
		VehicleNumber:    st.VehicleNumber,
		CurrentLocation:  toCurrentLocation(st.Vehicle),
		CurrentGeofences: toRefsJSON(st.CurrentGeofences),
	})
}

type vehicleStatusJSON struct {
	VehicleID        string               `json:"vehicle_id"`
	VehicleNumber    string               `json:"vehicle_number"`
	DriverName       string               `json:"driver_name"`
	VehicleType      string               `json:"vehicle_type"`
	Status           string               `json:"status"`
	CurrentLocation  *currentLocationJSON `json:"current_location"`
	CurrentGeofences []geofenceRefJSON    `json:"current_geofences"`
}

type listVehicleLocationsResponse struct {
	Vehicles []vehicleStatusJSON `json:"vehicles"`
	Timing
}

// GET /vehicles/locations (extra): every vehicle's position + geofences in one call,
// for the live map and dashboard.
func (a *API) listVehicleLocations(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	list, err := a.store.ListVehicleLocations(r.Context())
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	out := make([]vehicleStatusJSON, len(list))
	for i, st := range list {
		out[i] = vehicleStatusJSON{
			VehicleID: st.ID, VehicleNumber: st.VehicleNumber, DriverName: st.DriverName, VehicleType: st.VehicleType,
			Status: st.Status, CurrentLocation: toCurrentLocation(st.Vehicle), CurrentGeofences: toRefsJSON(st.CurrentGeofences),
		}
	}
	writeJSON(w, http.StatusOK, start, &listVehicleLocationsResponse{Vehicles: out})
}
