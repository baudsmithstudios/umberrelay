package app

import (
	"path/filepath"
	"testing"
	"time"

	"scrye/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func deviceFixture() store.Device {
	now := time.Now()
	return store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		FirstSeen: now,
		LastSeen:  now,
	}
}
