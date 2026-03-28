package demo

import (
	"path/filepath"
	"testing"
	"time"

	"scrye/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "demo.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSeedPopulatesEmptyDatabase(t *testing.T) {
	db := testDB(t)

	if err := Seed(db, time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)); err != nil {
		t.Fatalf("Seed: %v", err)
	}

	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) < 3 {
		t.Fatalf("got %d devices, want at least 3", len(devices))
	}

	queries, err := db.QueryLog("", "", time.Time{}, time.Now().Add(24*time.Hour), 1000, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(queries) < 10 {
		t.Fatalf("got %d queries, want at least 10", len(queries))
	}

	lists, err := db.ListLists()
	if err != nil {
		t.Fatalf("ListLists: %v", err)
	}
	if len(lists) == 0 {
		t.Fatal("expected seeded classification lists")
	}

	cached, err := db.LoadCachedDomains()
	if err != nil {
		t.Fatalf("LoadCachedDomains: %v", err)
	}
	if len(cached) == 0 {
		t.Fatal("expected cached classification domains")
	}

	retention, err := db.GetConfig("retention_days")
	if err != nil {
		t.Fatalf("GetConfig retention_days: %v", err)
	}
	if retention == "" {
		t.Fatal("expected retention_days config to be set")
	}
}

func TestSeedSkipsWhenDatabaseAlreadyHasQueries(t *testing.T) {
	db := testDB(t)
	now := time.Date(2026, 3, 28, 12, 0, 0, 0, time.UTC)

	if err := Seed(db, now); err != nil {
		t.Fatalf("first Seed: %v", err)
	}

	before, err := db.QueryLog("", "", time.Time{}, time.Now().Add(24*time.Hour), 1000, 0)
	if err != nil {
		t.Fatalf("QueryLog before: %v", err)
	}

	if err := Seed(db, now.Add(time.Hour)); err != nil {
		t.Fatalf("second Seed: %v", err)
	}

	after, err := db.QueryLog("", "", time.Time{}, time.Now().Add(24*time.Hour), 1000, 0)
	if err != nil {
		t.Fatalf("QueryLog after: %v", err)
	}

	if len(after) != len(before) {
		t.Fatalf("query count changed from %d to %d on reseed", len(before), len(after))
	}
}
