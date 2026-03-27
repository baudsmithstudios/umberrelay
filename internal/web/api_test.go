package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestUpdateSettingsAllowsValidKeys(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader("retention_days=7&list_refresh_hours=12")
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestUpdateSettingsRejectsUnknownKeys(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader("malicious_key=evil")
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
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
