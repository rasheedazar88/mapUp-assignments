// Package tracking contains the pure entry/exit detection rule.
package tracking

import "sort"

// Diff compares the geofences a vehicle was inside BEFORE this location update
// with the geofences it is inside NOW.
//
//	entered = now − before   (inside now, was not before)  → "entry" events
//	exited  = before − now   (was inside, is not now)      → "exit" events
//
// Geofences in both sets produce no event. That is what prevents duplicate
// alerts when a vehicle reports several positions inside the same zone.
// Results are sorted so the output is deterministic (stable tests, stable logs).
func Diff(before, now []string) (entered, exited []string) {
	beforeSet := toSet(before)
	nowSet := toSet(now)

	for id := range nowSet {
		if _, wasInside := beforeSet[id]; !wasInside {
			entered = append(entered, id)
		}
	}
	for id := range beforeSet {
		if _, stillInside := nowSet[id]; !stillInside {
			exited = append(exited, id)
		}
	}
	sort.Strings(entered)
	sort.Strings(exited)
	return entered, exited
}

// Go has no built-in set type; a map with empty-struct values is the idiom
// (struct{} takes zero bytes).
func toSet(ids []string) map[string]struct{} {
	s := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		s[id] = struct{}{}
	}
	return s
}
