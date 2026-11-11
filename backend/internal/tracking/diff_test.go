package tracking

import (
	"reflect"
	"testing"
)

func TestDiff(t *testing.T) {
	tests := []struct {
		name                    string
		before, now             []string
		wantEntered, wantExited []string
	}{
		{"first location outside everything", nil, nil, nil, nil},
		{"first location inside a zone = entry", nil, []string{"geo_a"}, []string{"geo_a"}, nil},
		{"still inside = no duplicate event", []string{"geo_a"}, []string{"geo_a"}, nil, nil},
		{"leaving = exit", []string{"geo_a"}, nil, nil, []string{"geo_a"}},
		{"moving between zones = exit + entry", []string{"geo_a"}, []string{"geo_b"}, []string{"geo_b"}, []string{"geo_a"}},
		{"entering an overlapping zone while inside another",
			[]string{"geo_a"}, []string{"geo_a", "geo_b"}, []string{"geo_b"}, nil},
		{"leaving two zones at once", []string{"geo_b", "geo_a"}, nil, nil, []string{"geo_a", "geo_b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entered, exited := Diff(tt.before, tt.now)
			if !reflect.DeepEqual(entered, tt.wantEntered) {
				t.Errorf("entered = %v, want %v", entered, tt.wantEntered)
			}
			if !reflect.DeepEqual(exited, tt.wantExited) {
				t.Errorf("exited = %v, want %v", exited, tt.wantExited)
			}
		})
	}
}
