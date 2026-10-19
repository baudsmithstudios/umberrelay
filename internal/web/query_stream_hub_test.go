package web

import (
	"sync"
	"testing"
	"time"

	"umberrelay/internal/store"
)

type fakeQueryFeed struct {
	mu   sync.Mutex
	rows []store.Query
}

func (f *fakeQueryFeed) add(ids ...int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, id := range ids {
		f.rows = append(f.rows, store.Query{ID: id, Domain: "example.com"})
	}
}

func (f *fakeQueryFeed) fetch(afterID int64, limit int) ([]store.Query, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Query
	for _, row := range f.rows {
		if row.ID > afterID && len(out) < limit {
			out = append(out, row)
		}
	}
	return out, nil
}

func (f *fakeQueryFeed) latestID() (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.rows) == 0 {
		return 0, nil
	}
	return f.rows[len(f.rows)-1].ID, nil
}

func subscribe(t *testing.T, hub *queryStreamHub) <-chan store.Query {
	t.Helper()
	stream, cancel, err := hub.Subscribe()
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	t.Cleanup(cancel)
	return stream
}

func receiveQueryIDs(t *testing.T, stream <-chan store.Query, n int) []int64 {
	t.Helper()
	var got []int64
	for len(got) < n {
		select {
		case query := <-stream:
			got = append(got, query.ID)
		case <-time.After(2 * time.Second):
			t.Fatalf("received IDs %v, want %d queries", got, n)
		}
	}
	return got
}

func TestQueryStreamHubBroadcastsToAllSubscribers(t *testing.T) {
	feed := &fakeQueryFeed{}
	hub := newQueryStreamHub(feed.fetch, feed.latestID, 100)
	defer hub.Close()
	first := subscribe(t, hub)
	second := subscribe(t, hub)

	feed.add(1)
	hub.NotifyNewQueries()

	for _, stream := range []<-chan store.Query{first, second} {
		if got := receiveQueryIDs(t, stream, 1); got[0] != 1 {
			t.Fatalf("query ID = %d, want 1", got[0])
		}
	}
}

func TestQueryStreamHubSkipsRowsWrittenBeforeFirstSubscriber(t *testing.T) {
	feed := &fakeQueryFeed{}
	feed.add(1, 2)
	hub := newQueryStreamHub(feed.fetch, feed.latestID, 100)
	defer hub.Close()
	stream := subscribe(t, hub)

	feed.add(3)
	hub.NotifyNewQueries()

	if got := receiveQueryIDs(t, stream, 1); got[0] != 3 {
		t.Fatalf("query ID = %d, want 3", got[0])
	}
}

func TestQueryStreamHubDrainsMultipleBatches(t *testing.T) {
	feed := &fakeQueryFeed{}
	hub := newQueryStreamHub(feed.fetch, feed.latestID, 2)
	defer hub.Close()
	stream := subscribe(t, hub)

	feed.add(1, 2, 3, 4, 5)
	hub.NotifyNewQueries()

	got := receiveQueryIDs(t, stream, 5)
	want := []int64{1, 2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got IDs %v, want %v", got, want)
		}
	}
}
