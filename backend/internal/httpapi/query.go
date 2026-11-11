package httpapi

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"geofence/internal/store"
)

// parsePage reads optional ?limit=&offset= (bonus: pagination on list endpoints).
// Without them, list endpoints return everything, exactly as the README describes.
func parsePage(q url.Values, fe fieldErrors) store.Page {
	var p store.Page
	if s := q.Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > 1000 {
			fe.add("limit", "must be an integer between 1 and 1000")
		}
		p.Limit = n
	}
	p.Offset = parseOffset(q, fe)
	return p
}

func parseOffset(q url.Values, fe fieldErrors) int {
	s := q.Get("offset")
	if s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		fe.add("offset", "must be a non-negative integer")
		return 0
	}
	return n
}

// parseTimestamp accepts ISO 8601 / RFC 3339 timestamps such as
// "2025-01-15T10:35:00Z", "2025-01-15T10:35:00.123Z" or "2025-01-15T16:05:00+05:30".
func parseTimestamp(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(s))
	return t, err == nil
}

// parseDateParam is for history filters. Besides full timestamps it accepts a
// plain date ("2025-01-15"): start_date = start of that day, end_date = end of that day (UTC).
func parseDateParam(s string, endOfDay bool) (time.Time, bool) {
	if t, ok := parseTimestamp(s); ok {
		return t, true
	}
	d, err := time.Parse("2006-01-02", strings.TrimSpace(s))
	if err != nil {
		return time.Time{}, false
	}
	if endOfDay {
		return d.Add(24*time.Hour - time.Nanosecond), true
	}
	return d, true
}
