package httpapi

// End-to-end tests: real HTTP handler + real PostGIS (via TEST_DATABASE_URL).
// They exercise all 9 README endpoints, the WebSocket feed, validation and errors.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"geofence/internal/realtime"
	"geofence/internal/store"
	"geofence/internal/testutil"
)

// ---------- harness ----------

type harness struct {
	t   *testing.T
	srv *httptest.Server
	hub *realtime.Hub
}

func newHarness(t *testing.T, opts ...func(*Options)) *harness {
	t.Helper()
	pool := testutil.NewTestDB(t)
	st := store.New(pool)

	ctx, cancel := context.WithCancel(context.Background())
	hub := realtime.NewHub()
	go hub.Run(ctx)

	o := Options{AllowedOrigins: []string{"http://localhost:5173"}, RateLimitRPS: 1000, RateLimitBurst: 1000, Ping: st.Ping}
	for _, f := range opts {
		f(&o)
	}
	srv := httptest.NewServer(NewHandler(st, hub, o))
	t.Cleanup(func() { srv.Close(); cancel() })
	return &harness{t: t, srv: srv, hub: hub}
}

// do sends a request and decodes the JSON response into a generic map.
// It also asserts the cross-cutting rule: EVERY response has a numeric-string time_ns.
func (h *harness) do(method, path string, body any) (int, map[string]any) {
	h.t.Helper()
	var reader *bytes.Reader
	switch b := body.(type) {
	case nil:
		reader = bytes.NewReader(nil)
	case string:
		reader = bytes.NewReader([]byte(b))
	default:
		raw, _ := json.Marshal(b)
		reader = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, reader)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer resp.Body.Close()

	var out map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		h.t.Fatalf("%s %s: response is not JSON: %v", method, path, err)
	}
	ns, ok := out["time_ns"].(string)
	if !ok {
		h.t.Fatalf("%s %s: time_ns missing or not a string: %v", method, path, out)
	}
	if n, err := strconv.ParseInt(ns, 10, 64); err != nil || n < 0 {
		h.t.Fatalf("%s %s: time_ns %q is not a non-negative integer string", method, path, ns)
	}
	return resp.StatusCode, out
}

func (h *harness) mustStatus(want int, method, path string, body any) map[string]any {
	h.t.Helper()
	got, out := h.do(method, path, body)
	if got != want {
		h.t.Fatalf("%s %s: status %d, want %d; body %v", method, path, got, want, out)
	}
	return out
}

var readmePolygon = [][]float64{
	{37.7749, -122.4194}, {37.7849, -122.4194}, {37.7849, -122.4094}, {37.7749, -122.4094}, {37.7749, -122.4194},
}

func (h *harness) createGeofence(name, category string, coords [][]float64) string {
	out := h.mustStatus(201, "POST", "/geofences", map[string]any{
		"name": name, "description": "test", "coordinates": coords, "category": category,
	})
	return out["id"].(string)
}

var vehicleSeq int
var vehicleSeqMu sync.Mutex

func (h *harness) createVehicle() string {
	vehicleSeqMu.Lock()
	vehicleSeq++
	n := vehicleSeq
	vehicleSeqMu.Unlock()
	out := h.mustStatus(201, "POST", "/vehicles", map[string]any{
		"vehicle_number": fmt.Sprintf("KA-01-AB-%04d", n), "driver_name": "John Doe", "vehicle_type": "truck", "phone": "+1234567890",
	})
	return out["id"].(string)
}

func (h *harness) move(vehicleID string, lat, lng float64, ts string) map[string]any {
	return h.mustStatus(200, "POST", "/vehicles/location", map[string]any{
		"vehicle_id": vehicleID, "latitude": lat, "longitude": lng, "timestamp": ts,
	})
}

func insideIDs(resp map[string]any) []string {
	var out []string
	for _, g := range resp["current_geofences"].([]any) {
		out = append(out, g.(map[string]any)["geofence_id"].(string))
	}
	return out
}

