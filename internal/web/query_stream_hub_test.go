package web

import (
	"path/filepath"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func hubDB(t *testing.T) *store.DB {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func newDBHub(db *store.DB, batch int) *queryStreamHub {
	return newQueryStreamHub(func(afterID int64, limit int) ([]store.Query, error) {
		return db.QueryFeed(afterID, store.QueryFeedFilter{}, limit)
	}, db.LatestQueryID, batch)
}

// writeQueries stores n queries; IDs count up from 1 in a fresh database.
func writeQueries(t *testing.T, db *store.DB, n int) {
	t.Helper()
	queries := make([]store.Query, n)
	for i := range queries {
		queries[i] = store.Query{SourceIP: "10.0.0.7", Domain: "example.com", QueryType: "A", Timestamp: time.Now()}
	}
	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
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
	db := hubDB(t)
	hub := newDBHub(db, 100)
	defer hub.Close()
	first := subscribe(t, hub)
	second := subscribe(t, hub)

	writeQueries(t, db, 1)
	hub.NotifyNewQueries()

	for _, stream := range []<-chan store.Query{first, second} {
		if got := receiveQueryIDs(t, stream, 1); got[0] != 1 {
			t.Fatalf("query ID = %d, want 1", got[0])
		}
	}
}

func TestQueryStreamHubSkipsRowsWrittenBeforeFirstSubscriber(t *testing.T) {
	db := hubDB(t)
	writeQueries(t, db, 2)
	hub := newDBHub(db, 100)
	defer hub.Close()
	stream := subscribe(t, hub)

	writeQueries(t, db, 1)
	hub.NotifyNewQueries()

	if got := receiveQueryIDs(t, stream, 1); got[0] != 3 {
		t.Fatalf("query ID = %d, want 3", got[0])
	}
}

func TestQueryStreamHubDrainsMultipleBatches(t *testing.T) {
	db := hubDB(t)
	hub := newDBHub(db, 2)
	defer hub.Close()
	stream := subscribe(t, hub)

	writeQueries(t, db, 5)
	hub.NotifyNewQueries()

	got := receiveQueryIDs(t, stream, 5)
	want := []int64{1, 2, 3, 4, 5}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got IDs %v, want %v", got, want)
		}
	}
}

func TestQueryStreamHubCloseWaitsForInFlightPoll(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	hub := newQueryStreamHub(func(int64, int) ([]store.Query, error) {
		close(entered)
		<-release
		return nil, nil
	}, func() (int64, error) { return 0, nil }, 100)
	subscribe(t, hub)
	<-entered

	closed := make(chan struct{})
	go func() {
		hub.Close()
		close(closed)
	}()
	select {
	case <-closed:
		t.Fatal("Close returned while a poll was in flight")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return after the poll finished")
	}
}
