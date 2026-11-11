package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"geofence/internal/store"
)

// Timing is embedded in every response struct. Go's JSON encoder "promotes"
// the fields of an embedded struct, so {"...": ..., "time_ns": "123"} comes out flat.
type Timing struct {
	TimeNS string `json:"time_ns"`
}

func (t *Timing) setTimeNS(ns string) { t.TimeNS = ns }

// timed is satisfied by any struct that embeds Timing (Go interfaces are
// satisfied implicitly: no "implements" keyword needed).
type timed interface{ setTimeNS(string) }

// writeJSON stamps time_ns and writes the response.
//
// Every handler's first line is `start := time.Now()`. time.Since(start) here,
// right before encoding, measures the handler's whole execution time
// (validation + database work). This is the README's "start and end of each
// request handler". The value is a string of nanoseconds, as the README specifies.
func writeJSON(w http.ResponseWriter, status int, start time.Time, body timed) {
	body.setTimeNS(strconv.FormatInt(time.Since(start).Nanoseconds(), 10))
	buf, err := json.Marshal(body)
	if err != nil {
		slog.Error("encode response", "err", err)
		http.Error(w, `{"error":{"code":"internal_error","message":"failed to encode response"},"time_ns":"0"}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(buf)
}

// ---- Errors ----
// One JSON shape for every error:
//   {"error": {"code": "validation_error", "message": "...", "details": {...}}, "time_ns": "..."}

type apiError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Details map[string]any `json:"details,omitempty"`
}

type errorResponse struct {
	Error apiError `json:"error"`
	Timing
}

func writeError(w http.ResponseWriter, status int, start time.Time, code, message string, details map[string]any) {
	writeJSON(w, status, start, &errorResponse{Error: apiError{Code: code, Message: message, Details: details}})
}

// fieldErrors collects validation problems so the client sees ALL of them at once.
type fieldErrors map[string]string

func (fe fieldErrors) add(field, msg string) {
	if _, exists := fe[field]; !exists {
		fe[field] = msg
	}
}

// writeValidation writes a 400 listing every invalid field.
func writeValidation(w http.ResponseWriter, start time.Time, fe fieldErrors) {
	parts := make([]string, 0, len(fe))
	fields := make(map[string]any, len(fe))
	for f, m := range fe {
		parts = append(parts, f+": "+m)
		fields[f] = m
	}
	sort.Strings(parts)
	writeError(w, http.StatusBadRequest, start, "validation_error", strings.Join(parts, "; "), map[string]any{"fields": fields})
}

// writeStoreError maps data-layer errors to HTTP status codes in ONE place
// (centralized error handling). Unknown errors become a generic 500: the real
// error is logged, never sent to the client.
func writeStoreError(w http.ResponseWriter, r *http.Request, start time.Time, err error) {
	var nf *store.NotFoundError
	var conflict *store.ConflictError
	var validation *store.ValidationError
	switch {
	case errors.As(err, &nf):
		writeError(w, http.StatusNotFound, start, "not_found", nf.Resource+" not found", map[string]any{"id": nf.ID})
	case errors.As(err, &conflict):
		writeError(w, http.StatusConflict, start, "conflict", conflict.Message, conflict.Details)
	case errors.As(err, &validation):
		writeError(w, http.StatusBadRequest, start, "validation_error", validation.Error(),
			map[string]any{"fields": map[string]any{validation.Field: validation.Message}})
	case errors.Is(err, r.Context().Err()) && r.Context().Err() != nil:
		// Client went away / request timed out. 499-style; nobody is listening.
		writeError(w, http.StatusServiceUnavailable, start, "request_cancelled", "request was cancelled", nil)
	default:
		slog.Error("internal error", "method", r.Method, "path", r.URL.Path, "err", err)
		writeError(w, http.StatusInternalServerError, start, "internal_error", "something went wrong on our side", nil)
	}
}

// ---- Request decoding ----

const maxBodyBytes = 1 << 20 // 1 MiB is plenty for a polygon with thousands of points

// decodeJSON reads the body into dst and turns Go's decoder errors into
// messages a user can act on.
func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(dst)
	if err == nil {
		if dec.More() {
			return errors.New("request body must contain a single JSON object")
		}
		return nil
	}

	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var maxErr *http.MaxBytesError
	switch {
	case errors.Is(err, io.EOF):
		return errors.New("request body is required")
	case errors.As(err, &syntaxErr):
		return fmt.Errorf("malformed JSON at position %d", syntaxErr.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return errors.New("malformed JSON")
	case errors.As(err, &typeErr):
		if typeErr.Field != "" {
			return fmt.Errorf("field %q must be of type %s", typeErr.Field, jsonTypeName(typeErr.Type.String()))
		}
		return errors.New("request body must be a JSON object")
	case errors.As(err, &maxErr):
		return fmt.Errorf("request body must not exceed %d bytes", maxBodyBytes)
	default:
		return err
	}
}

func jsonTypeName(goType string) string {
	switch {
	case strings.HasPrefix(goType, "float"), strings.HasPrefix(goType, "*float"):
		return "number"
	case strings.Contains(goType, "string"):
		return "string"
	case strings.HasPrefix(goType, "[]"):
		return "array"
	default:
		return goType
	}
}

func writeBadJSON(w http.ResponseWriter, start time.Time, err error) {
	writeError(w, http.StatusBadRequest, start, "invalid_json", err.Error(), nil)
}