func assertKeys(t *testing.T, what string, obj map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := obj[k]; !ok {
			t.Errorf("%s: missing key %q in %v", what, k, obj)
		}
	}
}

// ---------- 1 & 2: geofences ----------

func TestGeofences(t *testing.T) {
	h := newHarness(t)

	out := h.mustStatus(201, "POST", "/geofences", map[string]any{
		"name": "Downtown Delivery Zone", "description": "Main delivery area for downtown customers",
		"coordinates": readmePolygon, "category": "delivery_zone",
	})
	assertKeys(t, "create response", out, "id", "name", "status", "time_ns")
	if !strings.HasPrefix(out["id"].(string), "geo_") || out["status"] != "active" || out["name"] != "Downtown Delivery Zone" {
		t.Fatalf("unexpected create response %v", out)
	}
	h.createGeofence("No-go yard", "restricted_zone", [][]float64{{1, 1}, {1, 2}, {2, 2}, {1, 1}})

	all := h.mustStatus(200, "GET", "/geofences", nil)["geofences"].([]any)
	if len(all) != 2 {
		t.Fatalf("want 2 geofences, got %d", len(all))
	}
	filtered := h.mustStatus(200, "GET", "/geofences?category=delivery_zone", nil)["geofences"].([]any)
	if len(filtered) != 1 {
		t.Fatalf("category filter: want 1, got %d", len(filtered))
	}
	g := filtered[0].(map[string]any)
	assertKeys(t, "geofence", g, "id", "name", "description", "coordinates", "category", "created_at")
	// Coordinates must come back in [lat, lng] order, unchanged.
	raw, _ := json.Marshal(g["coordinates"])
	want, _ := json.Marshal(readmePolygon)
	if string(raw) != string(want) {
		t.Fatalf("coordinates round trip:\n got %s\nwant %s", raw, want)
	}
	if _, err := time.Parse(time.RFC3339, g["created_at"].(string)); err != nil {
		t.Errorf("created_at not ISO 8601: %v", g["created_at"])
	}

	h.mustStatus(400, "GET", "/geofences?category=airport", nil)
	h.mustStatus(200, "GET", "/geofences?limit=1&offset=1", nil)
}

func TestGeofenceValidation(t *testing.T) {
	h := newHarness(t)
	base := func(coords any) map[string]any {
		return map[string]any{"name": "Z", "coordinates": coords, "category": "toll_zone"}
	}
	cases := []struct {
		name string
		body any
		want string // substring of the error message
	}{
		{"not closed", base([][]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}), "first and last"},
		{"too few points", base([][]float64{{0, 0}, {1, 0}, {0, 0}}), "at least 4"},
		{"latitude out of range", base([][]float64{{95, 0}, {1, 0}, {0, 1}, {95, 0}}), "latitude"},
		{"longitude out of range", base([][]float64{{0, 200}, {1, 0}, {0, 1}, {0, 200}}), "longitude"},
		{"self-intersecting (PostGIS check)", base([][]float64{{0, 0}, {2, 2}, {2, 0}, {0, 1}, {0, 0}}), "not valid"},
		{"missing coordinates", map[string]any{"name": "Z", "category": "toll_zone"}, "coordinates: is required"},
		{"missing name", map[string]any{"coordinates": readmePolygon, "category": "toll_zone"}, "name: is required"},
		{"bad category", map[string]any{"name": "Z", "coordinates": readmePolygon, "category": "airport"}, "category"},
		{"coordinates wrong type", `{"name":"Z","category":"toll_zone","coordinates":[["a","b"]]}`, "must be of type"},
		{"malformed JSON", `{"name":`, "malformed"},
		{"empty body", "", "required"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := h.mustStatus(400, "POST", "/geofences", c.body)
			msg := out["error"].(map[string]any)["message"].(string)
			if !strings.Contains(msg, c.want) {
				t.Fatalf("message %q does not contain %q", msg, c.want)
			}
		})
	}
}

// ---------- 3 & 4: vehicles ----------

