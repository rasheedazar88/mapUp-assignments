# API Reference

Base URL: `http://localhost:8080` (local). All bodies are JSON. **Every response, including errors, has `"time_ns": "<nanoseconds as a string>"`**, the handler's execution time measured with Go's `time` package.

Coordinates are always **`[latitude, longitude]`**.

IDs are prefixed random strings:

| Prefix | Identifies |
|---|---|
| `geo_` | geofence |
| `veh_` | vehicle |
| `alert_` | alert rule |
| `viol_` | entry/exit event |
| `evt_` | fired alert / WebSocket event |

## Errors

```json
{
  "error": {
    "code": "validation_error",
    "message": "coordinates: first and last coordinates must be identical (closed polygon)",
    "details": { "fields": { "coordinates": "first and last coordinates must be identical (closed polygon)" } }
  },
  "time_ns": "84211"
}
```

| Status | `code` | When |
|---|---|---|
| 400 | `validation_error` | A field is missing or invalid (all problems are listed in `details.fields`) |
| 400 | `invalid_json` | Body is malformed, empty, has wrong types, or is over 1 MB |
| 401 | `unauthorized` | `API_KEY` is enabled and `X-API-Key` is missing or wrong (write requests only) |
| 404 | `not_found` | Unknown vehicle/geofence/alert, or unknown route |
| 405 | `method_not_allowed` | Wrong method on a known path (an `Allow` header is set) |
| 409 | `conflict` | Duplicate `vehicle_number` (case-insensitive) or duplicate alert rule |
| 429 | `rate_limited` | Too many location updates for one vehicle (`Retry-After: 1`) |
| 500 | `internal_error` | Unexpected; details are logged on the server, never returned |

---

## README endpoints

### 1. `POST /geofences` → 201

```json
{ "name": "Downtown Delivery Zone", "description": "optional",
  "coordinates": [[37.7749,-122.4194],[37.7849,-122.4194],[37.7849,-122.4094],[37.7749,-122.4094],[37.7749,-122.4194]],
  "category": "delivery_zone" }
```

Validation:
- `name` is required (max 120 characters).
- `category` must be one of `delivery_zone | restricted_zone | toll_zone | customer_area`.
- `coordinates`:
  - At least 4 points, with at least 3 unique.
  - Each point is exactly 2 numbers.
  - Latitude is in [-90, 90] and longitude in [-180, 180].
  - First point == last point.
  - Non-zero area.
  - No self-intersection (checked by PostGIS `ST_IsValid`).

Response: `{"id":"geo_…","name":"Downtown Delivery Zone","status":"active","time_ns":"…"}`

### 2. `GET /geofences?category=` → 200

`{"geofences":[{"id","name","description","coordinates","category","status","created_at"}],"time_ns"}`. Results are newest first. An invalid `category` returns 400. Optional `limit` (1–1000) and `offset`.

### 3. `POST /vehicles` → 201

`vehicle_number`, `driver_name`, `vehicle_type` and `phone` are all required.
- `phone`: digits with an optional leading `+`; spaces, dashes and parentheses are allowed; 7–20 characters.
- `vehicle_type` is lower-cased.

Response: `{"id":"veh_…","vehicle_number","status":"active","time_ns"}`.

### 4. `GET /vehicles` → 200

`{"vehicles":[{"id","vehicle_number","driver_name","vehicle_type","phone","status","created_at"}],"time_ns"}`. Optional `limit`/`offset`.

### 5. `POST /vehicles/location` → 200

```json
{ "vehicle_id": "veh_…", "latitude": 37.7849, "longitude": -122.4194, "timestamp": "2025-01-15T10:35:00Z" }
```

Field rules:
- `timestamp` is RFC 3339 / ISO 8601 (offsets and fractions allowed). It may be at most 5 minutes in the future.
- `latitude: 0` and `longitude: 0` are valid values, not "missing".

Response:

```json
{ "vehicle_id": "veh_…", "location_updated": true,
  "current_geofences": [{ "geofence_id": "geo_…", "geofence_name": "Downtown Delivery Zone", "status": "inside" }],
  "time_ns": "2345678" }
```

Behaviour, all in one transaction:
1. Store the point in history.
2. Find the active geofences that cover the point. A point on the boundary counts as inside.
3. Diff against the vehicle's previous geofences, producing `entry`/`exit` events.
4. Persist the events.
5. Match the active alert rules and persist one alert per matched event.
6. Commit.
7. Broadcast the alerts over WebSocket asynchronously.

