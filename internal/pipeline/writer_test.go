package pipeline

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"umberrelay/internal/classify"
	"umberrelay/internal/device"
	"umberrelay/internal/dns"
	"umberrelay/internal/store"
)

func testSetup(t *testing.T) (*store.DB, *device.Tracker, *classify.Manager) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	tracker := device.NewTracker(db, device.NewOUIDB(nil))
	tracker.SetARPEntry("192.168.1.10", "aa:bb:cc:dd:ee:ff")

	mgr := classify.NewManager(db)

	return db, tracker, mgr
}

func TestWriterProcessesRecords(t *testing.T) {
	db, tracker, mgr := testSetup(t)

	// Seed device
	db.UpsertDevice(store.Device{
		MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10",
		FirstSeen: time.Now(), LastSeen: time.Now(),
	})

	ch := make(chan dns.QueryRecord, 10)
	flushed := make(chan struct{}, 1)
	w := NewWriter(ch, db, tracker, mgr, Config{
		BatchSize:     1,
		FlushInterval: 10 * time.Second,
		OnFlush: func() {
			select {
			case flushed <- struct{}{}:
			default:
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	go w.Run(ctx)

	ch <- dns.QueryRecord{
		SourceIP:  "192.168.1.10",
		Domain:    "example.com.",
		QueryType: "A",
		Timestamp: time.Now(),
	}

	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for flush")
	}
	cancel()

	queries, err := db.QueryLog("", "", time.Time{}, time.Now().Add(time.Minute), 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(queries) != 1 {
		t.Fatalf("got %d queries, want 1", len(queries))
	}
	if queries[0].Domain != "example.com." {
		t.Errorf("domain = %q, want example.com.", queries[0].Domain)
	}
	if queries[0].DeviceMAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("device_mac = %q, want aa:bb:cc:dd:ee:ff", queries[0].DeviceMAC)
	}
	if queries[0].SourceIP != "192.168.1.10" {
		t.Errorf("source_ip = %q, want 192.168.1.10", queries[0].SourceIP)
	}
}

func TestWriterDrainsOnShutdown(t *testing.T) {
	db, tracker, mgr := testSetup(t)

	db.UpsertDevice(store.Device{
		MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10",
		FirstSeen: time.Now(), LastSeen: time.Now(),
	})

	ch := make(chan dns.QueryRecord, 10)
	flushed := make(chan struct{}, 1)
	w := NewWriter(ch, db, tracker, mgr, Config{
		BatchSize:     100,
		FlushInterval: 10 * time.Second, // Long interval so it doesn't auto-flush
		OnFlush: func() {
			select {
			case flushed <- struct{}{}:
			default:
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()

	ch <- dns.QueryRecord{
		SourceIP: "192.168.1.10", Domain: "drain.test.",
		QueryType: "A", Timestamp: time.Now(),
	}

	// Cancel immediately — writer should drain
	cancel()
	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for drain flush")
	}
	<-done

	queries, _ := db.QueryLog("", "", time.Time{}, time.Now().Add(time.Minute), 100, 0)
	if len(queries) != 1 {
		t.Fatalf("got %d queries after drain, want 1", len(queries))
	}
}

func TestWriterCallsOnFlushHookAfterSuccessfulWrite(t *testing.T) {
	db, tracker, mgr := testSetup(t)

	db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	})

	var flushCalls atomic.Int32
	flushed := make(chan struct{}, 1)
	ch := make(chan dns.QueryRecord, 10)
	w := NewWriter(ch, db, tracker, mgr, Config{
		BatchSize:     1,
		FlushInterval: 5 * time.Second,
		OnFlush: func() {
			flushCalls.Add(1)
			select {
			case flushed <- struct{}{}:
			default:
			}
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go w.Run(ctx)

	ch <- dns.QueryRecord{
		SourceIP:  "192.168.1.10",
		Domain:    "flush-hook.test.",
		QueryType: "A",
		Timestamp: time.Now(),
	}

	select {
	case <-flushed:
	case <-time.After(2 * time.Second):
		t.Fatalf("flush hook calls = %d, want 1", flushCalls.Load())
	}
	if flushCalls.Load() != 1 {
		t.Fatalf("flush hook calls = %d, want 1", flushCalls.Load())
	}
}