func TestVehicles(t *testing.T) {
	h := newHarness(t)
	body := map[string]any{"vehicle_number": "KA-01-AB-1234", "driver_name": "John Doe", "vehicle_type": "truck", "phone": "+1234567890"}
	out := h.mustStatus(201, "POST", "/vehicles", body)
	assertKeys(t, "create vehicle", out, "id", "vehicle_number", "status", "time_ns")
	if !strings.HasPrefix(out["id"].(string), "veh_") || out["status"] != "active" {
		t.Fatalf("unexpected %v", out)
	}

	// Duplicate, case-insensitive.
	body["vehicle_number"] = "ka-01-ab-1234"
	h.mustStatus(409, "POST", "/vehicles", body)

	h.mustStatus(400, "POST", "/vehicles", map[string]any{"vehicle_number": "X"})
	h.mustStatus(400, "POST", "/vehicles", map[string]any{"vehicle_number": "X", "driver_name": "A", "vehicle_type": "car", "phone": "call me"})

	list := h.mustStatus(200, "GET", "/vehicles", nil)["vehicles"].([]any)
	if len(list) != 1 {
		t.Fatalf("want 1 vehicle, got %d", len(list))
	}
	assertKeys(t, "vehicle", list[0].(map[string]any), "id", "vehicle_number", "driver_name", "vehicle_type", "phone", "status", "created_at")
}

// ---------- 5 & 6: location updates, point-in-polygon, entry/exit ----------

func TestLocationEntryExitAndHistory(t *testing.T) {
	h := newHarness(t)
	geoID := h.createGeofence("Downtown Delivery Zone", "delivery_zone", readmePolygon)
	vehID := h.createVehicle()

	// GET before any location: current_location is null.
	out := h.mustStatus(200, "GET", "/vehicles/location/"+vehID, nil)
	if out["current_location"] != nil || len(out["current_geofences"].([]any)) != 0 {
		t.Fatalf("expected empty location state, got %v", out)
	}

	// README example point: exactly on a polygon corner → inside (ST_Covers).
	out = h.move(vehID, 37.7849, -122.4194, "2025-01-15T10:35:00Z")
	assertKeys(t, "location response", out, "vehicle_id", "location_updated", "current_geofences", "time_ns")
	if out["location_updated"] != true || len(insideIDs(out)) != 1 || insideIDs(out)[0] != geoID {
		t.Fatalf("vertex point should be inside: %v", out)
	}
	if out["current_geofences"].([]any)[0].(map[string]any)["status"] != "inside" {
		t.Fatalf("status should be inside: %v", out)
	}

	// Still inside (centre) → no new event.
	h.move(vehID, 37.7799, -122.4144, "2025-01-15T10:36:00Z")
	// On an edge midpoint → still inside, no event.
	h.move(vehID, 37.7799, -122.4194, "2025-01-15T10:37:00Z")

	out = h.mustStatus(200, "GET", "/vehicles/location/"+vehID, nil)
	assertKeys(t, "get location", out, "vehicle_id", "vehicle_number", "current_location", "current_geofences", "time_ns")
	loc := out["current_location"].(map[string]any)
	if loc["latitude"] != 37.7799 || loc["longitude"] != -122.4194 || loc["timestamp"] != "2025-01-15T10:37:00Z" {
		t.Fatalf("current_location wrong: %v", loc)
	}
	cg := out["current_geofences"].([]any)[0].(map[string]any)
	if cg["category"] != "delivery_zone" || cg["geofence_name"] != "Downtown Delivery Zone" {
		t.Fatalf("current_geofences wrong: %v", cg)
	}

	// Just outside (0.0001° north of the top edge) → exit.
	out = h.move(vehID, 37.7850, -122.4144, "2025-01-15T10:40:00Z")
	if len(insideIDs(out)) != 0 {
		t.Fatalf("should be outside: %v", out)
	}

	// Delayed packet from inside with an OLD timestamp: stored, but no state change.
	h.move(vehID, 37.7799, -122.4144, "2025-01-15T10:38:00Z")
	out = h.mustStatus(200, "GET", "/vehicles/location/"+vehID, nil)
	if len(out["current_geofences"].([]any)) != 0 {
		t.Fatalf("stale update must not change state: %v", out)
	}

	// History: exactly one entry + one exit, newest first.
	hist := h.mustStatus(200, "GET", "/violations/history?vehicle_id="+vehID, nil)
	assertKeys(t, "history", hist, "violations", "total_count", "time_ns")
	v := hist["violations"].([]any)
	if hist["total_count"].(float64) != 2 || len(v) != 2 {
		t.Fatalf("want 2 events, got %v", hist)
	}
	first := v[0].(map[string]any)
	assertKeys(t, "violation", first, "id", "vehicle_id", "vehicle_number", "geofence_id", "geofence_name", "event_type", "latitude", "longitude", "timestamp")
	if first["event_type"] != "exit" || v[1].(map[string]any)["event_type"] != "entry" {
		t.Fatalf("order/event types wrong: %v", v)
	}

	// Filters & pagination.
	if n := h.mustStatus(200, "GET", "/violations/history?geofence_id="+geoID+"&limit=1", nil); n["total_count"].(float64) != 2 || len(n["violations"].([]any)) != 1 {
		t.Fatalf("limit=1 should return 1 row but total_count 2: %v", n)
	}
	if n := h.mustStatus(200, "GET", "/violations/history?start_date=2025-01-15T10:39:00Z", nil); n["total_count"].(float64) != 1 {
		t.Fatalf("start_date filter: %v", n)
	}
	if n := h.mustStatus(200, "GET", "/violations/history?start_date=2025-01-15&end_date=2025-01-15", nil); n["total_count"].(float64) != 2 {
		t.Fatalf("date-only range should cover the whole day: %v", n)
	}
	if n := h.mustStatus(200, "GET", "/violations/history?end_date=2024-12-31T00:00:00Z", nil); n["total_count"].(float64) != 0 {
		t.Fatalf("end_date filter: %v", n)
	}
	h.mustStatus(200, "GET", "/violations/history?limit=9999", nil) // clamped to 500, not an error
	h.mustStatus(400, "GET", "/violations/history?limit=0", nil)
	h.mustStatus(400, "GET", "/violations/history?start_date=yesterday", nil)
	h.mustStatus(400, "GET", "/violations/history?start_date=2025-02-01T00:00:00Z&end_date=2025-01-01T00:00:00Z", nil)
}

