# SETUP

## 1. Prerequisites

| Tool | Version used | Needed for |
|---|---|---|
| Docker + Docker Compose | 29 / v2.40 | running PostGIS and the backend |
| Go | 1.27 | running or testing the backend without Docker |
| Node.js + npm | 24 / 11 | the frontend |
| Git Bash, or any bash | – | `scripts/seed.sh` and the curl examples (Windows: Git Bash) |

Ports used: `8080` (API), `5433` (PostGIS on the host, which avoids clashing with a local PostgreSQL on 5432), `5173` (Vite dev server), `3000` (Dockerized frontend, optional).

## 2. Run with Docker Compose (recommended)

```bash
git clone <repo-url> && cd geofencing-real-time-alert-system
cp .env.example .env            # optional: every value has a default
docker compose up --build       # db + backend
```

- PostGIS starts first. The backend waits for it, runs migrations automatically (they are embedded in the binary), then listens on **http://localhost:8080**.
- Check that it's up: `curl http://localhost:8080/health` should return `{"status":"ok","database":"ok",...,"time_ns":"..."}`.
- Optional demo data: `bash scripts/seed.sh` loads 4 geofences, 3 vehicles and 3 alert rules.
- Run everything in containers, including the UI at http://localhost:3000: `docker compose --profile full up --build`
- Stop: `docker compose down`. To also wipe the database, add `-v`: `docker compose down -v`.

> **Windows note:** if you have a global `ALLOWED_ORIGINS` environment variable from another project, it does not affect this stack. Compose reads `CORS_ALLOWED_ORIGINS` instead.

## 3. Local development (without the backend container)

```bash
docker compose up -d db                       # only PostGIS

# backend
cd backend
export DATABASE_URL="postgres://geofence:geofence@localhost:5433/geofence?sslmode=disable"
export ALLOWED_ORIGINS="http://localhost:5173"
go run ./cmd/server                           # http://localhost:8080

# frontend (second terminal)
cd frontend
cp .env.example .env.local                    # VITE_API_URL=http://localhost:8080
npm install
npm run dev                                   # http://localhost:5173
```

PowerShell uses `$env:DATABASE_URL="..."` instead of `export`.

### Environment variables (backend)

| Variable | Default | Meaning |
|---|---|---|
| `DATABASE_URL` | **required** | PostgreSQL URL. The DB must allow `CREATE EXTENSION postgis`. |
| `PORT` | `8080` | HTTP port |
| `ALLOWED_ORIGINS` | `http://localhost:5173` | Comma-separated CORS/WebSocket origins; `*` allows any |
| `API_KEY` | _(empty = off)_ | When set, POST/PATCH/DELETE need `X-API-Key` |
| `RATE_LIMIT_RPS` / `RATE_LIMIT_BURST` | `5` / `10` | Location updates per second per vehicle |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` (JSON logs) |

Frontend: `VITE_API_URL` (the backend base URL; the WebSocket URL is derived from it) and an optional `VITE_API_KEY`.

## 4. Tests

```bash
# backend unit + integration tests (needs PostGIS running, e.g. `docker compose up -d db`)
cd backend
TEST_DATABASE_URL="postgres://geofence:geofence@localhost:5433/geofence?sslmode=disable" go test ./...
# race detector (needs cgo; on Windows the easiest way is inside a Linux container):
docker run --rm -v "$PWD:/src" -w /src -e TEST_DATABASE_URL="postgres://geofence:geofence@host.docker.internal:5433/geofence?sslmode=disable" golang:1.27 go test -race ./...

# frontend unit tests, type check, lint
cd frontend
npx vitest run && npx tsc -b && npx oxlint
```

Each test run creates its own schema (`test_xxxxxxxx`) and drops it afterwards, so your dev data is untouched. Without `TEST_DATABASE_URL`, the integration tests are skipped and only the pure unit tests run.

## 5. API testing guide (curl)

The examples use bash. Each response includes `time_ns`, the handler's execution time in nanoseconds.

```bash
API=http://localhost:8080

# 1. Create a geofence: coordinates are [latitude, longitude], first == last
curl -s -X POST $API/geofences -H 'Content-Type: application/json' -d '{
  "name": "Downtown Delivery Zone",
  "description": "Main delivery area for downtown customers",
  "coordinates": [[37.7749,-122.4194],[37.7849,-122.4194],[37.7849,-122.4094],[37.7749,-122.4094],[37.7749,-122.4194]],
  "category": "delivery_zone"
}'
# → 201 {"id":"geo_…","name":"Downtown Delivery Zone","status":"active","time_ns":"…"}

# 2. List geofences (optional ?category=)
curl -s "$API/geofences?category=delivery_zone"

# 3. Register a vehicle
curl -s -X POST $API/vehicles -H 'Content-Type: application/json' -d '{
  "vehicle_number": "KA-01-AB-1234", "driver_name": "John Doe", "vehicle_type": "truck", "phone": "+1234567890"
}'
# → 201 {"id":"veh_…","vehicle_number":"KA-01-AB-1234","status":"active","time_ns":"…"}

# 4. List vehicles
curl -s $API/vehicles

GEO=geo_xxxxxxxxxx   # paste the ids from above
VEH=veh_xxxxxxxxxx

# 7. Configure an alert rule (omit vehicle_id to apply to ALL vehicles)
curl -s -X POST $API/alerts/configure -H 'Content-Type: application/json' \
  -d "{\"geofence_id\":\"$GEO\",\"vehicle_id\":\"$VEH\",\"event_type\":\"entry\"}"

