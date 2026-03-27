package web

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"scrye/internal/store"
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

func TestUpdateSettingsRejectsInvalidValues(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader("retention_days=0&list_refresh_hours=999")
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

func TestAPIUpdateDeviceAcceptsForm(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader("label=Living+Room+TV")
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	dev, err := s.db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Label != "Living Room TV" {
		t.Fatalf("label = %q, want %q", dev.Label, "Living Room TV")
	}
}

func TestAPIAddListRejectsLocalURL(t *testing.T) {
	s := testServerWithClassify(t)
	body := strings.NewReader("url=http://localhost/list.txt&name=Local&category=tracking")
	req := httptest.NewRequest("POST", "/api/lists", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIUpdateListEnabled(t *testing.T) {
	s := testServer(t)
	id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader("enabled=false")
	req := httptest.NewRequest("PUT", "/api/lists/"+itoa(id), body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}

	lists, err := s.db.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Enabled {
		t.Fatalf("enabled = %v, want false", lists[0].Enabled)
	}
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

func itoa(v int64) string {
	return strconv.FormatInt(v, 10)
}
