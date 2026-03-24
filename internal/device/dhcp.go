package device

import (
	"log"
	"net"
	"time"

	"scrye/internal/store"
)

// RunDHCP starts listening for DHCP packets on UDP port 67. Blocks until done is closed.
func (t *Tracker) RunDHCP(done <-chan struct{}) {
	conn, err := net.ListenPacket("udp4", ":67")
	if err != nil {
		log.Printf("dhcp listener: %v (run as root?)", err)
		return
	}
	defer conn.Close()

	go func() {
		<-done
		conn.Close()
	}()

	buf := make([]byte, 1500)
	for {
		n, _, err := conn.ReadFrom(buf)
		if err != nil {
			return
		}
		if n < 240 {
			continue
		}
		t.parseDHCP(buf[:n])
	}
}

func (t *Tracker) parseDHCP(pkt []byte) {
	// DHCP packet layout:
	// byte 0: op (1=request, 2=reply)
	// byte 12-15: ciaddr (client IP)
	// byte 28-33: chaddr (first 6 bytes are MAC)
	// byte 236-239: magic cookie
	// byte 240+: options

	if pkt[0] != 1 {
		return // Only process client requests
	}

	// Extract MAC from chaddr (offset 28, 6 bytes)
	mac := net.HardwareAddr(pkt[28:34]).String()
	if mac == "00:00:00:00:00:00" {
		return
	}

	// Parse options starting at byte 240 (after magic cookie)
	hostname := ""
	if len(pkt) > 240 {
		hostname = parseDHCPOption12(pkt[240:])
	}

	if hostname == "" {
		return
	}

	clientIP := net.IP(pkt[12:16]).String()

	now := time.Now()
	if clientIP != "0.0.0.0" {
		t.arpCache.Store(clientIP, mac)
	}
	if t.db != nil {
		dev := store.Device{
			MAC:       mac,
			Hostname:  hostname,
			FirstSeen: now,
			LastSeen:  now,
		}
		if clientIP != "0.0.0.0" {
			dev.IP = clientIP
		}
		t.db.UpsertDevice(dev)
	}
	log.Printf("dhcp: %s (%s) hostname=%q", mac, clientIP, hostname)
}

// parseDHCPOption12 extracts the hostname (option 12) from DHCP options.
func parseDHCPOption12(opts []byte) string {
	for i := 0; i < len(opts)-1; {
		optType := opts[i]
		if optType == 255 { // End
			break
		}
		if optType == 0 { // Padding
			i++
			continue
		}
		if i+1 >= len(opts) {
			break
		}
		optLen := int(opts[i+1])
		if i+2+optLen > len(opts) {
			break
		}
		if optType == 12 {
			return string(opts[i+2 : i+2+optLen])
		}
		i += 2 + optLen
	}
	return ""
}