# 8. List alert rules (optional ?geofence_id= &vehicle_id=)
curl -s "$API/alerts?geofence_id=$GEO"

# 5. Update the location: inside the zone → "entry" event + WebSocket alert
curl -s -X POST $API/vehicles/location -H 'Content-Type: application/json' \
  -d "{\"vehicle_id\":\"$VEH\",\"latitude\":37.7849,\"longitude\":-122.4194,\"timestamp\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\"}"
# → {"vehicle_id":"veh_…","location_updated":true,"current_geofences":[{"geofence_id":"geo_…","geofence_name":"Downtown Delivery Zone","status":"inside"}],"time_ns":"…"}

# 6. Current location + geofence status
curl -s $API/vehicles/location/$VEH

# 9. Violation history (all filters optional; limit default 50, max 500)
curl -s "$API/violations/history?vehicle_id=$VEH&limit=100"
curl -s "$API/violations/history?geofence_id=$GEO&start_date=2025-01-01T00:00:00Z&end_date=2030-01-01T00:00:00Z"
```

**Watch the WebSocket from a terminal** (any client works, for example `npx wscat`):

```bash
npx wscat -c ws://localhost:8080/ws/alerts
# then send a location update that crosses a configured geofence from another terminal
```

**Error examples:**

```bash
curl -s -X POST $API/geofences -H 'Content-Type: application/json' -d '{"name":"x","category":"toll_zone","coordinates":[[1,2],[3,4],[1,2]]}'
# → 400 {"error":{"code":"validation_error","message":"coordinates: polygon needs at least 4 points …","details":{…}},"time_ns":"…"}
curl -s $API/vehicles/location/veh_unknown     # → 404 not_found
curl -s -X DELETE $API/geofences               # → 405 method_not_allowed (JSON + time_ns)
```

The complete reference is in [docs/API.md](docs/API.md).

## 6. Frontend usage guide

| Page | What you can do |
|---|---|
| **Dashboard** | Totals (vehicles, geofences, active rules, events in the last 24h), live map, vehicle status breakdown, recent violations, latest alerts |
| **Geofences** | Search/filter by category; click a row or polygon for details. **New geofence:** pick the polygon tool (top-left of the map), click the corners, then click the first corner to close the shape. You can drag the corners afterwards. Or type/paste coordinates (*Use example* fills in the README polygon). For a *Restricted zone*, a checkbox also creates an "all vehicles / entry" alert rule. |
| **Vehicles** | Register a vehicle; the table shows the current location and the geofences it is in (refreshes automatically). Click a row for details and recent events; **Move** opens Live Tracking with that vehicle selected. |
| **Live Tracking** | Pick a vehicle, **click the map** (or type the lat/lng), choose "use current time" or a custom timestamp, then **Send location update**. The result card shows `current_geofences` and what was entered or exited. **Simulate trip:** click waypoints, then *Start trip* to send an update every interval. |
| **Alert Rules** | Create a rule (geofence, vehicle or *All vehicles*, entry/exit/both). Filter, enable/disable or delete rules. |
| **Live Alerts** | Real-time feed over WebSocket, with a toast for every alert. Click an alert to see it on the map. The header shows the connection status (*Live* or *Reconnecting…*) and an unread badge. |
| **Violation History** | Filter by vehicle, geofence or date-time range; server-side pagination; 📍 shows the event on the map. |

**Suggested demo flow:**
1. Create a restricted zone by drawing it on the map.
2. Register a vehicle.
3. On Live Tracking, click inside the zone and send the update. A red toast appears, and the alert shows up in Live Alerts.
4. Click outside the zone. The exit is recorded in history; it is only alerted if the rule includes exits.
5. Filter Violation History by that vehicle.

## 7. Deployment

### Backend image → Docker Hub

```bash
docker login
docker build -t <dockerhub-user>/geofence-backend:1.0.0 -t <dockerhub-user>/geofence-backend:latest ./backend
docker push <dockerhub-user>/geofence-backend:1.0.0
docker push <dockerhub-user>/geofence-backend:latest
```

### Backend → Render (any Docker host works)

1. Push the repo to GitHub (private).
2. Render: **New → Blueprint**, then select the repo. `render.yaml` creates the web service (Docker, `backend/`) and a Postgres database, and wires up `DATABASE_URL`. Render Postgres supports PostGIS, and the first migration enables it.
3. Once the frontend is deployed, set `ALLOWED_ORIGINS=https://<your-app>.vercel.app` on the service.
4. Check it: `curl https://<backend>.onrender.com/health`.

Free Render instances sleep when idle. Open `/health` once before a demo to wake the instance.

Alternatives: Railway, Fly.io, or a VM running `docker compose up -d`. The only requirement is a PostgreSQL that has PostGIS (Neon and Supabase work).

### Frontend → Vercel

1. Vercel: **Add New → Project**, import the repo, and set **Root Directory = `frontend`**. `vercel.json` provides the build command and SPA rewrites.
2. Environment variable: `VITE_API_URL=https://<backend-host>` (https; the app derives `wss://…/ws/alerts` automatically).
3. Deploy, then add the Vercel URL to the backend's `ALLOWED_ORIGINS`.

### Submission checklist

- [ ] Private GitHub repo, with the 9 collaborators from the brief added
- [ ] Backend image pushed to Docker Hub
- [ ] Backend and frontend deployed; URLs added to README.md
- [ ] Demo video link added to README.md
- [ ] Google form submitted
