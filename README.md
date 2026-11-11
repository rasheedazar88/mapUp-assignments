# GeoFleet: Geofencing & Real-time Alert System

A full-stack geofencing and vehicle-tracking system:

- **Backend:** Go, PostgreSQL + PostGIS, WebSocket.
- **Frontend:** React + TypeScript, Ant Design, Leaflet.

You can draw geofences on a map, register vehicles, and push location updates. Whenever a vehicle enters or exits a zone that has an alert rule, the dashboard gets an alert within milliseconds.

| | |
|---|---|
| **Live frontend** | _add Vercel URL after deploying_ |
| **Live backend** | _add backend URL after deploying_ |
| **Docker Hub image** | _add `docker.io/<user>/geofence-backend` after pushing_ |
| **Demo video** | _add link (YouTube unlisted / Loom / Drive)_ |
| **Assessment brief** | [docs/ASSESSMENT.md](docs/ASSESSMENT.md) |

## Features

- **Geofences:** draw polygons on the map (Leaflet-Geoman) or paste `[lat, lng]` coordinates. Each geofence has one of 4 categories, and the list can be filtered by category. A details drawer shows the vehicles inside and the alert rules.
- **Vehicles:** registration with validation. The list shows each vehicle's live location and the geofences it is in.
- **Location updates:** click the map or type coordinates and a timestamp. A **trip simulator** drives a vehicle along waypoints, which is handy for demos.
- **Entry/exit detection:**
  - Point-in-polygon runs in PostGIS (`ST_Covers` with a GiST index).
  - Each update compares the vehicle's previous geofences with its current ones inside one transaction, with a per-vehicle row lock, so there are no duplicate events.
- **Alert rules:** a rule is a geofence plus one vehicle or all vehicles, plus entry, exit, or both. Rules can be enabled, disabled, or deleted.
- **Real-time alerts:**
  - `/ws/alerts` uses a goroutine hub that broadcasts each alert to every connected client.
  - Delivery is asynchronous, so the HTTP response never waits on it.
  - Every alert is saved in the database first.
  - The UI shows toasts and a live feed, and reconnects automatically with backoff.
- **Violation history:** filter by vehicle, geofence, or date range, with server-side pagination (`total_count`).
- **Every API response includes `time_ns`.** Error responses and 404/405 responses include it too.
- **Bonus items:**
  - Per-vehicle rate limiting on location updates.
  - Optional API-key auth.
  - Optional `limit`/`offset` pagination on all list endpoints.
  - Unit and integration tests.

## Quick start

```bash
docker compose up --build          # PostGIS + Go API on http://localhost:8080
bash scripts/seed.sh               # optional demo data (Git Bash / macOS / Linux)
cd frontend && npm install && npm run dev   # UI on http://localhost:5173
```

You can also run everything in Docker, including the UI on http://localhost:3000:

```bash
docker compose --profile full up --build
```

Full instructions, the curl API guide and a frontend walkthrough are in **[SETUP.md](SETUP.md)**.

## Documentation

- [SETUP.md](SETUP.md): prerequisites, local setup, Docker Compose, testing, curl examples, frontend usage, deployment.
- [docs/API.md](docs/API.md): every endpoint, its request/response and error formats, and the WebSocket protocol.
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md): design, data model, geospatial logic, concurrency model, scaling notes.
- [docs/PLAN.md](docs/PLAN.md): requirement checklist and implementation status.

## Repository layout

```
backend/    Go API (cmd/server, internal/{httpapi,store,geo,tracking,realtime,db,config,ids})
frontend/   React + TS + Vite app (src/{api,pages,components,hooks,store,utils})
scripts/    seed.sh (demo data via the public API)
docs/       assessment brief, API, architecture, plan
docker-compose.yml, render.yaml, .env.example
```

## Tests

```bash
cd backend && TEST_DATABASE_URL=postgres://geofence:geofence@localhost:5433/geofence?sslmode=disable go test ./...
cd frontend && npx vitest run
```

Backend tests cover all 9 README endpoints against a real PostGIS. They include:
- Validation and errors.
- Point-in-polygon edge cases: vertex, edge, concave polygons, and overlapping zones.
- Entry/exit, out-of-order timestamps, and concurrent updates.
- Alert rule matching.
- WebSocket broadcast, disconnect, and reconnect.
- `time_ns` on every response.
- CORS, the API key, and the rate limit.

The suite also passes `go test -race`.
