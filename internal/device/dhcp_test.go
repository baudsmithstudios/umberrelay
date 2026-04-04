package device

import (
	"net"
	"testing"
)

func TestParseDHCPOption12(t *testing.T) {
	hostname := "living-room-tv"
	opts := []byte{
		53, 1, 1,
		12, byte(len(hostname)),
	}
	opts = append(opts, []byte(hostname)...)
	opts = append(opts, 255)

	if got := parseDHCPOption12(opts); got != hostname {
		t.Fatalf("parseDHCPOption12() = %q, want %q", got, hostname)
	}
}

func TestParseDHCPOption12ReturnsEmptyForMalformedOptions(t *testing.T) {
	opts := []byte{12, 20, 'x', 'y'}
	if got := parseDHCPOption12(opts); got != "" {
		t.Fatalf("parseDHCPOption12() = %q, want empty", got)
	}
}

func TestParseDHCPPersistsHostnameAndAddress(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)

	hostname := "kitchen-speaker"
	packet := makeDHCPDiscoverPacket("aa:bb:cc:dd:ee:ff", "192.168.1.20", hostname)
	tracker.parseDHCP(packet)

	device, err := db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if device.Hostname != hostname {
		t.Fatalf("hostname = %q, want %q", device.Hostname, hostname)
	}
	if device.IP != "192.168.1.20" {
		t.Fatalf("ip = %q, want %q", device.IP, "192.168.1.20")
	}
	if got := tracker.ResolveIP("192.168.1.20"); got != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("ResolveIP = %q, want aa:bb:cc:dd:ee:ff", got)
	}
}

func TestParseDHCPIgnoresNonRequestPackets(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)

	packet := makeDHCPDiscoverPacket("aa:bb:cc:dd:ee:ff", "192.168.1.20", "kitchen-speaker")
	packet[0] = 2
	tracker.parseDHCP(packet)

	if _, err := db.GetDevice("aa:bb:cc:dd:ee:ff"); err == nil {
		t.Fatal("GetDevice succeeded for non-request packet, want not found")
	}
}

func makeDHCPDiscoverPacket(mac, ip, hostname string) []byte {
	packet := make([]byte, 240)
	packet[0] = 1
	copy(packet[12:16], net.ParseIP(ip).To4())

	hw, _ := net.ParseMAC(mac)
	copy(packet[28:34], hw)

	options := []byte{
		53, 1, 1,
		12, byte(len(hostname)),
	}
	options = append(options, []byte(hostname)...)
	options = append(options, 255)
	return append(packet, options...)
}
