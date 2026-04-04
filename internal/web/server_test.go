package web

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

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

func TestNotifyNewQueriesWakesHub(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	if err := s.db.WriteQueries([]store.Query{
		{SourceIP: "10.0.0.7", Domain: "wake.example.com", QueryType: "A", Timestamp: now},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	stream, cancel := s.queryHub.Subscribe()
	defer cancel()
	s.NotifyNewQueries()

	select {
	case query := <-stream:
		if query.Domain != "wake.example.com" {
			t.Fatalf("domain = %q, want %q", query.Domain, "wake.example.com")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for query hub notification")
	}
}

func TestListenAndServeReturnsWhenContextCancelled(t *testing.T) {
	s := testServer(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.ListenAndServe(ctx, "127.0.0.1:0")
	}()

	cancel()

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ListenAndServe() error = %v, want nil or http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe did not return after context cancellation")
	}
}
