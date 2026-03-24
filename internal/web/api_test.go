package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAPISummary(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/summary", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestAPIDevices(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
