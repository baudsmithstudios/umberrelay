package device

import (
	"log"
	"net"
	"strings"
	"time"

	"scrye/internal/store"

	mdns "github.com/miekg/dns"
)

// RunMDNS starts listening for mDNS announcements on 224.0.0.251:5353. Blocks until done is closed.
func (t *Tracker) RunMDNS(done <-chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp4", "224.0.0.251:5353")
	if err != nil {
		log.Printf("mdns resolve: %v", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("mdns listener: %v", err)
		return
	}
	defer conn.Close()

	go func() {
		<-done
		conn.Close()
	}()

	buf := make([]byte, 4096)
	for {
		n, src, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		t.parseMDNS(buf[:n], src.IP.String())
	}
}

func (t *Tracker) parseMDNS(pkt []byte, srcIP string) {
	msg := new(mdns.Msg)
	if err := msg.Unpack(pkt); err != nil {
		return
	}

	hostname := ""
	for _, rr := range msg.Answer {
		switch r := rr.(type) {
		case *mdns.PTR:
			name := extractMDNSHostname(r.Ptr)
			if name != "" {
				hostname = name
			}
		case *mdns.SRV:
			name := extractMDNSHostname(r.Target)
			if name != "" {
				hostname = name
			}
		}
	}

	if hostname == "" {
		return
	}

	mac := t.ResolveIP(srcIP)
	if mac == "" {
		return
	}

	now := time.Now()
	if t.db != nil {
		t.db.UpsertDevice(store.Device{
			MAC:       mac,
			Hostname:  hostname,
			FirstSeen: now,
			LastSeen:  now,
		})
	}
	log.Printf("mdns: %s hostname=%q", mac, hostname)
}

// extractMDNSHostname extracts a clean hostname from an mDNS name like "MyDevice._http._tcp.local."
func extractMDNSHostname(name string) string {
	name = strings.TrimSuffix(name, ".")
	name = strings.TrimSuffix(name, ".local")
	if idx := strings.Index(name, "._"); idx > 0 {
		name = name[:idx]
	}
	if name == "" || strings.HasPrefix(name, "_") {
		return ""
	}
	return name
}
