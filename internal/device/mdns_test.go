package device

import (
	"testing"

	mdns "github.com/miekg/dns"
)

func TestExtractMDNSHostname(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "service name", in: "LivingRoomTV._http._tcp.local.", want: "LivingRoomTV"},
		{name: "local suffix", in: "speaker.local.", want: "speaker"},
		{name: "invalid underscore prefix", in: "_services._dns-sd._udp.local.", want: ""},
		{name: "empty", in: "", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractMDNSHostname(tt.in); got != tt.want {
				t.Fatalf("extractMDNSHostname(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestParseMDNSPersistsDiscoveredHostname(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, NewOUIDB(nil))
	tracker.SetARPEntry("192.168.1.10", "aa:bb:cc:dd:ee:ff")

	msg := new(mdns.Msg)
	msg.Answer = []mdns.RR{
		&mdns.PTR{
			Hdr: mdns.RR_Header{Name: "_services._dns-sd._udp.local.", Rrtype: mdns.TypePTR, Class: mdns.ClassINET, Ttl: 60},
			Ptr: "LivingRoomTV._http._tcp.local.",
		},
	}
	packet, err := msg.Pack()
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	tracker.parseMDNS(packet, "192.168.1.10")

	device, err := db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if device.Hostname != "LivingRoomTV" {
		t.Fatalf("hostname = %q, want %q", device.Hostname, "LivingRoomTV")
	}
}

func TestParseMDNSIgnoresUnknownSourceIP(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, NewOUIDB(nil))

	msg := new(mdns.Msg)
	msg.Answer = []mdns.RR{
		&mdns.SRV{
			Hdr:    mdns.RR_Header{Name: "_airplay._tcp.local.", Rrtype: mdns.TypeSRV, Class: mdns.ClassINET, Ttl: 60},
			Target: "BedroomTV.local.",
		},
	}
	packet, err := msg.Pack()
	if err != nil {
		t.Fatalf("Pack: %v", err)
	}

	tracker.parseMDNS(packet, "192.168.1.50")

	if _, err := db.GetDevice("aa:bb:cc:dd:ee:ff"); err == nil {
		t.Fatal("GetDevice succeeded for unknown source IP, want not found")
	}
}
