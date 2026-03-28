package web

import (
	"bytes"
	"encoding/json"
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
	body := bytes.NewBufferString(`{"retention_days":7,"list_refresh_hours":12}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestUpdateSettingsRejectsUnknownKeys(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"malicious_key":"evil"}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettingsRejectsInvalidValues(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"retention_days":0,"list_refresh_hours":999}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettingsRejectsNonJSONRequests(t *testing.T) {
	s := testServer(t)
	body := strings.NewReader("retention_days=7&list_refresh_hours=12")
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestUpdateSettingsReturnsJSONErrors(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"retention_days":0}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] == "" {
		t.Fatalf("error response missing message: %#v", response)
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

func TestAPIDeviceNotFoundReturnsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "device not found" {
		t.Fatalf("error = %q, want %q", response["error"], "device not found")
	}
}

func TestAPIUpdateDeviceAcceptsJSON(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"label":"Living Room TV"}`)
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/json")
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

func TestAPIUpdateDeviceRejectsNonJSONRequests(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	body := strings.NewReader("label=Living+Room+TV")
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAPIAddListRejectsLocalURL(t *testing.T) {
	s := testServerWithClassify(t)
	body := bytes.NewBufferString(`{"url":"http://localhost/list.txt","name":"Local","category":"tracking"}`)
	req := httptest.NewRequest("POST", "/api/lists", body)
	req.Header.Set("Content-Type", "application/json")
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

	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest("PUT", "/api/lists/"+itoa(id), body)
	req.Header.Set("Content-Type", "application/json")
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

func TestAPIUpdateListRejectsNonJSONRequests(t *testing.T) {
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
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAPISetOverrideAcceptsJSON(t *testing.T) {
	s := testServerWithClassify(t)
	body := bytes.NewBufferString(`{"category":"tracking"}`)
	req := httptest.NewRequest("PUT", "/api/overrides/example.com", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := s.classify.Classify("example.com."); got != "tracking" {
		t.Fatalf("override category = %q, want %q", got, "tracking")
	}
}

func TestAPISetOverrideRejectsNonJSONRequests(t *testing.T) {
	s := testServerWithClassify(t)
	body := strings.NewReader("category=tracking")
	req := httptest.NewRequest("PUT", "/api/overrides/example.com", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnsupportedMediaType)
	}
}

func TestAPIQueriesRejectInvalidFromAsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/queries?from=not-a-time", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "from must be RFC3339" {
		t.Fatalf("error = %q, want %q", response["error"], "from must be RFC3339")
	}
}

func TestAPIRefreshListsWithoutManagerReturnsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("POST", "/api/lists/refresh", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", ct)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "classify manager not available" {
		t.Fatalf("error = %q, want %q", response["error"], "classify manager not available")
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
