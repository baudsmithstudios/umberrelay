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
	return testServerVersion(t, "dev")
}

func testServerVersion(t *testing.T, version string) *Server {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	s := NewServer(db, classify.NewManager(db), version)
	s.refreshListSources = func(context.Context, *classify.Manager, []classify.ListSource) error { return nil }
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

func TestMutationOriginGuard(t *testing.T) {
	cases := []struct {
		name   string
		header string
		value  string
		want   int
	}{
		{"cross-site origin", "Origin", "http://evil.local", http.StatusForbidden},
		{"same origin", "Origin", "http://umberrelay.local", http.StatusNoContent},
		{"cross-site fetch", "Sec-Fetch-Site", "cross-site", http.StatusForbidden},
		{"cross-site referer", "Referer", "http://evil.local/page", http.StatusForbidden},
		{"no browser signals", "", "", http.StatusNoContent},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := testServer(t)
			req := httptest.NewRequest("PUT", "http://umberrelay.local/api/settings", bytes.NewBufferString(`{"retention_days":30}`))
			req.Header.Set("Content-Type", "application/json")
			if c.header != "" {
				req.Header.Set(c.header, c.value)
			}
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)
			if w.Code != c.want {
				t.Fatalf("status = %d, want %d", w.Code, c.want)
			}
		})
	}
}

func TestHTTPServerDisablesWriteTimeoutForStreaming(t *testing.T) {
	s := testServer(t)
	httpServer := s.httpServer(":8080")
	if httpServer.WriteTimeout != 0 {
		t.Fatalf("WriteTimeout = %s, want 0", httpServer.WriteTimeout)
	}
}

func TestListenAndServeWaitsForInFlightRequestOnCancel(t *testing.T) {
	s := testServer(t)
	entered := make(chan struct{}, 1)
	handled := make(chan struct{})
	s.mux.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
		defer close(handled)
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
	case err := <-done:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Fatalf("ListenAndServe() error = %v, want nil or http.ErrServerClosed", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("ListenAndServe did not return after cancellation")
	}
	select {
	case <-handled:
	default:
		t.Fatal("ListenAndServe returned before the in-flight request finished")
	}

	select {
	case err := <-reqDone:
		if err != nil {
			t.Fatalf("in-flight request failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for in-flight request")
	}
}

func TestListenAndServeEndsLiveStreamsOnCancel(t *testing.T) {
	s := testServer(t)
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

	deadline := time.Now().Add(2 * time.Second)
	for {
		resp, err := http.Get("http://" + addr + "/api/queries/stream")
		if err == nil {
			defer resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("server did not become ready: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("ListenAndServe waited on an open live stream")
	}
}

func TestRefreshClassificationAsyncCancelsOnServerClose(t *testing.T) {
	s := testServer(t)

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
	s := testServer(t)

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
