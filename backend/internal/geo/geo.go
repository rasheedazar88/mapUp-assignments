// Package geo holds the pure (no database) geospatial helpers:
// validating polygons and points, and converting between the API's
// [latitude, longitude] order and PostGIS's (longitude latitude) order.
//
// Every coordinate swap in the backend goes through this file, so it is
// the one place to check when debugging "the polygon is in the wrong place".
package geo

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

const MinPolygonPoints = 4 // 3 unique corners + the closing point

// ValidateLatLng checks a single point. NaN/Inf are rejected explicitly
// because NaN fails every comparison and would otherwise slip through.
func ValidateLatLng(lat, lng float64) error {
	if math.IsNaN(lat) || math.IsInf(lat, 0) || lat < -90 || lat > 90 {
		return fmt.Errorf("latitude must be between -90 and 90")
	}
	if math.IsNaN(lng) || math.IsInf(lng, 0) || lng < -180 || lng > 180 {
		return fmt.Errorf("longitude must be between -180 and 180")
	}
	return nil
}

// ValidatePolygon checks the README rules plus a few that keep PostGIS happy.
// coords are [latitude, longitude] pairs.
//
// Self-intersection ("bow-tie" shapes) is checked afterwards by PostGIS
// ST_IsValid, which implements it robustly.
func ValidatePolygon(coords [][]float64) error {
	if len(coords) < MinPolygonPoints {
		return fmt.Errorf("polygon needs at least %d points (3 unique + 1 closing point), got %d", MinPolygonPoints, len(coords))
	}
	for i, p := range coords {
		if len(p) != 2 {
			return fmt.Errorf("point %d must be a [latitude, longitude] pair", i)
		}
		if err := ValidateLatLng(p[0], p[1]); err != nil {
			return fmt.Errorf("point %d: %w", i, err)
		}
	}

	first, last := coords[0], coords[len(coords)-1]
	if first[0] != last[0] || first[1] != last[1] {
		return fmt.Errorf("first and last coordinates must be identical (closed polygon)")
	}

	// Count distinct corners in the open ring (everything except the closing point).
	unique := map[[2]float64]struct{}{}
	for _, p := range coords[:len(coords)-1] {
		unique[[2]float64{p[0], p[1]}] = struct{}{}
	}
	if len(unique) < 3 {
		return fmt.Errorf("polygon needs at least 3 unique points, got %d", len(unique))
	}

	if math.Abs(signedArea(coords)) < 1e-12 {
		return fmt.Errorf("polygon has zero area (all points are on one line)")
	}
	return nil
}

// signedArea uses the shoelace formula. Its sign tells orientation; we only
// need "is it non-zero" to reject collinear points (a flat "polygon").
func signedArea(coords [][]float64) float64 {
	var sum float64
	for i := 0; i < len(coords)-1; i++ {
		sum += coords[i][1]*coords[i+1][0] - coords[i+1][1]*coords[i][0]
	}
	return sum / 2
}

// PolygonWKT converts [lat,lng] pairs to Well-Known Text for PostGIS.
// WKT is (x y) = (longitude latitude), so each pair is SWAPPED here.
//
//	[[37.77, -122.41], ...]  →  "POLYGON((-122.41 37.77, ...))"
//
// The WKT string is passed to SQL as a bind parameter ($1), never concatenated
// into the query, and it is built only from validated float64 values.
func PolygonWKT(coords [][]float64) string {
	var b strings.Builder
	b.WriteString("POLYGON((")
	for i, p := range coords {
		if i > 0 {
			b.WriteString(", ")
		}
		lat, lng := p[0], p[1]
		b.WriteString(strconv.FormatFloat(lng, 'f', -1, 64))
		b.WriteByte(' ')
		b.WriteString(strconv.FormatFloat(lat, 'f', -1, 64))
	}
	b.WriteString("))")
	return b.String()
}

// PolygonFromGeoJSON converts PostGIS ST_AsGeoJSON output back to API order.
// GeoJSON polygons are {"type":"Polygon","coordinates":[ring, ...]} where each
// position is [longitude, latitude] — so we SWAP back to [latitude, longitude].
// Only the outer ring is returned (the API has no concept of holes).
func PolygonFromGeoJSON(raw []byte) ([][]float64, error) {
	var g struct {
		Type        string        `json:"type"`
		Coordinates [][][]float64 `json:"coordinates"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		return nil, fmt.Errorf("decode geojson: %w", err)
	}
	if g.Type != "Polygon" || len(g.Coordinates) == 0 {
		return nil, fmt.Errorf("expected a Polygon, got %q", g.Type)
	}
	ring := g.Coordinates[0]
	out := make([][]float64, len(ring))
	for i, pos := range ring {
		if len(pos) < 2 {
			return nil, fmt.Errorf("invalid position at %d", i)
		}
		out[i] = []float64{pos[1], pos[0]} // [lng, lat] → [lat, lng]
	}
	return out, nil
}
