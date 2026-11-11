-- 0001_init.sql — initial schema for the geofencing & alert system.
--
-- COORDINATE ORDER: PostGIS stores points as (x, y) = (longitude, latitude).
-- The REST API uses [latitude, longitude]. Conversion happens in internal/geo
-- and in the SQL that calls ST_MakePoint(lng, lat). Never anywhere else.

CREATE EXTENSION IF NOT EXISTS postgis WITH SCHEMA public;

-- Geofences: named polygons in WGS84 (SRID 4326).
CREATE TABLE geofences (
    id          TEXT PRIMARY KEY,                                  -- geo_xxxxxxxxxx
    name        TEXT NOT NULL CHECK (length(btrim(name)) > 0),
    description TEXT NOT NULL DEFAULT '',
    category    TEXT NOT NULL CHECK (category IN ('delivery_zone', 'restricted_zone', 'toll_zone', 'customer_area')),
    boundary    geometry(Polygon, 4326) NOT NULL,
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT geofences_boundary_valid CHECK (ST_IsValid(boundary))
);
-- GiST = spatial index. Lets ST_Covers discard non-candidate polygons by bounding box first.
CREATE INDEX idx_geofences_boundary ON geofences USING GIST (boundary);
CREATE INDEX idx_geofences_category ON geofences (category);

-- Vehicles. last_location/last_seen_at are denormalized for fast "where is it now" reads.
CREATE TABLE vehicles (
    id             TEXT PRIMARY KEY,                               -- veh_xxxxxxxxxx
    vehicle_number TEXT NOT NULL CHECK (length(btrim(vehicle_number)) > 0),
    driver_name    TEXT NOT NULL,
    vehicle_type   TEXT NOT NULL,
    phone          TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    last_location  geometry(Point, 4326),
    last_seen_at   TIMESTAMPTZ,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- Case-insensitive uniqueness: "ka-01-ab-1234" and "KA-01-AB-1234" are the same vehicle.
CREATE UNIQUE INDEX uq_vehicles_vehicle_number ON vehicles (upper(vehicle_number));

-- Full movement history (every accepted location update).
CREATE TABLE vehicle_locations (
    id          BIGSERIAL PRIMARY KEY,
    vehicle_id  TEXT NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    location    geometry(Point, 4326) NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL,                              -- device timestamp from the request
    received_at TIMESTAMPTZ NOT NULL DEFAULT now()                 -- server time
);
CREATE INDEX idx_vehicle_locations_vehicle_time ON vehicle_locations (vehicle_id, recorded_at DESC);

-- Current membership: which vehicle is inside which geofence right now.
-- Entry/exit detection = set difference between this table and the new position's geofences.
CREATE TABLE vehicle_geofence_state (
    vehicle_id  TEXT NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    geofence_id TEXT NOT NULL REFERENCES geofences (id) ON DELETE CASCADE,
    entered_at  TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (vehicle_id, geofence_id)
);
CREATE INDEX idx_vehicle_geofence_state_geofence ON vehicle_geofence_state (geofence_id);

-- Alert rules (exposed as "alerts" by POST /alerts/configure and GET /alerts).
CREATE TABLE alert_configurations (
    id          TEXT PRIMARY KEY,                                  -- alert_xxxxxxxxxx
    geofence_id TEXT NOT NULL REFERENCES geofences (id) ON DELETE CASCADE,
    vehicle_id  TEXT REFERENCES vehicles (id) ON DELETE CASCADE,  -- NULL = applies to all vehicles
    event_type  TEXT NOT NULL CHECK (event_type IN ('entry', 'exit', 'both')),
    status      TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'inactive')),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
-- No duplicate rules. NULLS NOT DISTINCT (PostgreSQL 15+) makes two "all vehicles" rules collide too.
CREATE UNIQUE INDEX uq_alert_configurations_rule
    ON alert_configurations (geofence_id, vehicle_id, event_type) NULLS NOT DISTINCT;
CREATE INDEX idx_alert_configurations_vehicle ON alert_configurations (vehicle_id);

-- Every entry/exit transition. Backs GET /violations/history.
CREATE TABLE geofence_events (
    id          TEXT PRIMARY KEY,                                  -- viol_xxxxxxxxxx
    vehicle_id  TEXT NOT NULL REFERENCES vehicles (id) ON DELETE CASCADE,
    geofence_id TEXT NOT NULL REFERENCES geofences (id) ON DELETE CASCADE,
    location_id BIGINT REFERENCES vehicle_locations (id) ON DELETE SET NULL,
    event_type  TEXT NOT NULL CHECK (event_type IN ('entry', 'exit')),
    location    geometry(Point, 4326) NOT NULL,
    occurred_at TIMESTAMPTZ NOT NULL
);
CREATE INDEX idx_geofence_events_time ON geofence_events (occurred_at DESC);
CREATE INDEX idx_geofence_events_vehicle_time ON geofence_events (vehicle_id, occurred_at DESC);
CREATE INDEX idx_geofence_events_geofence_time ON geofence_events (geofence_id, occurred_at DESC);

-- Fired alerts: one per geofence event that matched an active rule.
-- payload is the exact JSON message broadcast over /ws/alerts.
CREATE TABLE alerts (
    id                TEXT PRIMARY KEY,                            -- evt_xxxxxxxxxx (= WebSocket event_id)
    geofence_event_id TEXT NOT NULL UNIQUE REFERENCES geofence_events (id) ON DELETE CASCADE,
    alert_config_id   TEXT REFERENCES alert_configurations (id) ON DELETE SET NULL,
    payload           JSONB NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX idx_alerts_created ON alerts (created_at DESC);
