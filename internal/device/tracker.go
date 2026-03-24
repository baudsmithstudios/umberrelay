package device

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"scrye/internal/store"
)

type arpEntry struct {
	IP  string
	MAC string
}

// Tracker maintains the device inventory via passive observation.
type Tracker struct {
	db       *store.DB
	oui      *OUIDB
	arpCache sync.Map // IP -> MAC
}

// NewTracker creates a device tracker.
func NewTracker(db *store.DB, oui *OUIDB) *Tracker {
	return &Tracker{db: db, oui: oui}
}

// ResolveIP returns the MAC address for an IP, or empty string if unknown.
func (t *Tracker) ResolveIP(ip string) string {
	if v, ok := t.arpCache.Load(ip); ok {
		return v.(string)
	}
	return ""
}

// SetARPEntry manually adds an IP->MAC mapping (for testing).
func (t *Tracker) SetARPEntry(ip, mac string) {
	t.arpCache.Store(ip, mac)
}

// Run starts the ARP polling loop and broadcast listeners. Blocks until ctx is cancelled.
func (t *Tracker) Run(ctx context.Context) {
	t.pollARP()

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		close(done)
	}()

	go t.RunDHCP(done)
	go t.RunMDNS(done)
	go t.RunSSDP(done)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			t.pollARP()
		}
	}
}

func (t *Tracker) pollARP() {
	data, err := os.ReadFile("/proc/net/arp")
	if err != nil {
		log.Printf("read arp table: %v", err)
		return
	}
	entries := parseARPTable(data)
	now := time.Now()
	for _, e := range entries {
		t.arpCache.Store(e.IP, e.MAC)
		if t.db != nil {
			vendor := ""
			if t.oui != nil {
				vendor = t.oui.Lookup(e.MAC)
			}
			t.db.UpsertDevice(store.Device{
				MAC:       e.MAC,
				IP:        e.IP,
				Vendor:    vendor,
				FirstSeen: now,
				LastSeen:  now,
			})
		}
	}
}

func parseARPTable(data []byte) []arpEntry {
	var entries []arpEntry
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Scan() // skip header
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 4 {
			mac := fields[3]
			if mac == "00:00:00:00:00:00" {
				continue
			}
			entries = append(entries, arpEntry{IP: fields[0], MAC: mac})
		}
	}
	return entries
}
