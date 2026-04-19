package device

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"umberrelay/internal/store"
)

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
	t.saveDiscoveredDevice(store.Device{
		MAC:       mac,
		FirstSeen: now,
		LastSeen:  now,
	}, "ssdp")
	log.Printf(
		"ssdp discovery: mac=%s server=%s",
		redactIdentifier(mac),
		redactIdentifier(server),
	)
}
