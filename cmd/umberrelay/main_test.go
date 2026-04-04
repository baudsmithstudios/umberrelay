package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func TestRuntimeDefaults(t *testing.T) {
	if defaultConfigPath != "/etc/umberrelay/config.toml" {
		t.Fatalf("defaultConfigPath = %q, want %q", defaultConfigPath, "/etc/umberrelay/config.toml")
	}
	if defaultDBName != "umberrelay.db" {
		t.Fatalf("defaultDBName = %q, want %q", defaultDBName, "umberrelay.db")
	}
	if runtimeName != "umberrelay" {
		t.Fatalf("runtimeName = %q, want %q", runtimeName, "umberrelay")
	}
}

func TestPurgeRemovesRowsOlderThanConfiguredRetention(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	if err := db.SetConfig("retention_days", "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := db.WriteQueries([]store.Query{
		{SourceIP: "10.0.0.1", Domain: "stale.example.com", QueryType: "A", Timestamp: now.Add(-48 * time.Hour)},
		{SourceIP: "10.0.0.1", Domain: "recent.example.com", QueryType: "A", Timestamp: now.Add(-2 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	purge(db)

	queries, err := db.QueryLog("", "", time.Time{}, time.Now().Add(time.Hour), 100, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(queries) != 1 {
		t.Fatalf("len(queries) = %d, want 1", len(queries))
	}
	if queries[0].Domain != "recent.example.com" {
		t.Fatalf("domain = %q, want recent.example.com", queries[0].Domain)
	}
}

func TestRunPurgeRunsImmediateCycleBeforeWaiting(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	if err := db.SetConfig("retention_days", "1"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := db.WriteQueries([]store.Query{
		{SourceIP: "10.0.0.1", Domain: "old.example.com", QueryType: "A", Timestamp: now.Add(-72 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	runPurge(ctx, db)

	queries, err := db.QueryLog("", "", time.Time{}, time.Now().Add(time.Hour), 100, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(queries) != 0 {
		t.Fatalf("len(queries) = %d, want 0", len(queries))
	}
}

func TestDefaultListSourcesSeedsDefaultsWhenStoreIsEmpty(t *testing.T) {
	db := testDB(t)

	sources := defaultListSources(db)
	if len(sources) != 3 {
		t.Fatalf("len(sources) = %d, want 3", len(sources))
	}

	lists, err := db.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) != 3 {
		t.Fatalf("len(lists) = %d, want 3", len(lists))
	}
}

func TestDefaultListSourcesReturnsOnlyEnabledStoredLists(t *testing.T) {
	db := testDB(t)
	firstID, err := db.AddList("https://example.com/one.txt", "One", "tracking")
	if err != nil {
		t.Fatalf("AddList(one): %v", err)
	}
	secondID, err := db.AddList("https://example.com/two.txt", "Two", "analytics")
	if err != nil {
		t.Fatalf("AddList(two): %v", err)
	}
	if err := db.UpdateListEnabled(secondID, false); err != nil {
		t.Fatalf("UpdateListEnabled: %v", err)
	}

	sources := defaultListSources(db)
	if len(sources) != 1 {
		t.Fatalf("len(sources) = %d, want 1", len(sources))
	}
	if sources[0].ID != firstID {
		t.Fatalf("source ID = %d, want %d", sources[0].ID, firstID)
	}
}

func testDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
