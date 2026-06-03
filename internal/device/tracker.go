package device

import (
	"bufio"
	"bytes"
	"context"
	"log"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"umberrelay/internal/store"
)

const (
	deviceWriteBufferSize = 1024
	deviceBatchSize       = 64
	deviceFlushInterval   = time.Second
)

type arpEntry struct {
	IP  string
	MAC string
}

type pendingDevice struct {
	dev    store.Device
	source string
}

type Tracker struct {
	db           *store.DB
	oui          *OUIDB
	arpCache     sync.Map // IP -> MAC
	deviceWrites chan pendingDevice
	writerActive atomic.Bool
}

func NewTracker(db *store.DB, oui *OUIDB) *Tracker {
	return &Tracker{
		db:           db,
		oui:          oui,
		deviceWrites: make(chan pendingDevice, deviceWriteBufferSize),
	}
}

func (t *Tracker) ResolveIP(ip string) string {
	if v, ok := t.arpCache.Load(ip); ok {
		return v.(string)
	}
	return ""
}

func (t *Tracker) SetARPEntry(ip, mac string) {
	t.arpCache.Store(ip, mac)
}

func (t *Tracker) saveDiscoveredDevice(dev store.Device, source string) {
	if t.db == nil {
		return
	}
	if t.writerActive.Load() {
		select {
		case t.deviceWrites <- pendingDevice{dev: dev, source: source}:
			return
		default:
			// Buffer full — fall through to a synchronous write so we never
			// drop a discovery on Pi hardware under burst load.
		}
	}
	if err := t.db.UpsertDevice(dev); err != nil {
		log.Printf("%s device save %s: %v", source, redactIdentifier(dev.MAC), err)
	}
}

func (t *Tracker) runDeviceWriter(ctx context.Context) {
	t.writerActive.Store(true)
	defer t.writerActive.Store(false)

	buf := make([]pendingDevice, 0, deviceBatchSize)
	flush := func() {
		if len(buf) == 0 {
			return
		}
		devs := make([]store.Device, len(buf))
		for i, p := range buf {
			devs[i] = p.dev
		}
		if err := t.db.UpsertDevices(devs); err != nil {
			log.Printf("%s device save batch (%d): %v", buf[0].source, len(buf), err)
		}
		buf = buf[:0]
	}

	ticker := time.NewTicker(deviceFlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			for {
				select {
				case p := <-t.deviceWrites:
					buf = append(buf, p)
					if len(buf) >= deviceBatchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		case p := <-t.deviceWrites:
			buf = append(buf, p)
			if len(buf) >= deviceBatchSize {
				flush()
			}
		case <-ticker.C:
			flush()
		}
	}
}

func (t *Tracker) Run(ctx context.Context) {
	if t.db != nil {
		go t.runDeviceWriter(ctx)
	}

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
		t.SetARPEntry(e.IP, e.MAC)
		vendor := ""
		if t.oui != nil {
			vendor = t.oui.Lookup(e.MAC)
		}
		t.saveDiscoveredDevice(store.Device{
			MAC:       e.MAC,
			IP:        e.IP,
			Vendor:    vendor,
			FirstSeen: now,
			LastSeen:  now,
		}, "arp")
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
