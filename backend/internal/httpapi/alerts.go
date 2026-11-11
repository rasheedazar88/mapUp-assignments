package httpapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"geofence/internal/store"
)

var eventTypes = []string{"entry", "exit", "both"}

type configureAlertRequest struct {
	GeofenceID string  `json:"geofence_id"`
	VehicleID  *string `json:"vehicle_id"` // optional: omitted/null/"" = all vehicles
	EventType  string  `json:"event_type"`
}

type configureAlertResponse struct {
	AlertID    string  `json:"alert_id"`
	GeofenceID string  `json:"geofence_id"`
	VehicleID  *string `json:"vehicle_id"` // null for "all vehicles"
	EventType  string  `json:"event_type"`
	Status     string  `json:"status"`
	Timing
}

// POST /alerts/configure
func (a *API) configureAlert(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req configureAlertRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadJSON(w, start, err)
		return
	}
	req.GeofenceID = strings.TrimSpace(req.GeofenceID)
	req.EventType = strings.ToLower(strings.TrimSpace(req.EventType))
	if req.VehicleID != nil {
		trimmed := strings.TrimSpace(*req.VehicleID)
		if trimmed == "" {
			req.VehicleID = nil
		} else {
			req.VehicleID = &trimmed
		}
	}

	fe := fieldErrors{}
	if req.GeofenceID == "" {
		fe.add("geofence_id", "is required")
	}
	switch {
	case req.EventType == "":
		fe.add("event_type", "is required")
	case !slices.Contains(eventTypes, req.EventType):
		fe.add("event_type", "must be one of: entry, exit, both")
	}
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	rule, err := a.store.CreateAlertRule(r.Context(), store.CreateAlertRuleInput{
		GeofenceID: req.GeofenceID, VehicleID: req.VehicleID, EventType: req.EventType,
	})
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusCreated, start, &configureAlertResponse{
		AlertID: rule.ID, GeofenceID: rule.GeofenceID, VehicleID: rule.VehicleID, EventType: rule.EventType, Status: rule.Status,
	})
}

type alertRuleJSON struct {
	AlertID       string  `json:"alert_id"`
	GeofenceID    string  `json:"geofence_id"`
	GeofenceName  string  `json:"geofence_name"`
	VehicleID     *string `json:"vehicle_id"`
	VehicleNumber *string `json:"vehicle_number"`
	EventType     string  `json:"event_type"`
	Status        string  `json:"status"`
	CreatedAt     string  `json:"created_at"`
}

func toAlertRuleJSON(r store.AlertRule) alertRuleJSON {
	return alertRuleJSON{
		AlertID: r.ID, GeofenceID: r.GeofenceID, GeofenceName: r.GeofenceName, VehicleID: r.VehicleID,
		VehicleNumber: r.VehicleNumber, EventType: r.EventType, Status: r.Status, CreatedAt: store.FormatTime(r.CreatedAt),
	}
}

type listAlertsResponse struct {
	Alerts []alertRuleJSON `json:"alerts"`
	Timing
}

// GET /alerts?geofence_id=&vehicle_id=
func (a *API) listAlerts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	q := r.URL.Query()
	fe := fieldErrors{}
	page := parsePage(q, fe)
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}
	rules, err := a.store.ListAlertRules(r.Context(),
		strings.TrimSpace(q.Get("geofence_id")), strings.TrimSpace(q.Get("vehicle_id")), page)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	out := make([]alertRuleJSON, len(rules))
	for i, rule := range rules {
		out[i] = toAlertRuleJSON(rule)
	}
	writeJSON(w, http.StatusOK, start, &listAlertsResponse{Alerts: out})
}

type alertRuleResponse struct {
	alertRuleJSON
	Timing
}

// PATCH /alerts/{alert_id} (extra) body: {"status": "active" | "inactive"}
func (a *API) updateAlertStatus(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	var req struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadJSON(w, start, err)
		return
	}
	if req.Status != "active" && req.Status != "inactive" {
		writeValidation(w, start, fieldErrors{"status": "must be active or inactive"})
		return
	}
	rule, err := a.store.SetAlertRuleStatus(r.Context(), r.PathValue("alert_id"), req.Status)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusOK, start, &alertRuleResponse{alertRuleJSON: toAlertRuleJSON(rule)})
}

type deleteAlertResponse struct {
	AlertID string `json:"alert_id"`
	Deleted bool   `json:"deleted"`
	Timing
}

// DELETE /alerts/{alert_id} (extra)
func (a *API) deleteAlert(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	id := r.PathValue("alert_id")
	if err := a.store.DeleteAlertRule(r.Context(), id); err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusOK, start, &deleteAlertResponse{AlertID: id, Deleted: true})
}

type firedAlertsResponse struct {
	Events []store.AlertMessage `json:"events"`
	Timing
}

// GET /alerts/events?limit=50 (extra): recently fired alerts in the exact
// WebSocket message format. The UI uses it to backfill the live feed.
func (a *API) listFiredAlerts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	limit := 50
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 200 {
			writeValidation(w, start, fieldErrors{"limit": "must be an integer between 1 and 200"})
			return
		}
		limit = n
	}
	events, err := a.store.ListFiredAlerts(r.Context(), limit)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusOK, start, &firedAlertsResponse{Events: events})
}
