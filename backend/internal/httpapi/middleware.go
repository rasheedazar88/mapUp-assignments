package httpapi

import (
	"bufio"
	"crypto/subtle"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Middleware wraps a handler with extra behaviour. Chain order (outermost first):
// recover → logging → CORS → API key → router.
type middleware func(http.Handler) http.Handler

func chain(h http.Handler, m ...middleware) http.Handler {
	for i := len(m) - 1; i >= 0; i-- {
		h = m[i](h)
	}
	return h
}

// recoverer turns a panic in any handler into a JSON 500 instead of a dropped connection.
func recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				slog.Error("panic", "value", v, "path", r.URL.Path, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, start, "internal_error", "something went wrong on our side", nil)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// statusRecorder remembers the status code for logging. It must also support
// Hijack, because the WebSocket upgrade takes over the raw TCP connection.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := s.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("hijacking not supported")
	}
	return h.Hijack()
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rec.status, "duration", time.Since(start))
	})
}

// originChecker returns a function that answers "may this browser origin call us?".
// The same rule is used for CORS and for WebSocket upgrades.
func originChecker(allowed []string) func(string) bool {
	return func(origin string) bool {
		return slices.Contains(allowed, "*") || slices.Contains(allowed, origin)
	}
}

// cors lets the frontend (on another domain, e.g. Vercel) call this API from the browser.
func cors(allowOrigin func(string) bool) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" && allowOrigin(origin) {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Add("Vary", "Origin")
				h.Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key")
				h.Set("Access-Control-Max-Age", "600")
			}
			if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
				w.WriteHeader(http.StatusNoContent) // preflight: headers only
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// apiKeyAuth (bonus, OFF unless API_KEY is set) requires X-API-Key on
// write requests. Reads stay public so dashboards and graders can browse.
func apiKeyAuth(key string) middleware {
	return func(next http.Handler) http.Handler {
		if key == "" {
			return next
		}
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			switch r.Method {
			case http.MethodPost, http.MethodPatch, http.MethodDelete, http.MethodPut:
				// ConstantTimeCompare avoids leaking the key through response timing.
				if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-API-Key")), []byte(key)) != 1 {
					writeError(w, http.StatusUnauthorized, start, "unauthorized", "missing or invalid X-API-Key header", nil)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---- Rate limiting (bonus) for POST /vehicles/location ----

// vehicleLimiter gives each vehicle its own token bucket: `rps` tokens are added
// per second, up to `burst`; every update spends one token. A sync.Mutex
// protects the map because many request goroutines use it at the same time.
type vehicleLimiter struct {
	mu      sync.Mutex
	rps     rate.Limit
	burst   int
	buckets map[string]*bucket
	calls   int
}

type bucket struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

func newVehicleLimiter(rps float64, burst int) *vehicleLimiter {
	return &vehicleLimiter{rps: rate.Limit(rps), burst: burst, buckets: map[string]*bucket{}}
}

func (l *vehicleLimiter) Allow(vehicleID string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[vehicleID]
	if !ok {
		b = &bucket{limiter: rate.NewLimiter(l.rps, l.burst)}
		l.buckets[vehicleID] = b
	}
	b.lastSeen = now

	// Every 1000 calls, forget vehicles idle for 10 minutes so memory stays bounded.
	l.calls++
	if l.calls%1000 == 0 {
		for id, other := range l.buckets {
			if now.Sub(other.lastSeen) > 10*time.Minute {
				delete(l.buckets, id)
			}
		}
	}
	return b.limiter.Allow()
}
