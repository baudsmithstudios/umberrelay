package device

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"scrye/internal/store"
)

// RunSSDP starts listening for SSDP announcements on 239.255.255.250:1900. Blocks until done is closed.
func (t *Tracker) RunSSDP(done <-chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp4", "239.255.255.250:1900")
	if err != nil {
		log.Printf("ssdp resolve: %v", err)
		return
	}

	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		log.Printf("ssdp listener: %v", err)
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
		t.parseSSDP(buf[:n], src.IP.String())
	}
}

func (t *Tracker) parseSSDP(pkt []byte, srcIP string) {
	req, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(pkt)))
	if err != nil {
		t.parseSSDPHeaders(pkt, srcIP)
		return
	}
	defer req.Body.Close()

	server := req.Header.Get("SERVER")
	if server == "" {
		server = req.Header.Get("Server")
	}
	t.upsertFromSSDP(srcIP, server)
}

func (t *Tracker) parseSSDPHeaders(pkt []byte, srcIP string) {
	scanner := bufio.NewScanner(bytes.NewReader(pkt))
	server := ""
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(strings.ToUpper(line), "SERVER:") {
			server = strings.TrimSpace(line[7:])
			break
		}
	}
	if server != "" {
		t.upsertFromSSDP(srcIP, server)
	}
}

func (t *Tracker) upsertFromSSDP(srcIP, server string) {
	if server == "" {
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
			FirstSeen: now,
			LastSeen:  now,
		})
	}
	log.Printf("ssdp: %s server=%q", mac, server)
}
