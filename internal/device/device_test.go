package device

import (
	"path/filepath"
	"testing"

	"umberrelay/internal/store"
)

func testDB(t *testing.T) *store.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	return db
}
