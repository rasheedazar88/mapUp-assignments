# Architecture

## 1. Overview

```
 Browser (React SPA on Vercel)                     Docker host
 ┌──────────────────────────────┐   HTTPS/JSON    ┌───────────────────────────────────────────┐
 │ pages → api/endpoints (axios)│ ──────────────► │ net/http ServeMux                          │
 │ Zustand alert store          │                 │   middleware: recover → log → CORS → key   │
 │ useAlertSocket (1 WebSocket) │ ◄────WSS─────── │   handlers (httpapi)                       │
 │ React-Leaflet + Geoman       │                 │        │                │                  │
 └──────────────────────────────┘                 │   store (SQL, pgx)   realtime.Hub ─► clients│
                                                  └────────┼──────────────────────────────────┘
                                                           ▼
                                                 PostgreSQL 17 + PostGIS 3.5
```

## 2. Backend packages (`backend/internal`)

| Package | Responsibility | Why it's separate |
|---|---|---|
| `httpapi` | Routes, JSON decoding, validation, status codes, `time_ns` | HTTP concerns in one place; the route table in `router.go` mirrors the README |
| `store` | All SQL (pgx), transactions, error types | Every query is visible, parameterized, and in one package |
| `geo` | Polygon/point validation and `[lat,lng]` ⇄ PostGIS conversion | Coordinate order is the #1 bug risk, so every swap is in one tested file |
| `tracking` | `Diff(before, now)` → entered/exited | Pure logic, unit-tested in isolation |
| `realtime` | WebSocket hub and clients | All concurrency for broadcasting lives here |
| `db` | Connection pool and an embedded SQL migration runner | Migrations ship inside the binary |
| `config`, `ids` | Env config, prefixed random IDs | |

There are deliberately no interfaces "for testing": tests hit a real PostGIS, which is the only honest way to test geospatial SQL.

### Why these choices

- **Go 1.22+ `http.ServeMux`.** It supports `"GET /vehicles/location/{vehicle_id}"` patterns, so the project needs no router library.
- **pgx.** It is the fastest PostgreSQL driver for Go, and every call takes a `context.Context`, so a client that disconnects cancels its SQL.
- **The point-in-polygon check runs in the database.** The database already has every polygon, and a GiST index can discard most of them by bounding box before the exact check. Doing it in Go would mean loading every polygon into memory on every request.
- **Self-written migration runner (~60 lines).** It applies embedded `.sql` files in order, inside transactions, under an advisory lock so two replicas can't migrate at the same time. That is easier to explain than a framework.

## 3. Data model

