package device

import (
	"strings"
)

// OUIDB is an in-memory MAC prefix to vendor name lookup.
type OUIDB struct {
	prefixes map[string]string // "AA:BB:CC" -> "Vendor"
}

// NewOUIDB creates an OUI database from a prefix map.
// Keys must be uppercase "AA:BB:CC" format.
func NewOUIDB(prefixes map[string]string) *OUIDB {
	return &OUIDB{prefixes: prefixes}
}

// Lookup returns the vendor name for a MAC address, or empty string if unknown.
func (db *OUIDB) Lookup(mac string) string {
	mac = strings.ToUpper(mac)
	if len(mac) < 8 {
		return ""
	}
	prefix := mac[:8] // "AA:BB:CC"
	return db.prefixes[prefix]
}