func TestOverlappingGeofencesAndConcavePolygon(t *testing.T) {
	h := newHarness(t)
	a := h.createGeofence("Big", "customer_area", [][]float64{{0, 0}, {0, 10}, {10, 10}, {10, 0}, {0, 0}})
	b := h.createGeofence("Small", "toll_zone", [][]float64{{4, 4}, {4, 6}, {6, 6}, {6, 4}, {4, 4}})
	// "U" shape: the notch between the arms (around lat 5, lng 25) is OUTSIDE.
	u := h.createGeofence("U", "delivery_zone", [][]float64{{0, 20}, {10, 20}, {10, 30}, {0, 30}, {0, 28}, {8, 28}, {8, 22}, {0, 22}, {0, 20}})
	veh := h.createVehicle()

	got := insideIDs(h.move(veh, 5, 5, "2025-01-01T00:00:00Z"))
	if len(got) != 2 || !(contains(got, a) && contains(got, b)) {
		t.Fatalf("expected inside both overlapping zones, got %v", got)
	}
	if got := insideIDs(h.move(veh, 5, 25, "2025-01-01T00:01:00Z")); len(got) != 0 {
		t.Fatalf("U notch must be outside, got %v", got)
	}
	if got := insideIDs(h.move(veh, 9, 25, "2025-01-01T00:02:00Z")); len(got) != 1 || got[0] != u {
		t.Fatalf("U base must be inside U, got %v", got)
	}
	hist := h.mustStatus(200, "GET", "/violations/history?vehicle_id="+veh, nil)
	// entry a, entry b, exit a, exit b, entry u = 5
	if hist["total_count"].(float64) != 5 {
		t.Fatalf("want 5 events, got %v", hist["total_count"])
	}
}