An update whose `timestamp` is older than the vehicle's last one is stored in history but does not change state or fire events.

Errors: unknown vehicle → 404; more than `RATE_LIMIT_RPS` per vehicle → 429.

### 6. `GET /vehicles/location/{vehicle_id}` → 200

```json
{ "vehicle_id": "veh_…", "vehicle_number": "KA-01-AB-1234",
  "current_location": { "latitude": 37.7849, "longitude": -122.4194, "timestamp": "2025-01-15T10:35:00Z" },
  "current_geofences": [{ "geofence_id": "geo_…", "geofence_name": "Downtown Delivery Zone", "category": "delivery_zone" }],
  "time_ns": "…" }
```

If no location has been reported yet: `current_location: null` and `current_geofences: []`. Unknown vehicle → 404.

### 7. `POST /alerts/configure` → 201

`{"geofence_id":"geo_…","vehicle_id":"veh_…","event_type":"entry"}`

- `vehicle_id` is optional. If it is omitted, null or `""`, the rule applies to all vehicles.
- `event_type` is `entry | exit | both`.

Response: `{"alert_id":"alert_…","geofence_id","vehicle_id": "veh_…" | null,"event_type","status":"active","time_ns"}`.

Errors: unknown geofence or vehicle → 404; an identical rule already exists → 409 (`details.alert_id` gives the existing rule).

### 8. `GET /alerts?geofence_id=&vehicle_id=` → 200

`{"alerts":[{"alert_id","geofence_id","geofence_name","vehicle_id","vehicle_number","event_type","status","created_at"}],"time_ns"}`

`vehicle_id`/`vehicle_number` are `null` for all-vehicle rules. Filtering by `vehicle_id` also returns the all-vehicle rules, because they apply to that vehicle too.

### 9. `GET /violations/history` → 200

Query parameters, all optional:
- `vehicle_id`, `geofence_id`.
- `start_date`, `end_date`: ISO 8601. A plain date `YYYY-MM-DD` is accepted and covers the whole day.
- `limit`: default 50; values over 500 are clamped to 500.
- `offset`: extra, for pagination.

```json
{ "violations": [{ "id": "viol_…", "vehicle_id", "vehicle_number", "geofence_id", "geofence_name", "category",
                   "event_type": "entry", "latitude", "longitude", "timestamp" }],
  "total_count": 245, "time_ns": "…" }
```

Results are newest first. `total_count` counts all rows that match the filters, ignoring limit/offset. If `end_date` is before `start_date`, the response is 400.

---

## WebSocket: `GET /ws/alerts`

Connect with `ws://host/ws/alerts` (`wss://` in production).
- The server only pushes; any messages the client sends are ignored.
- The server sends a ping every ~54 s, and clients must answer within 60 s. Browsers do this automatically.
- A client that falls far behind is disconnected rather than slowing everyone down.

One message per fired alert (exactly the README format):

```json
{
  "event_id": "evt_999",
  "event_type": "entry",
  "timestamp": "2025-01-15T10:35:00Z",
  "vehicle":  { "vehicle_id": "veh_456", "vehicle_number": "KA-01-AB-1234", "driver_name": "John Doe" },
  "geofence": { "geofence_id": "geo_123", "geofence_name": "Downtown Delivery Zone", "category": "delivery_zone" },
  "location": { "latitude": 37.7849, "longitude": -122.4194 }
}
```

An alert is generated only when an entry/exit event matches an **active** rule:
- The rule is on the same geofence.
- It is for the same vehicle or for all vehicles.
- Its event type is the same, or `both`.

One event produces at most one alert, even if several rules match.

Reconnect: the client reconnects with backoff. It then calls `GET /alerts/events` to backfill anything it missed while disconnected.

---

## Extra endpoints (additive; not required by the brief)

| Method & path | Purpose |
|---|---|
| `GET /health` | `{"status":"ok","database":"ok","ws_clients":N,"time_ns"}` (503 if the DB is unreachable) |
| `GET /geofences/{geofence_id}` | One geofence |
| `GET /vehicles/locations` | Every vehicle with `current_location` + `current_geofences` (one call for the live map) |
| `PATCH /alerts/{alert_id}` | `{"status":"active"\|"inactive"}`: enable or disable a rule |
| `DELETE /alerts/{alert_id}` | Delete a rule (alerts already fired are kept) |
| `GET /alerts/events?limit=50` | Recently fired alerts, in the exact WebSocket message format (max 200) |
