package device

import (
	"strings"
)

type OUIDB struct {
	prefixes map[string]string // "AA:BB:CC" -> "Vendor"
}

// Keys must be uppercase "AA:BB:CC" format.
func NewOUIDB(prefixes map[string]string) *OUIDB {
	return &OUIDB{prefixes: prefixes}
}

func (db *OUIDB) Lookup(mac string) string {
	mac = strings.ToUpper(mac)
	if len(mac) < 8 {
		return ""
	}
	prefix := mac[:8] // "AA:BB:CC"
	return db.prefixes[prefix]
}
