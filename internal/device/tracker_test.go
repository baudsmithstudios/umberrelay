package device

import (
	"testing"
)

func TestParseARPTable(t *testing.T) {
	content := `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.1      0x1         0x2         aa:bb:cc:dd:ee:ff     *        eth0
192.168.1.2      0x1         0x2         11:22:33:44:55:66     *        eth0
`
	entries := parseARPTable([]byte(content))
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].IP != "192.168.1.1" || entries[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("entry 0: IP=%q MAC=%q", entries[0].IP, entries[0].MAC)
	}
}

func TestResolveIP(t *testing.T) {
	tracker := NewTracker(nil, nil)
	tracker.arpCache.Store("192.168.1.10", "aa:bb:cc:dd:ee:ff")

	mac := tracker.ResolveIP("192.168.1.10")
	if mac != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("ResolveIP = %q, want aa:bb:cc:dd:ee:ff", mac)
	}

	mac = tracker.ResolveIP("192.168.1.99")
	if mac != "" {
		t.Errorf("ResolveIP unknown = %q, want empty", mac)
	}
}
