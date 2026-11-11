package geo

import (
	"math"
	"reflect"
	"strings"
	"testing"
)

// The exact polygon from the assessment README.
var readmePolygon = [][]float64{
	{37.7749, -122.4194},
	{37.7849, -122.4194},
	{37.7849, -122.4094},
	{37.7749, -122.4094},
	{37.7749, -122.4194},
}

func TestValidatePolygon(t *testing.T) {
	tests := []struct {
		name    string
		coords  [][]float64
		wantErr string // substring; "" = valid
	}{
		{"readme example is valid", readmePolygon, ""},
		{"triangle (minimum)", [][]float64{{0, 0}, {1, 0}, {0, 1}, {0, 0}}, ""},
		{"too few points", [][]float64{{0, 0}, {1, 0}, {0, 0}}, "at least 4 points"},
		{"empty", nil, "at least 4 points"},
		{"not closed", [][]float64{{0, 0}, {1, 0}, {1, 1}, {0, 1}}, "first and last"},
		{"latitude too high", [][]float64{{91, 0}, {1, 0}, {0, 1}, {91, 0}}, "latitude"},
		{"latitude too low", [][]float64{{-90.5, 0}, {1, 0}, {0, 1}, {-90.5, 0}}, "latitude"},
		{"longitude out of range", [][]float64{{0, 181}, {1, 0}, {0, 1}, {0, 181}}, "longitude"},
		{"NaN rejected", [][]float64{{math.NaN(), 0}, {1, 0}, {0, 1}, {0, 0}}, "latitude"},
		{"point with 3 numbers", [][]float64{{0, 0, 5}, {1, 0}, {0, 1}, {0, 0}}, "pair"},
		{"point with 1 number", [][]float64{{0}, {1, 0}, {0, 1}, {0}}, "pair"},
		{"only 2 unique points", [][]float64{{0, 0}, {1, 1}, {1, 1}, {0, 0}}, "3 unique"},
		{"collinear points", [][]float64{{0, 0}, {1, 1}, {2, 2}, {0, 0}}, "zero area"},
		{"boundary values allowed", [][]float64{{-90, -180}, {90, -180}, {90, 180}, {-90, -180}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePolygon(tt.coords)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected valid, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestValidateLatLng(t *testing.T) {
	ok := [][2]float64{{0, 0}, {90, 180}, {-90, -180}, {37.7849, -122.4194}}
	for _, p := range ok {
		if err := ValidateLatLng(p[0], p[1]); err != nil {
			t.Errorf("%v should be valid: %v", p, err)
		}
	}
	bad := [][2]float64{{90.0001, 0}, {0, -180.0001}, {math.Inf(1), 0}, {0, math.NaN()}}
	for _, p := range bad {
		if err := ValidateLatLng(p[0], p[1]); err == nil {
			t.Errorf("%v should be invalid", p)
		}
	}
}

// The single most important test in this package: the API is [lat, lng]
// but WKT must be (lng lat).
func TestPolygonWKTSwapsToLngLat(t *testing.T) {
	got := PolygonWKT(readmePolygon)
	want := "POLYGON((-122.4194 37.7749, -122.4194 37.7849, -122.4094 37.7849, -122.4094 37.7749, -122.4194 37.7749))"
	if got != want {
		t.Fatalf("\n got: %s\nwant: %s", got, want)
	}
}

func TestPolygonFromGeoJSONSwapsBackToLatLng(t *testing.T) {
	raw := []byte(`{"type":"Polygon","coordinates":[[[-122.4194,37.7749],[-122.4194,37.7849],[-122.4094,37.7849],[-122.4094,37.7749],[-122.4194,37.7749]]]}`)
	got, err := PolygonFromGeoJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, readmePolygon) {
		t.Fatalf("round trip mismatch:\n got %v\nwant %v", got, readmePolygon)
	}
}

func TestPolygonFromGeoJSONRejectsNonPolygon(t *testing.T) {
	if _, err := PolygonFromGeoJSON([]byte(`{"type":"Point","coordinates":[1,2]}`)); err == nil {
		t.Fatal("expected error for Point")
	}
}
