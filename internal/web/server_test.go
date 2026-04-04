package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"umberrelay/internal/classify"
	"umberrelay/internal/store"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, nil)
	t.Cleanup(func() {
		s.Close()
		db.Close()
	})
	return s
}

func testServerWithClassify(t *testing.T) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, classify.NewManager(db))
	t.Cleanup(func() {
		s.Close()
		db.Close()
	})
	return s
}

func TestHealthEndpoint(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHTTPServerDisablesWriteTimeoutForStreaming(t *testing.T) {
	s := testServer(t)
	httpServer := s.httpServer(":8080")
	if httpServer.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0", httpServer.WriteTimeout)
	}
}

func TestParsePagesIncludesFragmentsTemplateSet(t *testing.T) {
	pages := parsePages()
	fragments, ok := pages["fragments"]
	if !ok {
		t.Fatalf("missing fragments template set")
	}

	var out bytes.Buffer
	err := fragments.ExecuteTemplate(&out, "label-edit", struct {
		Device     store.Device
		DeviceName string
	}{
		Device: store.Device{
			MAC:   "aa:bb:cc:dd:ee:ff",
			Label: "Living Room TV",
		},
		DeviceName: "Living Room TV",
	})
	if err != nil {
		t.Fatalf("ExecuteTemplate(label-edit): %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("label-edit fragment rendered empty output")
	}
}
