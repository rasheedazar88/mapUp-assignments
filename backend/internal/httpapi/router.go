// Package httpapi is the HTTP layer: routing, request validation, and JSON responses.
package httpapi

import (
	"context"
	"net/http"
	"time"

	"geofence/internal/realtime"
	"geofence/internal/store"
)

type Options struct {
	AllowedOrigins []string
	APIKey         string
	RateLimitRPS   float64
	RateLimitBurst int
	Ping           func(ctx context.Context) error // database health check
}

type API struct {
	store   *store.Store
	hub     *realtime.Hub
	limiter *vehicleLimiter
	ping    func(ctx context.Context) error
}

// NewHandler builds the complete HTTP handler.
//
// Go 1.22+ http.ServeMux understands "METHOD /path/{param}" patterns, so the
// route table below reads exactly like the README's endpoint list.
func NewHandler(st *store.Store, hub *realtime.Hub, opt Options) http.Handler {
	api := &API{
		store:   st,
		hub:     hub,
		limiter: newVehicleLimiter(opt.RateLimitRPS, opt.RateLimitBurst),
		ping:    opt.Ping,
	}
	allowOrigin := originChecker(opt.AllowedOrigins)

	mux := http.NewServeMux()

	// ---- The 9 endpoints required by the README (paths and methods unchanged) ----
	mux.HandleFunc("POST /geofences", api.createGeofence)                         // 1
	mux.HandleFunc("GET /geofences", api.listGeofences)                           // 2
	mux.HandleFunc("POST /vehicles", api.createVehicle)                           // 3
	mux.HandleFunc("GET /vehicles", api.listVehicles)                             // 4
	mux.HandleFunc("POST /vehicles/location", api.updateLocation)                 // 5
	mux.HandleFunc("GET /vehicles/location/{vehicle_id}", api.getVehicleLocation) // 6
	mux.HandleFunc("POST /alerts/configure", api.configureAlert)                  // 7
	mux.HandleFunc("GET /alerts", api.listAlerts)                                 // 8
	mux.HandleFunc("GET /violations/history", api.violationHistory)               // 9

	// ---- Real-time alerts ----
	mux.HandleFunc("GET /ws/alerts", realtime.Handler(hub, allowOrigin))

	// ---- Extras (additive; nothing above depends on them) ----
	mux.HandleFunc("GET /health", api.health)
	mux.HandleFunc("GET /geofences/{geofence_id}", api.getGeofence)
	mux.HandleFunc("GET /vehicles/locations", api.listVehicleLocations)
	mux.HandleFunc("PATCH /alerts/{alert_id}", api.updateAlertStatus)
	mux.HandleFunc("DELETE /alerts/{alert_id}", api.deleteAlert)
	mux.HandleFunc("GET /alerts/events", api.listFiredAlerts)

	return chain(jsonFallback(mux),
		recoverer,
		logger,
		cors(allowOrigin),
		apiKeyAuth(opt.APIKey),
	)
}

// jsonFallback makes unknown paths (404) and wrong methods (405) answer in the
// same JSON format with time_ns, instead of the mux's plain-text defaults.
func jsonFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// No route matched. Let the mux decide 404 vs 405 (and set the Allow header),
		// capture the code, then write our JSON body instead of its text body.
		probe := &statusProbe{header: http.Header{}}
		mux.ServeHTTP(probe, r)
		if allow := probe.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		if probe.status == http.StatusMethodNotAllowed {
			writeError(w, http.StatusMethodNotAllowed, start, "method_not_allowed", r.Method+" is not allowed on "+r.URL.Path, nil)
			return
		}
		writeError(w, http.StatusNotFound, start, "not_found", "no endpoint "+r.Method+" "+r.URL.Path, nil)
	})
}

// statusProbe is a throw-away ResponseWriter that only records the status code.
type statusProbe struct {
	header http.Header
	status int
}

func (p *statusProbe) Header() http.Header         { return p.header }
func (p *statusProbe) Write(b []byte) (int, error) { return len(b), nil }
func (p *statusProbe) WriteHeader(code int)        { p.status = code }

type healthResponse struct {
	Status    string `json:"status"`
	Database  string `json:"database"`
	WSClients int    `json:"ws_clients"`
	Timing
}

// GET /health (extra): used by Docker/hosting health checks.
func (a *API) health(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := a.ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, start, &healthResponse{Status: "degraded", Database: "unreachable", WSClients: a.hub.ClientCount()})
		return
	}
	writeJSON(w, http.StatusOK, start, &healthResponse{Status: "ok", Database: "ok", WSClients: a.hub.ClientCount()})
}
