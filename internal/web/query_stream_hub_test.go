package web

import (
	"sync/atomic"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func TestQueryStreamHubPollsOnceAndBroadcastsToAllSubscribers(t *testing.T) {
	var fetchCalls atomic.Int32
	hub := newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		fetchCalls.Add(1)
		if afterID != 0 {
			t.Fatalf("afterID = %d, want 0", afterID)
		}
		return []store.Query{
			{ID: 1, Domain: "ads.example.com", QueryType: "A", Category: "tracking"},
		}, nil
	}, 100*time.Millisecond, 100)
	defer hub.Close()

	first, cancelFirst := hub.Subscribe()
	defer cancelFirst()
	second, cancelSecond := hub.Subscribe()
	defer cancelSecond()

	hub.pollOnce()

	select {
	case query := <-first:
		if query.ID != 1 {
			t.Fatalf("first subscriber query ID = %d, want 1", query.ID)
		}
	default:
		t.Fatalf("first subscriber did not receive broadcast query")
	}

	select {
	case query := <-second:
		if query.ID != 1 {
			t.Fatalf("second subscriber query ID = %d, want 1", query.ID)
		}
	default:
		t.Fatalf("second subscriber did not receive broadcast query")
	}

	if fetchCalls.Load() != 1 {
		t.Fatalf("fetch call count = %d, want 1", fetchCalls.Load())
	}
}

func TestQueryStreamHubAdvanceCursorSkipsOlderRows(t *testing.T) {
	hub := newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		if afterID != 5 {
			t.Fatalf("afterID = %d, want 5", afterID)
		}
		return []store.Query{
			{ID: 3, Domain: "old.example.com"},
			{ID: 6, Domain: "new.example.com"},
		}, nil
	}, 100*time.Millisecond, 100)
	defer hub.Close()

	stream, cancel := hub.Subscribe()
	defer cancel()
	hub.AdvanceCursor(5)

	hub.pollOnce()

	select {
	case query := <-stream:
		if query.ID != 6 {
			t.Fatalf("query ID = %d, want 6", query.ID)
		}
	default:
		t.Fatalf("subscriber did not receive newer query")
	}
}