| Table | Holds | Key constraints / indexes |
|---|---|---|
| `geofences` | polygon `geometry(Polygon,4326)`, category | `CHECK ST_IsValid`, **GiST(boundary)**, index(category) |
| `vehicles` | details plus `last_location` / `last_seen_at` | **unique(upper(vehicle_number))** |
| `vehicle_locations` | full movement history | index(vehicle_id, recorded_at DESC) |
| `vehicle_geofence_state` | which vehicle is inside which geofence **now** | PK(vehicle_id, geofence_id) |
| `alert_configurations` | rules (the README's "alerts") | **unique(geofence, vehicle, event_type) NULLS NOT DISTINCT** |
| `geofence_events` | every entry/exit (feeds `/violations/history`) | indexes on time, (vehicle,time), (geofence,time) |
| `alerts` | fired alerts, `payload` = the exact WebSocket JSON | unique(geofence_event_id), so at most 1 alert per event |

All foreign keys use `ON DELETE CASCADE`, except `alerts.alert_config_id`, which uses `SET NULL`. That way deleting a rule keeps the alert history.

## 4. Geospatial logic

### Coordinate order
- The API and Leaflet use `[latitude, longitude]`.
- PostGIS, WKT and GeoJSON use `(x, y) = (longitude, latitude)`.

Conversions:
- **Polygons:** `geo.PolygonWKT` swaps on the way in, and `geo.PolygonFromGeoJSON` swaps back on the way out. Unit tests check the round trip on the README polygon.
- **Points:** they never go through WKT. The SQL calls `ST_MakePoint($lng, $lat)` with the arguments written in that order, and reads points back with `ST_Y` = latitude and `ST_X` = longitude.

### Point in polygon
```sql
SELECT id, name, category FROM geofences
 WHERE status = 'active' AND ST_Covers(boundary, ST_SetSRID(ST_MakePoint($lng, $lat), 4326))
```
- **`ST_Covers` rather than `ST_Contains`.** `ST_Covers` treats points on the boundary as inside. The README's own example puts the vehicle exactly on a polygon corner and expects `"inside"`. Integration tests cover vertex, edge-midpoint, just-outside, concave ("U" notch) and overlapping cases.
- **The GiST index** makes the query O(log n) candidate lookup + exact test on a handful of polygons.
- **The idea behind it (ray casting):** draw a ray from the point to infinity and count how many polygon edges it crosses. An odd count means inside, an even count means outside. PostGIS implements this robustly, including the edge cases (rays through vertices, collinear edges) that naive implementations get wrong.
- **Planar math on SRID 4326** is accurate at city scale. Polygons crossing the antimeridian (±180°) are not supported (see Limitations).

### Entry/exit detection (`store.RecordLocation`)

All of the following runs in **one transaction**:
1. `SELECT … FROM vehicles WHERE id=$1 FOR UPDATE`. This locks the vehicle row, so concurrent updates for the **same** vehicle run one at a time. Different vehicles are not blocked.
2. Insert the point into `vehicle_locations`.
3. Run the point-in-polygon query above to get `now`.
4. If the timestamp is older than `last_seen_at`, stop here. The point stays in history, but state is unchanged.
5. Read `before` from `vehicle_geofence_state`.
6. `tracking.Diff(before, now)`:
   - entered = now − before
   - exited = before − now
7. For each exit, then each entry:
   - update the state row
   - insert a `geofence_events` row
   - find a matching **active** rule (vehicle-specific preferred over all-vehicles)
   - if one matches, insert an `alerts` row with the exact WebSocket payload
8. Update `vehicles.last_location`. COMMIT.

**Why there are no duplicate alerts:**
- A vehicle that stays inside a zone has the zone in both `before` and `now`, so no event is produced.
- The row lock stops two parallel requests from both seeing "was outside" (tested with 15 concurrent requests → exactly 1 entry).
- `alerts.geofence_event_id` is UNIQUE.

## 5. Real-time delivery and Go concurrency

```
 HTTP goroutine (POST /vehicles/location)
   └─ COMMIT ─► hub.Publish(msg) ──(buffered chan, non-blocking)──► Hub.Run goroutine
                                                                      ├─► client A.send ─► writePump A ─► socket
                                                                      ├─► client B.send ─► writePump B ─► socket
                                                                      └─► …
```

- **Goroutine:** a lightweight thread managed by Go. Thousands of them are cheap.
- **Channel:** a thread-safe queue that goroutines use to pass values to each other.
- **The hub owns the client set.** Only the `Hub.Run` goroutine reads or writes the `clients` map. Everyone else sends it messages on `register`, `unregister` and `broadcast` channels, so the map needs no mutex and cannot have a data race.
- **Each client has 2 goroutines:**
  - `readPump` detects disconnects and pong replies.
  - `writePump` is the only writer to that socket. Gorilla WebSocket allows a single concurrent writer, and this goroutine also sends the heartbeat pings.
- **Back-pressure:**
  - Each client has a 64-message buffer. If it fills up, the hub drops that client instead of blocking everyone.
  - `Publish` never blocks the HTTP handler: if the hub's 256-slot queue is full, the live push is skipped. The alert is still in the database and gets backfilled on reconnect.
- **Alerts are persisted before they are broadcast.** Saving happens inside the transaction and broadcasting after COMMIT, so a client never sees an alert that was rolled back.
- **Shutdown:**
  - SIGTERM stops the HTTP server and lets in-flight requests finish.
  - Cancelling the hub's context closes every socket with a "going away" frame.
  - A `done` channel makes sure no goroutine stays blocked on the stopped hub.
- **Frontend side:** a single socket for the whole app.
  - Reconnect uses exponential backoff with jitter.
  - After each (re)connect it backfills from `GET /alerts/events`, de-duplicating by `event_id`.

Verified with `go test -race`: 20 concurrent clients receiving a broadcast, disconnect cleanup, shutdown, and the guarantee that `Publish` never blocks.

## 6. Cross-cutting

- **`time_ns`:**
  - Every handler starts with `start := time.Now()`.
  - `writeJSON` sets `time_ns = time.Since(start).Nanoseconds()` (as a string) just before encoding.
  - Every response struct embeds `Timing`, so the field is impossible to forget.
  - 404/405 and panic responses go through the same helper.
- **Validation:**
  - All problems are reported at once (`details.fields`).
  - Request bodies are capped at 1 MB.
  - JSON type errors are translated into readable messages.
- **Security:**
  - Parameterized SQL everywhere.
  - A CORS allow-list, shared with the WebSocket origin check.
  - Internal errors are logged, never returned to the client.
  - Optional API key on write requests, compared in constant time.
  - Per-vehicle token-bucket rate limit (`golang.org/x/time/rate`).
  - Distroless image running as non-root.
- **Frontend:**
  - Typed API layer (`src/api/types.ts` mirrors the README).
  - The `useRequest` hook provides loading, error and retry states.
  - Zustand holds the alert feed and the socket status.
  - Pages are lazy-loaded.
  - Ant Design styles go in a CSS layer below Tailwind utilities.

## 7. Limitations and roadmap (for the video)

| Gap | Impact | Plan / estimate |
|---|---|---|
| No geofence update/delete endpoints | Geofences can only be created | Add `PUT/DELETE /geofences/{id}`. On a polygon change, recompute membership for every vehicle inside the old or new shape and emit events. (~0.5 day) |
| Antimeridian / polar polygons | Polygons crossing ±180° are misinterpreted | Use `geography` + `ST_Covers(geography)`, or split polygons at ±180°. (~0.5 day) |
| Alert delivery is at-most-once for live sockets | If the hub queue overflows, the live push is skipped (the DB copy remains) | Clients send their last `event_id` on reconnect; the server replays from the `alerts` table. Already half there through `/alerts/events`. (~0.5 day) |
| Single instance hub | With N backend replicas, a client only gets alerts from the replica it is connected to | Fan out through PostgreSQL `LISTEN/NOTIFY` or Redis pub/sub. Each replica subscribes and broadcasts to its own clients. (~1 day) |
| API-key auth only (bonus, off by default) | No per-user identity | JWT with roles (dispatcher/admin); scope WebSocket subscriptions by tenant. (~1–2 days) |
| `vehicle_locations` grows forever | Storage | Monthly partitions plus a retention policy, or TimescaleDB hypertables. (~0.5 day) |

### Scaling to thousands of vehicles and geofences
- **Reads:** the GiST index keeps point-in-polygon roughly logarithmic in the number of geofences. 10k polygons is still a millisecond query.
- **Writes:** each location update is one short transaction with only a row lock per vehicle, so vehicles never contend with each other. The pgx pool caps connections.
- **Further steps, in order:**
  1. Batch ingestion (accept arrays of points).
  2. A queue (Kafka/NATS) in front of `RecordLocation`, partitioned by `vehicle_id` so order is preserved per vehicle.
  3. Cache the active geofences in memory with an R-tree and use the DB only for persistence.
  4. Horizontal API replicas, with the hub fan-out described above.
  5. A read replica for the history queries.
