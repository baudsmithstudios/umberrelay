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
	}, 100)
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
	}, 100)
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

func TestQueryStreamHubOnlyPollsAfterNotify(t *testing.T) {
	var fetchCalls atomic.Int32
	hub := newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		call := fetchCalls.Add(1)
		return []store.Query{
			{ID: int64(call), Domain: "ads.example.com"},
		}, nil
	}, 100)
	defer hub.Close()

	stream, cancel := hub.Subscribe()
	defer cancel()

	select {
	case query := <-stream:
		if query.ID != 1 {
			t.Fatalf("initial query ID = %d, want 1", query.ID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("subscriber did not receive initial query after subscribe")
	}

	hub.NotifyNewQueries()

	select {
	case query := <-stream:
		if query.ID != 2 {
			t.Fatalf("query ID = %d, want 2", query.ID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("subscriber did not receive query after notify")
	}

	if fetchCalls.Load() < 2 {
		t.Fatalf("fetch call count after notify = %d, want at least 2", fetchCalls.Load())
	}
}

func TestQueryStreamHubPollOnceDrainsMultipleBatches(t *testing.T) {
	var fetchCalls atomic.Int32
	hub := newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		fetchCalls.Add(1)
		switch afterID {
		case 0:
			return []store.Query{
				{ID: 1, Domain: "one.example.com"},
				{ID: 2, Domain: "two.example.com"},
			}, nil
		case 2:
			return []store.Query{
				{ID: 3, Domain: "three.example.com"},
				{ID: 4, Domain: "four.example.com"},
			}, nil
		case 4:
			return []store.Query{
				{ID: 5, Domain: "five.example.com"},
			}, nil
		default:
			return nil, nil
		}
	}, 2)
	defer hub.Close()

	stream, cancel := hub.Subscribe()
	defer cancel()

	hub.pollOnce()

	var got []int64
	for i := 0; i < 5; i++ {
		select {
		case q := <-stream:
			got = append(got, q.ID)
		default:
			t.Fatalf("missing streamed query at index %d; got IDs: %v", i, got)
		}
	}

	want := []int64{1, 2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got IDs %v, want %v", got, want)
		}
	}
}

func TestQueryStreamHubSubscribeTriggersInitialPoll(t *testing.T) {
	hub := newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		if afterID != 0 {
			return nil, nil
		}
		return []store.Query{
			{ID: 1, Domain: "initial.example.com"},
		}, nil
	}, 100)
	defer hub.Close()

	stream, cancel := hub.Subscribe()
	defer cancel()

	select {
	case query := <-stream:
		if query.ID != 1 {
			t.Fatalf("query ID = %d, want 1", query.ID)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatalf("subscriber did not receive initial backlog query")
	}
}