func TestLocationValidationAndErrors(t *testing.T) {
	h := newHarness(t)
	veh := h.createVehicle()
	h.mustStatus(404, "POST", "/vehicles/location", map[string]any{"vehicle_id": "veh_missing", "latitude": 1, "longitude": 1, "timestamp": "2025-01-15T10:35:00Z"})
	h.mustStatus(400, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "longitude": 1, "timestamp": "2025-01-15T10:35:00Z"}) // missing latitude
	h.mustStatus(400, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "latitude": 91, "longitude": 1, "timestamp": "2025-01-15T10:35:00Z"})
	h.mustStatus(400, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "latitude": 1, "longitude": -181, "timestamp": "2025-01-15T10:35:00Z"})
	h.mustStatus(400, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "latitude": 1, "longitude": 1, "timestamp": "15/01/2025"})
	h.mustStatus(400, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "latitude": 1, "longitude": 1, "timestamp": time.Now().Add(time.Hour).Format(time.RFC3339)})
	// latitude 0 / longitude 0 is a real place, not "missing".
	h.mustStatus(200, "POST", "/vehicles/location", map[string]any{"vehicle_id": veh, "latitude": 0, "longitude": 0, "timestamp": "2025-01-15T10:35:00Z"})
	h.mustStatus(404, "GET", "/vehicles/location/veh_missing", nil)
}

// Two requests for the same vehicle at the same moment must not both see
// "was outside" and both create an entry event (row lock in RecordLocation).
func TestConcurrentUpdatesProduceSingleEntry(t *testing.T) {
	h := newHarness(t)
	h.createGeofence("Zone", "delivery_zone", readmePolygon)
	veh := h.createVehicle()

	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h.move(veh, 37.7799, -122.4144, "2025-01-15T10:35:00Z")
		}()
	}
	wg.Wait()
	hist := h.mustStatus(200, "GET", "/violations/history?vehicle_id="+veh, nil)
	if hist["total_count"].(float64) != 1 {
		t.Fatalf("want exactly 1 entry event, got %v", hist["total_count"])
	}
}

func TestRateLimit(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.RateLimitRPS = 1; o.RateLimitBurst = 2 })
	veh := h.createVehicle()
	body := map[string]any{"vehicle_id": veh, "latitude": 1, "longitude": 1, "timestamp": "2025-01-15T10:35:00Z"}
	h.mustStatus(200, "POST", "/vehicles/location", body)
	h.mustStatus(200, "POST", "/vehicles/location", body)
	h.mustStatus(429, "POST", "/vehicles/location", body)
}

// ---------- 7 & 8: alert rules ----------

