package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"geofence/internal/geo"
	"geofence/internal/store"
)

var geofenceCategories = []string{"delivery_zone", "restricted_zone", "toll_zone", "customer_area"}

type createGeofenceRequest struct {
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Coordinates [][]float64 `json:"coordinates"`
	Category    string      `json:"category"`
}

type createGeofenceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
	Timing
}

// POST /geofences
func (a *API) createGeofence(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	var req createGeofenceRequest
	if err := decodeJSON(w, r, &req); err != nil {
		writeBadJSON(w, start, err)
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	req.Description = strings.TrimSpace(req.Description)
	req.Category = strings.TrimSpace(req.Category)

	fe := fieldErrors{}
	switch {
	case req.Name == "":
		fe.add("name", "is required")
	case utf8.RuneCountInString(req.Name) > 120:
		fe.add("name", "must be at most 120 characters")
	}
	if utf8.RuneCountInString(req.Description) > 1000 {
		fe.add("description", "must be at most 1000 characters")
	}
	switch {
	case req.Category == "":
		fe.add("category", "is required")
	case !slices.Contains(geofenceCategories, req.Category):
		fe.add("category", "must be one of: "+strings.Join(geofenceCategories, ", "))
	}
	if req.Coordinates == nil {
		fe.add("coordinates", "is required")
	} else if err := geo.ValidatePolygon(req.Coordinates); err != nil {
		fe.add("coordinates", err.Error())
	}
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	g, err := a.store.CreateGeofence(r.Context(), store.CreateGeofenceInput{
		Name: req.Name, Description: req.Description, Category: req.Category, Coordinates: req.Coordinates,
	})
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusCreated, start, &createGeofenceResponse{ID: g.ID, Name: g.Name, Status: g.Status})
}

type geofenceJSON struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Description string      `json:"description"`
	Coordinates [][]float64 `json:"coordinates"`
	Category    string      `json:"category"`
	Status      string      `json:"status"`
	CreatedAt   string      `json:"created_at"`
}

func toGeofenceJSON(g store.Geofence) geofenceJSON {
	return geofenceJSON{
		ID: g.ID, Name: g.Name, Description: g.Description, Coordinates: g.Coordinates,
		Category: g.Category, Status: g.Status, CreatedAt: store.FormatTime(g.CreatedAt),
	}
}

type listGeofencesResponse struct {
	Geofences []geofenceJSON `json:"geofences"`
	Timing
}

// GET /geofences?category=delivery_zone
func (a *API) listGeofences(w http.ResponseWriter, r *http.Request) {
	start := time.Now()

	q := r.URL.Query()
	category := strings.TrimSpace(q.Get("category"))
	fe := fieldErrors{}
	if category != "" && !slices.Contains(geofenceCategories, category) {
		fe.add("category", "must be one of: "+strings.Join(geofenceCategories, ", "))
	}
	page := parsePage(q, fe)
	if len(fe) > 0 {
		writeValidation(w, start, fe)
		return
	}

	list, err := a.store.ListGeofences(r.Context(), category, page)
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	out := make([]geofenceJSON, len(list))
	for i, g := range list {
		out[i] = toGeofenceJSON(g)
	}
	writeJSON(w, http.StatusOK, start, &listGeofencesResponse{Geofences: out})
}

type getGeofenceResponse struct {
	geofenceJSON
	Timing
}

// GET /geofences/{geofence_id} (extra)
func (a *API) getGeofence(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	g, err := a.store.GetGeofence(r.Context(), r.PathValue("geofence_id"))
	if err != nil {
		writeStoreError(w, r, start, err)
		return
	}
	writeJSON(w, http.StatusOK, start, &getGeofenceResponse{geofenceJSON: toGeofenceJSON(g)})
}
