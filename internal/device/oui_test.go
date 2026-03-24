package device

import (
	"testing"
)

func TestOUILookup(t *testing.T) {
	db := NewOUIDB(map[string]string{
		"AA:BB:CC": "TestVendor",
		"11:22:33": "AnotherVendor",
	})

	tests := []struct {
		mac  string
		want string
	}{
		{"aa:bb:cc:dd:ee:ff", "TestVendor"},
		{"AA:BB:CC:DD:EE:FF", "TestVendor"},
		{"11:22:33:44:55:66", "AnotherVendor"},
		{"ff:ff:ff:ff:ff:ff", ""},
	}
	for _, tt := range tests {
		got := db.Lookup(tt.mac)
		if got != tt.want {
			t.Errorf("Lookup(%q) = %q, want %q", tt.mac, got, tt.want)
		}
	}
}