func TestAlertConfiguration(t *testing.T) {
	h := newHarness(t)
	geoID := h.createGeofence("Zone", "restricted_zone", readmePolygon)
	vehID := h.createVehicle()
	otherVeh := h.createVehicle()

	out := h.mustStatus(201, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "vehicle_id": vehID, "event_type": "entry"})
	assertKeys(t, "configure", out, "alert_id", "geofence_id", "vehicle_id", "event_type", "status", "time_ns")
	if !strings.HasPrefix(out["alert_id"].(string), "alert_") || out["vehicle_id"] != vehID || out["status"] != "active" {
		t.Fatalf("unexpected %v", out)
	}

	// vehicle_id omitted → all vehicles → null.
	all := h.mustStatus(201, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "event_type": "exit"})
	if all["vehicle_id"] != nil {
		t.Fatalf("vehicle_id should be null for all-vehicle rule, got %v", all["vehicle_id"])
	}

	h.mustStatus(409, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "event_type": "exit"}) // duplicate all-vehicles rule
	h.mustStatus(404, "POST", "/alerts/configure", map[string]any{"geofence_id": "geo_missing", "event_type": "entry"})
	h.mustStatus(404, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "vehicle_id": "veh_missing", "event_type": "entry"})
	h.mustStatus(400, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "event_type": "sometimes"})
	h.mustStatus(400, "POST", "/alerts/configure", map[string]any{"event_type": "entry"})

	list := h.mustStatus(200, "GET", "/alerts", nil)["alerts"].([]any)
	if len(list) != 2 {
		t.Fatalf("want 2 rules, got %d", len(list))
	}
	assertKeys(t, "alert", list[0].(map[string]any), "alert_id", "geofence_id", "geofence_name", "vehicle_id", "vehicle_number", "event_type", "status", "created_at")
	if n := len(h.mustStatus(200, "GET", "/alerts?geofence_id="+geoID, nil)["alerts"].([]any)); n != 2 {
		t.Fatalf("geofence filter: %d", n)
	}
	// vehicle filter: its own rule + the all-vehicles rule.
	if n := len(h.mustStatus(200, "GET", "/alerts?vehicle_id="+vehID, nil)["alerts"].([]any)); n != 2 {
		t.Fatalf("vehicle filter: %d", n)
	}
	if n := len(h.mustStatus(200, "GET", "/alerts?vehicle_id="+otherVeh, nil)["alerts"].([]any)); n != 1 {
		t.Fatalf("other vehicle filter should only see the all-vehicles rule: %d", n)
	}

	// Manage (extras): disable, delete.
	ruleID := out["alert_id"].(string)
	if p := h.mustStatus(200, "PATCH", "/alerts/"+ruleID, map[string]any{"status": "inactive"}); p["status"] != "inactive" {
		t.Fatalf("patch: %v", p)
	}
	h.mustStatus(200, "DELETE", "/alerts/"+ruleID, nil)
	h.mustStatus(404, "DELETE", "/alerts/"+ruleID, nil)
}

// ---------- Real-time alerts over WebSocket ----------

// wsClient reads messages on a background goroutine into a channel.
// (A gorilla connection is unusable after a read timeout, so tests must not
// use read deadlines to check "no message arrived"; they wait on the channel instead.)
type wsClient struct {
	conn *websocket.Conn
	msgs chan []byte
}

func dialWS(t *testing.T, h *harness) *wsClient {
	t.Helper()
	url := "ws" + strings.TrimPrefix(h.srv.URL, "http") + "/ws/alerts"
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		t.Fatal(err)
	}
	c := &wsClient{conn: conn, msgs: make(chan []byte, 16)}
	go func() {
		defer close(c.msgs)
		for {
			_, raw, err := conn.ReadMessage()
			if err != nil {
				return
			}
			c.msgs <- raw
		}
	}()
	t.Cleanup(func() { conn.Close() })
	return c
}

func (c *wsClient) Close() { c.conn.Close() }

func readAlert(t *testing.T, c *wsClient) map[string]any {
	t.Helper()
	select {
	case raw, ok := <-c.msgs:
		if !ok {
			t.Fatal("connection closed while waiting for an alert")
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatal(err)
		}
		return m
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for an alert")
		return nil
	}
}

func expectNoAlert(t *testing.T, c *wsClient) {
	t.Helper()
	select {
	case raw := <-c.msgs:
		t.Fatalf("unexpected alert: %s", raw)
	case <-time.After(400 * time.Millisecond):
	}
}

