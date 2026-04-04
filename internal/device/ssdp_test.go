package device

import "testing"

func TestParseSSDPPersistsDeviceWhenServerHeaderPresent(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)
	tracker.SetARPEntry("192.168.1.15", "aa:bb:cc:dd:ee:ff")

	packet := []byte("NOTIFY * HTTP/1.1\r\nHOST: 239.255.255.250:1900\r\nSERVER: Roku/1.0 UPnP/1.1\r\n\r\n")
	tracker.parseSSDP(packet, "192.168.1.15")

	if _, err := db.GetDevice("aa:bb:cc:dd:ee:ff"); err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
}

func TestParseSSDPHeadersFallbackPersistsDevice(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)
	tracker.SetARPEntry("192.168.1.16", "11:22:33:44:55:66")

	packet := []byte("NOTIFY * HTTP/1.1\nHOST: 239.255.255.250:1900\nSERVER: Sonos/1.0\n\n")
	tracker.parseSSDPHeaders(packet, "192.168.1.16")

	if _, err := db.GetDevice("11:22:33:44:55:66"); err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
}

func TestUpsertFromSSDPIgnoresEmptyServer(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)
	tracker.SetARPEntry("192.168.1.20", "aa:bb:cc:dd:ee:ff")

	tracker.upsertFromSSDP("192.168.1.20", "")

	if _, err := db.GetDevice("aa:bb:cc:dd:ee:ff"); err == nil {
		t.Fatal("GetDevice succeeded for empty server header, want not found")
	}
}
