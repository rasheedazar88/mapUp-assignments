// Package ids generates prefixed, random identifiers like "geo_k3m9x2p7qa".
// Random (not sequential) IDs cannot be guessed or enumerated.
package ids

import (
	"crypto/rand"
)

const (
	Geofence = "geo_"
	Vehicle  = "veh_"
	Alert    = "alert_" // alert rule (POST /alerts/configure)
	Event    = "viol_"  // geofence entry/exit event (GET /violations/history)
	Fired    = "evt_"   // fired alert / WebSocket event_id
)

// Lowercase letters + digits without look-alikes (0/o, 1/l/i).
const alphabet = "23456789abcdefghjkmnpqrstuvwxyz"

// New returns prefix + 10 random characters (31^10 ≈ 8×10^14 combinations).
func New(prefix string) string {
	b := make([]byte, 10)
	if _, err := rand.Read(b); err != nil {
		panic("crypto/rand failed: " + err.Error()) // never happens on supported OSes
	}
	for i := range b {
		b[i] = alphabet[int(b[i])%len(alphabet)]
	}
	return prefix + string(b)
}
