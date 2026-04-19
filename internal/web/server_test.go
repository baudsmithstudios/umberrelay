package web

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
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

func TestHandlerSetsSecurityHeaders(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/health", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)

	if got := w.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("X-Content-Type-Options = %q, want nosniff", got)
	}
	if got := w.Header().Get("X-Frame-Options"); got != "DENY" {
		t.Fatalf("X-Frame-Options = %q, want DENY", got)
	}
	if got := w.Header().Get("Referrer-Policy"); got != "no-referrer" {
		t.Fatalf("Referrer-Policy = %q, want no-referrer", got)
	}
}

func TestMutationRejectsCrossSiteOrigin(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("PUT", "http://umberrelay.local/api/settings", bytes.NewBufferString(`{"retention_days":30}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "http://evil.local")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestMutationRejectsCrossSiteFetchSite(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("POST", "/ui/settings", bytes.NewBufferString("retention_days=30&list_refresh_hours=24"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestMutationAllowsRequestWithoutBrowserOriginSignals(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("PUT", "/api/settings", bytes.NewBufferString(`{"retention_days":30}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code == http.StatusForbidden {
		t.Fatalf("status = %d, expected non-forbidden response", w.Code)
	}
}

func TestHTTPServerDisablesWriteTimeoutForStreaming(t *testing.T) {
	s := testServer(t)
	httpServer := s.httpServer(":8080")
	if httpServer.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0", httpServer.WriteTimeout)
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

func TestListenAndServeAllowsInFlightRequestOnCancel(t *testing.T) {
	s := testServer(t)
	entered := make(chan struct{}, 1)
	s.mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		entered <- struct{}{}
		time.Sleep(150 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.ListenAndServe(ctx, addr)
	}()

	client := &http.Client{Timeout: 2 * time.Second}
	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := client.Get("http://" + addr + "/api/health")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	reqDone := make(chan error, 1)
	go func() {
		resp, err := client.Get("http://" + addr + "/slow")
		if err != nil {
			reqDone <- err
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			reqDone <- errors.New("slow request did not return 200")
			return
		}
		reqDone <- nil
	}()

	select {
	case <-entered:
	case <-time.After(time.Second):
		cancel()
		t.Fatal("slow handler was not entered")
	}
	cancel()

	select {
	case err := <-reqDone:
		if err != nil {
			t.Fatalf("in-flight request failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight request")
	}

	select {
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ListenAndServe() error = %v, want nil or http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe did not return after cancellation")
	}
}

func TestRefreshClassificationAsyncCancelsOnServerClose(t *testing.T) {
	s := testServerWithClassify(t)

	started := make(chan struct{}, 1)
	finished := make(chan struct{}, 1)
	s.loadEnabledListSources = func(db *store.DB) ([]classify.ListSource, error) {
		return []classify.ListSource{}, nil
	}
	s.refreshListSources = func(ctx context.Context, mgr *classify.Manager, sources []classify.ListSource) error {
		started <- struct{}{}
		<-ctx.Done()
		finished <- struct{}{}
		return ctx.Err()
	}

	s.refreshClassificationAsync()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("refresh job did not start")
	}

	s.Close()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("refresh job did not stop after server close")
	}
}

func TestRefreshClassificationAsyncSkipsWhenRefreshRunning(t *testing.T) {
	s := testServerWithClassify(t)

	release := make(chan struct{})
	started := make(chan struct{}, 2)
	var calls int32
	s.loadEnabledListSources = func(db *store.DB) ([]classify.ListSource, error) {
		return []classify.ListSource{}, nil
	}
	s.refreshListSources = func(ctx context.Context, mgr *classify.Manager, sources []classify.ListSource) error {
		atomic.AddInt32(&calls, 1)
		started <- struct{}{}
		<-release
		return nil
	}

	s.refreshClassificationAsync()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("first refresh job did not start")
	}

	s.refreshClassificationAsync()
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("refresh call count = %d, want 1 while first job is running", got)
	}

	close(release)
}
