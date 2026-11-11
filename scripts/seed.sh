#!/usr/bin/env bash
# Seeds realistic demo data through the PUBLIC API (so it doubles as a curl example).
#   ./scripts/seed.sh                         # against http://localhost:8080
#   API=https://your-backend.onrender.com ./scripts/seed.sh
# Works in Git Bash on Windows, macOS and Linux. Needs only curl.
set -euo pipefail
API="${API:-http://localhost:8080}"
H='Content-Type: application/json'

post() { curl -sS -X POST "$API$1" -H "$H" -d "$2"; }
id_of() { sed -E 's/.*"(id|alert_id)":"([^"]+)".*/\2/'; }

echo "Seeding $API"
curl -sf "$API/health" >/dev/null || { echo "Backend not reachable at $API"; exit 1; }

# --- Geofences (San Francisco) — coordinates are [latitude, longitude] ---
DOWNTOWN=$(post /geofences '{"name":"Downtown Delivery Zone","description":"Main delivery area for downtown customers","category":"delivery_zone",
  "coordinates":[[37.7749,-122.4194],[37.7849,-122.4194],[37.7849,-122.4094],[37.7749,-122.4094],[37.7749,-122.4194]]}' | id_of)
YARD=$(post /geofences '{"name":"Port Restricted Yard","description":"Hazmat storage, authorised vehicles only","category":"restricted_zone",
  "coordinates":[[37.7955,-122.3990],[37.8005,-122.3990],[37.8005,-122.3920],[37.7955,-122.3920],[37.7955,-122.3990]]}' | id_of)
TOLL=$(post /geofences '{"name":"Bay Bridge Toll Plaza","description":"Toll collection area","category":"toll_zone",
  "coordinates":[[37.8235,-122.3170],[37.8265,-122.3170],[37.8265,-122.3090],[37.8235,-122.3090],[37.8235,-122.3170]]}' | id_of)
MISSION=$(post /geofences '{"name":"Mission Customer Hub","description":"Key customer cluster (concave shape)","category":"customer_area",
  "coordinates":[[37.7540,-122.4220],[37.7640,-122.4220],[37.7640,-122.4080],[37.7600,-122.4080],[37.7600,-122.4160],[37.7540,-122.4160],[37.7540,-122.4220]]}' | id_of)
echo "geofences: $DOWNTOWN $YARD $TOLL $MISSION"

# --- Vehicles ---
V1=$(post /vehicles '{"vehicle_number":"KA-01-AB-1234","driver_name":"John Doe","vehicle_type":"truck","phone":"+1234567890"}' | id_of)
V2=$(post /vehicles '{"vehicle_number":"CA-7XYZ-889","driver_name":"Maria Lopez","vehicle_type":"van","phone":"+14155550123"}' | id_of)
V3=$(post /vehicles '{"vehicle_number":"CA-4BIKE-21","driver_name":"Sam Lee","vehicle_type":"motorcycle","phone":"+14155550188"}' | id_of)
echo "vehicles: $V1 $V2 $V3"

# --- Alert rules ---
post /alerts/configure "{\"geofence_id\":\"$YARD\",\"event_type\":\"both\"}" >/dev/null          # all vehicles, entry+exit
post /alerts/configure "{\"geofence_id\":\"$DOWNTOWN\",\"vehicle_id\":\"$V1\",\"event_type\":\"entry\"}" >/dev/null
post /alerts/configure "{\"geofence_id\":\"$TOLL\",\"event_type\":\"entry\"}" >/dev/null
echo "alert rules created"

# --- Starting positions (timestamps = now) ---
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
post /vehicles/location "{\"vehicle_id\":\"$V1\",\"latitude\":37.7700,\"longitude\":-122.4300,\"timestamp\":\"$NOW\"}" >/dev/null
post /vehicles/location "{\"vehicle_id\":\"$V2\",\"latitude\":37.7590,\"longitude\":-122.4190,\"timestamp\":\"$NOW\"}" >/dev/null
post /vehicles/location "{\"vehicle_id\":\"$V3\",\"latitude\":37.7900,\"longitude\":-122.4000,\"timestamp\":\"$NOW\"}" >/dev/null
echo "positions set. Done. Open the frontend and try Live Tracking → Simulate trip."
