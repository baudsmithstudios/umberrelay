package device

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"strings"
	"testing"
	"time"

	"umberrelay/internal/store"
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

func TestSaveDiscoveredDeviceLogsStoreError(t *testing.T) {
	db := testDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	tracker := NewTracker(db, nil)

	var logs bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&logs)
	t.Cleanup(func() {
		log.SetOutput(prev)
	})

	tracker.saveDiscoveredDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}, "dhcp")

	if !strings.Contains(logs.String(), "dhcp device save") {
		t.Fatalf("expected device save error log, got %q", logs.String())
	}
}

func TestDeviceWriterCoalescesAndDrainsOnCancel(t *testing.T) {
	db := testDB(t)
	tracker := NewTracker(db, nil)

	ctx, cancel := context.WithCancel(context.Background())
	writerDone := make(chan struct{})
	go func() {
		defer close(writerDone)
		tracker.runDeviceWriter(ctx)
	}()

	deadline := time.Now().Add(time.Second)
	for !tracker.writerActive.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !tracker.writerActive.Load() {
		cancel()
		t.Fatal("device writer did not become active")
	}

	now := time.Now()
	const n = 50
	for i := 0; i < n; i++ {
		mac := fmt.Sprintf("aa:bb:cc:dd:ee:%02x", i)
		tracker.saveDiscoveredDevice(store.Device{
			MAC: mac, FirstSeen: now, LastSeen: now,
		}, "mdns")
	}

	cancel()
	select {
	case <-writerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("device writer did not drain after cancel")
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != n {
		t.Fatalf("got %d devices, want %d", len(devices), n)
	}
}

func TestRunStopsWhenContextCancelled(t *testing.T) {
	tracker := NewTracker(testDB(t), nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan struct{})
	go func() {
		tracker.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancellation")
	}
}

func TestRunDHCPReturnsWhenDoneClosed(t *testing.T) {
	tracker := NewTracker(testDB(t), nil)
	done := make(chan struct{})
	close(done)

	finished := make(chan struct{})
	go func() {
		tracker.RunDHCP(done)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("RunDHCP did not return")
	}
}

func TestRunMDNSReturnsWhenDoneClosed(t *testing.T) {
	tracker := NewTracker(testDB(t), nil)
	done := make(chan struct{})
	close(done)

	finished := make(chan struct{})
	go func() {
		tracker.RunMDNS(done)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("RunMDNS did not return")
	}
}

func TestRunSSDPReturnsWhenDoneClosed(t *testing.T) {
	tracker := NewTracker(testDB(t), nil)
	done := make(chan struct{})
	close(done)

	finished := make(chan struct{})
	go func() {
		tracker.RunSSDP(done)
		close(finished)
	}()

	select {
	case <-finished:
	case <-time.After(2 * time.Second):
		t.Fatal("RunSSDP did not return")
	}
}
