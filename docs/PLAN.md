# Implementation Plan & Requirement Checklist

Source of truth: [ASSESSMENT.md](ASSESSMENT.md) (the original assessment README).

Legend:
- `[x]` = implemented **and verified** (automated test or recorded manual check)
- `[ ]` = not done
- `(you)` = needs your accounts or actions
- `(extra)` = additive, beyond the brief

Last updated: 2026-11-11.

## Cross-cutting API rules
- [x] Every response has `time_ns` (a string of nanoseconds), including errors, 404, 405 and panics. Measured from handler start to end with `time.Now()`/`time.Since`. *Test: `TestEveryResponseHasTimeNS`; the test harness asserts it on every call.*
- [x] Paths, methods and field names exactly as in the brief. *Route table: `backend/internal/httpapi/router.go`; tests assert response keys.*
- [x] Proper status codes: 201/200/400/401/404/405/409/429/500.
- [x] One centralized JSON error shape (`writeStoreError`, `writeValidation`).
- [x] Parameterized SQL only.
- [x] CORS allow-list from env, shared with the WebSocket origin check. *`TestCORS`*
- [x] Config via environment variables.

## Endpoints
| # | Endpoint | Status | Tests |
|---|---|---|---|
| 1 | `POST /geofences` | [x] | `TestGeofences`, `TestGeofenceValidation` (11 cases including self-intersection) |
| 2 | `GET /geofences?category=` | [x] | `TestGeofences` (filter, invalid category, round trip of `[lat,lng]`) |
| 3 | `POST /vehicles` | [x] | `TestVehicles` (duplicate 409 case-insensitive, validation) |
| 4 | `GET /vehicles` | [x] | `TestVehicles` |
| 5 | `POST /vehicles/location` | [x] | `TestLocationEntryExitAndHistory`, `TestOverlappingGeofencesAndConcavePolygon`, `TestLocationValidationAndErrors`, `TestConcurrentUpdatesProduceSingleEntry`, `TestRateLimit` |
| 6 | `GET /vehicles/location/{vehicle_id}` | [x] | `TestLocationEntryExitAndHistory` (null before the first update, 404) |
| 7 | `POST /alerts/configure` | [x] | `TestAlertConfiguration` (all-vehicles → null, 404s, 409, 400) |
| 8 | `GET /alerts` | [x] | `TestAlertConfiguration` (geofence/vehicle filters) |
| 9 | `GET /violations/history` | [x] | `TestLocationEntryExitAndHistory` (filters, dates, limit clamp, total_count, 400s) |

## Real-time alerts
- [x] `/ws/alerts` with Gorilla WebSocket; multiple concurrent clients. *`TestHubBroadcastAndLifecycle` (20 clients), `TestRealtimeAlerts`.*
- [x] Lifecycle: connect, disconnect cleanup, reconnect, ping/pong heartbeat, graceful shutdown.
- [x] Exact README message format (exactly 6 top-level keys, checked in tests).
- [x] Only events with a matching **active** rule are broadcast; at most one alert per event.
- [x] Async delivery: `Publish` never blocks. *`TestPublishNeverBlocks`*
- [x] All alerts persisted (`alerts` table, `GET /alerts/events`).
- [x] `go test -race ./...` passes (run in the golang:1.27 container).

## Frontend
- [x] Geofence create: name, description, category, coordinates, drawn on the map (Geoman) **or** typed. *Browser E2E: drew a polygon, created it.*
- [x] Geofence list with category filter, polygons on the map, details drawer.
- [x] Vehicle register, list, current location + geofence status per vehicle, details drawer (uses endpoint 6).
- [x] Location update by map click **and** by inputs, plus a timestamp; shows the current geofences and what was entered/exited. Trip simulator (extra). *E2E verified.*
- [x] Alert rules: create (geofence, optional vehicle, event type), list, filter, enable/disable, delete.
- [x] Live alerts over WebSocket: toast and feed with vehicle, geofence name and category, event type, timestamp, location; map view; reconnect with backfill. *E2E: toast received within ~1 s, also against the Docker-served frontend.*
- [x] Violation history: filters for vehicle, geofence and date range; server-side pagination with `total_count`.
- [x] Dashboard: totals, live map, vehicle status overview, recent violations, latest alerts.
- [x] Loading, empty and error states with retry; user-friendly messages; responsive layout (no horizontal overflow at 390 px; sidebar becomes a drawer on mobile).
- [x] Zero browser console errors/warnings in the E2E run. `tsc`, oxlint and vitest are clean.

## Infrastructure and docs
- [x] Backend Dockerfile (multi-stage, distroless, non-root, 13.8 MB).
- [x] `docker-compose.yml`: PostGIS, backend (migrations on start), optional frontend (`--profile full`). *Verified from a clean volume.*
- [x] `.env.example` (root and frontend).
- [x] README.md, SETUP.md (curl guide, frontend guide), docs/API.md, docs/ARCHITECTURE.md.
- [x] Deployment configs: `render.yaml` (backend + Postgres), `frontend/vercel.json`.
- [ ] (you) Push the image to Docker Hub (commands in SETUP.md §7).
- [ ] (you) Deploy the backend (Render or another host) and the frontend (Vercel); put the URLs in README.md.
- [ ] (you) Private GitHub repo + the 9 collaborators.
- [ ] (you) Record the 30–45 min video; add the link to README.md; submit the Google form.

## Bonus
- [x] Rate limiting on `POST /vehicles/location` (per-vehicle token bucket). *`TestRateLimit`*
- [x] API-key auth (optional, off by default so graders' curl calls work). *`TestAPIKey`*
- [x] Pagination (`limit`/`offset`) on all list endpoints.
- [x] Unit tests: `geo`, `tracking`, `realtime`, plus frontend `coords`.

## Decisions on the open questions
- **Q1 (restricted zones):** alerts are rule-driven. When you create a `restricted_zone` in the UI, a checkbox (on by default) also creates an "all vehicles / entry" rule, so the rule is visible and can be removed.
- **Q2:** added `GET /vehicles/locations` (bulk) for the maps.
- **Q3:** geofences can be viewed but not edited or deleted (listed as a roadmap item with the event-recomputation caveat).
- **Q4:** API key is optional and off by default.
- **Q5:** `render.yaml` provided; any Docker host works.