func waitForClients(t *testing.T, h *harness, n int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for h.hub.ClientCount() != n {
		if time.Now().After(deadline) {
			t.Fatalf("hub has %d clients, want %d", h.hub.ClientCount(), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRealtimeAlerts(t *testing.T) {
	h := newHarness(t)
	geoID := h.createGeofence("Restricted Yard", "restricted_zone", readmePolygon)
	vehID := h.createVehicle()
	h.mustStatus(201, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "vehicle_id": vehID, "event_type": "entry"})

	c1, c2 := dialWS(t, h), dialWS(t, h)
	waitForClients(t, h, 2)

	// Entry → both clients get the alert in the exact README format.
	h.move(vehID, 37.7799, -122.4144, "2025-01-15T10:35:00Z")
	for _, c := range []*wsClient{c1, c2} {
		m := readAlert(t, c)
		assertKeys(t, "ws message", m, "event_id", "event_type", "timestamp", "vehicle", "geofence", "location")
		if len(m) != 6 {
			t.Fatalf("ws message must have exactly the README fields, got %v", m)
		}
		assertKeys(t, "ws vehicle", m["vehicle"].(map[string]any), "vehicle_id", "vehicle_number", "driver_name")
		assertKeys(t, "ws geofence", m["geofence"].(map[string]any), "geofence_id", "geofence_name", "category")
		assertKeys(t, "ws location", m["location"].(map[string]any), "latitude", "longitude")
		if m["event_type"] != "entry" || m["timestamp"] != "2025-01-15T10:35:00Z" ||
			m["geofence"].(map[string]any)["category"] != "restricted_zone" ||
			m["vehicle"].(map[string]any)["driver_name"] != "John Doe" ||
			!strings.HasPrefix(m["event_id"].(string), "evt_") {
			t.Fatalf("unexpected ws message %v", m)
		}
	}

	// Exit is NOT configured for this rule → event recorded in history, no alert.
	h.move(vehID, 0, 0, "2025-01-15T10:40:00Z")
	expectNoAlert(t, c1)
	if n := h.mustStatus(200, "GET", "/violations/history?vehicle_id="+vehID, nil)["total_count"].(float64); n != 2 {
		t.Fatalf("history should contain entry+exit, got %v", n)
	}

	// Client 2 disconnects; client 1 keeps receiving.
	c2.Close()
	waitForClients(t, h, 1)
	h.move(vehID, 37.7799, -122.4144, "2025-01-15T10:45:00Z")
	if m := readAlert(t, c1); m["event_type"] != "entry" {
		t.Fatalf("got %v", m)
	}

	// Reconnect works.
	c3 := dialWS(t, h)
	waitForClients(t, h, 2)
	h.move(vehID, 0, 0, "2025-01-15T10:50:00Z")
	h.mustStatus(201, "POST", "/alerts/configure", map[string]any{"geofence_id": geoID, "event_type": "both"}) // all vehicles
	h.move(vehID, 37.7799, -122.4144, "2025-01-15T10:55:00Z")
	// Vehicle rule + all-vehicles rule both match: still exactly ONE alert.
	readAlert(t, c3)
	expectNoAlert(t, c3)

	// Persistence: every fired alert is in the database.
	events := h.mustStatus(200, "GET", "/alerts/events", nil)["events"].([]any)
	if len(events) != 3 {
		t.Fatalf("want 3 persisted alerts, got %d", len(events))
	}
}

// ---------- Cross-cutting ----------

func TestEveryResponseHasTimeNS(t *testing.T) {
	h := newHarness(t)
	// h.do asserts time_ns on each of these, including errors and unknown routes.
	h.mustStatus(200, "GET", "/health", nil)
	h.mustStatus(404, "GET", "/does-not-exist", nil)
	h.mustStatus(405, "DELETE", "/geofences", nil)
	h.mustStatus(404, "GET", "/geofences/geo_missing", nil)
	h.mustStatus(400, "POST", "/vehicles", "not json")
	h.mustStatus(200, "GET", "/vehicles/locations", nil)
}

func TestCORS(t *testing.T) {
	h := newHarness(t)
	req, _ := http.NewRequest("OPTIONS", h.srv.URL+"/geofences", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	req.Header.Set("Access-Control-Request-Method", "POST")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 || resp.Header.Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Fatalf("preflight failed: %d %v", resp.StatusCode, resp.Header)
	}
	req.Header.Set("Origin", "https://evil.example")
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("unknown origin must not be allowed")
	}
}

func TestAPIKey(t *testing.T) {
	h := newHarness(t, func(o *Options) { o.APIKey = "secret" })
	h.mustStatus(401, "POST", "/vehicles", map[string]any{})
	h.mustStatus(200, "GET", "/vehicles", nil) // reads stay public
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
