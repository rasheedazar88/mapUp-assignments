package httpapi

import (
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"geofence/internal/store"
)

// Phone: optional leading +, then 7–20 digits/spaces/dashes/parentheses, e.g. "+1234567890", "+91 98765-43210".
var phonePattern = regexp.MustCompile(`^\+?[0-9][0-9 ()\-]{5,18}[0-9]$`)

type createVehicleRequest struct {
	VehicleNumber string `json:"vehicle_number"`
	DriverName    string `json:"driver_name"`
	VehicleType   string `json:"vehicle_type"`
	Phone         string `json:"phone"`
}

type createVehicleResponse struct {
	ID            string `json:"id"`
	VehicleNumber string `json:"vehicle_number"`
	Status        string `json:"status"`
	Timing
}

// POST /vehicles
func (a *API) createVehicle(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req createVehicleRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadJSON(w, start, err)
		return
	}
	req.VehicleNumber = strings.TrimSpace(req.VehicleNumber)
	req.DriverName = strings.TrimSpace(req.DriverName)
	req.VehicleType = strings.ToLower(strings.TrimSpace(req.VehicleType))
	req.Phone = strings.TrimSpace(req.Phone)

	fe := fieldErrors{}
	requireText(fe, "vehicle_number", req.VehicleNumber, 32)
	requireText(fe, "driver_name", req.DriverName, 100)
	requireText(fe, "vehicle_type", req.VehicleType, 30)
	if req.Phone == "" {
		fe.add("phone", "is required")
	} else if !phonePattern.MatchString(req.Phone) {
		fe.add("phone", "must be a valid phone number, e.g. +1234567890")
	}
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	v, err := a.store.CreateVehicle(r.Context(), store.CreateVehicleInput{
		VehicleNumber: req.VehicleNumber, DriverName: req.DriverName, VehicleType: req.VehicleType, Phone: req.Phone,
	})
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusCreated, start, &createVehicleResponse{ID: v.ID, VehicleNumber: v.VehicleNumber, Status: v.Status})
}

func requireText(fe fieldErrors, field, value string, maxLen int) {
	switch {
	case value == "":
		fe.add(field, "is required")
	case utf8.RuneCountInString(value) > maxLen:
		fe.add(field, "is too long")
	}
}

type vehicleJSON struct {
	ID            string `json:"id"`
	VehicleNumber string `json:"vehicle_number"`
	DriverName    string `json:"driver_name"`
	VehicleType   string `json:"vehicle_type"`
	Phone         string `json:"phone"`
	Status        string `json:"status"`
	CreatedAt     string `json:"created_at"`
}

type listVehiclesResponse struct {
	Vehicles []vehicleJSON `json:"vehicles"`
	Timing
}

// GET /vehicles
func (a *API) listVehicles(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	fe := fieldErrors{}
	page := parsePage(r.URL.Query(), fe)
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}
	list, err := a.store.ListVehicles(r.Context(), page)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	out := make([]vehicleJSON, len(list))
	for i, v := range list {
		out[i] = vehicleJSON{
			ID: v.ID, VehicleNumber: v.VehicleNumber, DriverName: v.DriverName, VehicleType: v.VehicleType,
			Phone: v.Phone, Status: v.Status, CreatedAt: store.FormatTime(v.CreatedAt),
		}
	}
	writeJSON(w, http.StatusOK, start, &listVehiclesResponse{Vehicles: out})
}
