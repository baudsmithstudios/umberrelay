package web

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"umberrelay/internal/store"
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

func TestUpdateSettingsRejectsMultipleJSONValues(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"retention_days":7}{"list_refresh_hours":12}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestUpdateSettingsRejectsTrailingJSONValue(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"retention_days":7} true`)
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

func TestAPIGetSettingsReturnsNumericValues(t *testing.T) {
	s := testServer(t)
	if err := s.db.SetConfig("retention_days", "7"); err != nil {
		t.Fatalf("SetConfig(retention_days): %v", err)
	}
	if err := s.db.SetConfig("list_refresh_hours", "12"); err != nil {
		t.Fatalf("SetConfig(list_refresh_hours): %v", err)
	}

	req := httptest.NewRequest("GET", "/api/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response struct {
		RetentionDays    int `json:"retention_days"`
		ListRefreshHours int `json:"list_refresh_hours"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response.RetentionDays != 7 {
		t.Fatalf("retention_days = %d, want %d", response.RetentionDays, 7)
	}
	if response.ListRefreshHours != 12 {
		t.Fatalf("list_refresh_hours = %d, want %d", response.ListRefreshHours, 12)
	}
}

func TestAPIGetSettingsReturnsDefaultsForOutOfRangeValues(t *testing.T) {
	s := testServer(t)
	if err := s.db.SetConfig("retention_days", "-1"); err != nil {
		t.Fatalf("SetConfig(retention_days): %v", err)
	}
	if err := s.db.SetConfig("list_refresh_hours", "0"); err != nil {
		t.Fatalf("SetConfig(list_refresh_hours): %v", err)
	}

	req := httptest.NewRequest("GET", "/api/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response struct {
		RetentionDays    int `json:"retention_days"`
		ListRefreshHours int `json:"list_refresh_hours"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response.RetentionDays != 30 {
		t.Fatalf("retention_days = %d, want %d", response.RetentionDays, 30)
	}
	if response.ListRefreshHours != 24 {
		t.Fatalf("list_refresh_hours = %d, want %d", response.ListRefreshHours, 24)
	}
}

func TestAPIGetSettingsReturnsInternalErrorForStoreFailures(t *testing.T) {
	s := testServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
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

func TestAPIActorsIncludesUnattributedSources(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "known-device",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "known.example.com", QueryType: "A", Timestamp: now.Add(-5 * time.Minute)},
		{DeviceMAC: "", SourceIP: "10.55.0.17", Domain: "unknown.example.com", QueryType: "A", Timestamp: now.Add(-4 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/actors", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		Key       string `json:"key"`
		Type      string `json:"type"`
		Name      string `json:"name"`
		DeviceMAC string `json:"device_mac"`
		SourceIP  string `json:"source_ip"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	foundDevice := false
	foundSource := false
	for _, actor := range response {
		if actor.Key == "device:aa:bb:cc:dd:ee:ff" && actor.Type == "device" && actor.DeviceMAC == "aa:bb:cc:dd:ee:ff" {
			foundDevice = true
		}
		if actor.Key == "source:10.55.0.17" && actor.Type == "source" && actor.SourceIP == "10.55.0.17" {
			if actor.Name == "" {
				t.Fatalf("source actor name is empty")
			}
			foundSource = true
		}
	}

	if !foundDevice {
		t.Fatalf("response missing known device actor: %#v", response)
	}
	if !foundSource {
		t.Fatalf("response missing source fallback actor: %#v", response)
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

func TestAPIDeviceReturnsInternalErrorForStoreFailures(t *testing.T) {
	s := testServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "internal error" {
		t.Fatalf("error = %q, want %q", response["error"], "internal error")
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

func TestAPIUpdateDeviceRejectsMultipleJSONObjects(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"label":"Living Room TV"}{"label":"Kitchen TV"}`)
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	dev, err := s.db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Label != "" {
		t.Fatalf("label = %q, want empty string", dev.Label)
	}
}

func TestAPIUpdateDeviceReturnsNotFoundForMissingDevice(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"label":"Living Room TV"}`)
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "device not found" {
		t.Fatalf("error = %q, want %q", response["error"], "device not found")
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

func TestAPIUpdateListReturnsNotFoundForMissingList(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest("PUT", "/api/lists/999", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "list not found" {
		t.Fatalf("error = %q, want %q", response["error"], "list not found")
	}
}

func TestAPIDeleteListReturnsNotFoundForMissingList(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("DELETE", "/api/lists/999", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "list not found" {
		t.Fatalf("error = %q, want %q", response["error"], "list not found")
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

func TestAPIQueriesRejectInvalidOffsetAsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/queries?offset=-1", nil)
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
	if response["error"] != "offset must be a non-negative integer" {
		t.Fatalf("error = %q, want %q", response["error"], "offset must be a non-negative integer")
	}
}

func TestAPIQueryStreamRejectsInvalidActorAsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/queries/stream?actor=bad-actor", nil)
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
	if response["error"] != "actor must be device:{mac} or source:{ip}" {
		t.Fatalf("error = %q, want %q", response["error"], "actor must be device:{mac} or source:{ip}")
	}
}

func TestAPIQueryStreamRejectsInvalidCategoryAsJSONError(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/queries/stream?category=not-a-real-category", nil)
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
	if response["error"] != "invalid category filter" {
		t.Fatalf("error = %q, want %q", response["error"], "invalid category filter")
	}
}

func TestAPIQueryStreamEmitsFilteredSSEEvent(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "living-room-tv",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if err := s.db.UpsertDevice(store.Device{
		MAC:       "11:22:33:44:55:66",
		IP:        "192.168.1.11",
		Hostname:  "laptop",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := s.db.WriteQueries([]store.Query{
		{
			DeviceMAC: "11:22:33:44:55:66",
			SourceIP:  "192.168.1.11",
			Domain:    "ignore.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(-2 * time.Second),
		},
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			SourceIP:  "192.168.1.10",
			Domain:    "ads.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(-1 * time.Second),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/queries/stream?actor=device:aa:bb:cc:dd:ee:ff&category=tracking", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("Content-Type = %q, want text/event-stream", ct)
	}

	body := w.Body.String()
	for _, want := range []string{
		"event: query",
		`"device_mac":"aa:bb:cc:dd:ee:ff"`,
		`"domain":"ads.example.com"`,
		`"category":"tracking"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q in body %q", want, body)
		}
	}
	if strings.Contains(body, "ignore.example.com") {
		t.Fatalf("stream response included non-matching query: %q", body)
	}
}

func TestAPIQueryStreamUncategorizedFilterIncludesEmptyCategory(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "living-room-tv",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := s.db.WriteQueries([]store.Query{
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			SourceIP:  "192.168.1.10",
			Domain:    "unknown.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-1 * time.Second),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()

	req := httptest.NewRequest("GET", "/api/queries/stream?category=uncategorized", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"domain":"unknown.example.com"`) {
		t.Fatalf("uncategorized stream did not include empty-category query: %q", body)
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

func TestAPIActivitySupportsRanges(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	for _, mac := range []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"} {
		if err := s.db.UpsertDevice(store.Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}
	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "analytics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(-48*time.Hour + time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/activity?range=7d", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		Timestamp int64 `json:"timestamp"`
		Total     int   `json:"total"`
		Tracker   int   `json:"tracker"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 7 {
		t.Fatalf("bucket count = %d, want 7", len(response))
	}
	total := 0
	tracker := 0
	for _, bucket := range response {
		total += bucket.Total
		tracker += bucket.Tracker
	}
	if total != 3 || tracker != 2 {
		t.Fatalf("totals = %d/%d, want 3/2", total, tracker)
	}
}

func TestAPIActivityDefaultsTo24Hours(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "recent.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "older.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-48 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/activity", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		Timestamp int64 `json:"timestamp"`
		Total     int   `json:"total"`
		Tracker   int   `json:"tracker"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 24 {
		t.Fatalf("bucket count = %d, want 24", len(response))
	}

	total := 0
	tracker := 0
	for _, bucket := range response {
		total += bucket.Total
		tracker += bucket.Tracker
	}
	if total != 1 || tracker != 1 {
		t.Fatalf("totals = %d/%d, want 1/1", total, tracker)
	}
}

func TestAPIActivitySupportsSourceActorFilter(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "", SourceIP: "10.55.0.17", Domain: "source-only.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "", SourceIP: "10.55.0.17", Domain: "source-only.example.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(-90 * time.Minute)},
		{DeviceMAC: "", SourceIP: "10.55.0.18", Domain: "other-source.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-80 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/activity?actor=source:10.55.0.17", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		Timestamp int64 `json:"timestamp"`
		Total     int   `json:"total"`
		Tracker   int   `json:"tracker"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 24 {
		t.Fatalf("bucket count = %d, want 24", len(response))
	}

	total := 0
	tracker := 0
	for _, bucket := range response {
		total += bucket.Total
		tracker += bucket.Tracker
	}
	if total != 2 || tracker != 1 {
		t.Fatalf("totals = %d/%d, want 2/1", total, tracker)
	}
}

func TestAPIActivityRejectsInvalidRange(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/api/activity?range=bogus", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response["error"] != "range must be one of 24h, 7d, 30d" {
		t.Fatalf("error = %q, want %q", response["error"], "range must be one of 24h, 7d, 30d")
	}
}

func TestAPIDomainsIncludesSourceListAndTotalDevices(t *testing.T) {
	s := testServer(t)
	now := time.Now()

	for _, mac := range []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66"} {
		if err := s.db.UpsertDevice(store.Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}
	listID, err := s.db.AddList("https://example.com/trackers.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := s.db.WriteListDomains(listID, map[string]string{
		"ads.example.com": "advertising",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}
	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "manual.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
	if err := s.db.SetDomainOverride("manual.example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/domains?limit=10", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response struct {
		TotalDevices int `json:"total_devices"`
		Domains      []struct {
			Domain      string `json:"domain"`
			Category    string `json:"category"`
			QueryCount  int    `json:"query_count"`
			DeviceCount int    `json:"device_count"`
			SourceList  string `json:"source_list"`
		} `json:"domains"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response.TotalDevices != 2 {
		t.Fatalf("total_devices = %d, want 2", response.TotalDevices)
	}
	if len(response.Domains) != 2 {
		t.Fatalf("domain count = %d, want 2", len(response.Domains))
	}
	if response.Domains[0].SourceList != "Tracking List" {
		t.Fatalf("top source_list = %q, want Tracking List", response.Domains[0].SourceList)
	}
	if response.Domains[0].DeviceCount != 2 {
		t.Fatalf("top device_count = %d, want 2", response.Domains[0].DeviceCount)
	}
	if response.Domains[1].SourceList != "manual" {
		t.Fatalf("manual source_list = %q, want manual", response.Domains[1].SourceList)
	}
}

func TestAPIDomainsCountSourceFallbackActors(t *testing.T) {
	s := testServer(t)
	now := time.Now()

	if err := s.db.UpsertDevice(store.Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "shared.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "", SourceIP: "10.0.0.7", Domain: "shared.example.com", QueryType: "A", Category: "", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "", SourceIP: "10.0.0.8", Domain: "shared.example.com", QueryType: "A", Category: "", Timestamp: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/domains?limit=10", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response struct {
		TotalDevices int `json:"total_devices"`
		Domains      []struct {
			Domain      string `json:"domain"`
			DeviceCount int    `json:"device_count"`
		} `json:"domains"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if response.TotalDevices != 3 {
		t.Fatalf("total_devices = %d, want 3", response.TotalDevices)
	}
	if len(response.Domains) == 0 {
		t.Fatalf("domain count = 0, want 1")
	}
	if response.Domains[0].Domain != "shared.example.com" {
		t.Fatalf("top domain = %q, want shared.example.com", response.Domains[0].Domain)
	}
	if response.Domains[0].DeviceCount != 3 {
		t.Fatalf("device_count = %d, want 3", response.Domains[0].DeviceCount)
	}
}

func TestAPIAnomalies(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "tracker-box",
		FirstSeen: now.Add(-8 * 24 * time.Hour),
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	listID, err := s.db.AddList("https://example.com/trackers.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := s.db.WriteListDomains(listID, map[string]string{
		"spike.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	var queries []store.Query
	for day := 2; day <= 8; day++ {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		queries = append(queries, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "baseline.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: base,
		})
		queries = append(queries, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "baseline.example.com",
			QueryType: "AAAA",
			Category:  "",
			Timestamp: base.Add(time.Minute),
		})
		queries = append(queries, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "baseline.example.com",
			QueryType: "TXT",
			Category:  "",
			Timestamp: base.Add(2 * time.Minute),
		})
	}
	for i := 0; i < 6; i++ {
		queries = append(queries, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "spike.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(-2*time.Hour + time.Duration(i)*time.Minute),
		})
	}
	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/anomalies", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		DeviceMAC           string  `json:"device_mac"`
		Type                string  `json:"type"`
		TopDomain           string  `json:"top_domain"`
		TopDomainSourceList string  `json:"top_domain_source_list"`
		Delta               float64 `json:"delta"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("anomaly count = %d, want 1", len(response))
	}
	if response[0].DeviceMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("device_mac = %q, want aa:bb:cc:dd:ee:ff", response[0].DeviceMAC)
	}
	if response[0].Type != "tracker_spike" {
		t.Fatalf("type = %q, want tracker_spike", response[0].Type)
	}
	if response[0].TopDomain != "spike.example.com" {
		t.Fatalf("top_domain = %q, want spike.example.com", response[0].TopDomain)
	}
	if response[0].TopDomainSourceList != "Tracking List" {
		t.Fatalf("top_domain_source_list = %q, want Tracking List", response[0].TopDomainSourceList)
	}
	if response[0].Delta <= 5 {
		t.Fatalf("delta = %v, want > 5", response[0].Delta)
	}
}

func TestAPIBypassSignals(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:01",
		Hostname:  "living-room-tv",
		FirstSeen: now.Add(-3 * 24 * time.Hour),
		LastSeen:  now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertDevice(likely): %v", err)
	}
	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:02",
		Hostname:  "kitchen-speaker",
		FirstSeen: now.Add(-3 * 24 * time.Hour),
		LastSeen:  now.Add(-6 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertDevice(suspected): %v", err)
	}
	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:03",
		Hostname:  "office-laptop",
		FirstSeen: now.Add(-3 * 24 * time.Hour),
		LastSeen:  now.Add(-4 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertDevice(active): %v", err)
	}

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "dns.google.", QueryType: "A", Timestamp: now.Add(-2 * 24 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "example.com.", QueryType: "A", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "example.org.", QueryType: "A", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "active.example.net.", QueryType: "A", Timestamp: now.Add(-15 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/bypass", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		DeviceMAC       string `json:"device_mac"`
		DeviceName      string `json:"device_name"`
		Confidence      string `json:"confidence"`
		HintDomain      string `json:"hint_domain"`
		SilentMinutes   int    `json:"silent_minutes"`
		PriorQueryCount int    `json:"prior_query_count"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	if len(response) != 2 {
		t.Fatalf("bypass signal count = %d, want 2", len(response))
	}
	if response[0].DeviceMAC != "aa:bb:cc:dd:ee:01" {
		t.Fatalf("first device_mac = %q, want aa:bb:cc:dd:ee:01", response[0].DeviceMAC)
	}
	if response[0].Confidence != "likely" {
		t.Fatalf("first confidence = %q, want likely", response[0].Confidence)
	}
	if response[0].HintDomain != "dns.google" {
		t.Fatalf("first hint_domain = %q, want dns.google", response[0].HintDomain)
	}
	if response[0].SilentMinutes <= 0 {
		t.Fatalf("first silent_minutes = %d, want > 0", response[0].SilentMinutes)
	}
	if response[1].DeviceMAC != "aa:bb:cc:dd:ee:02" {
		t.Fatalf("second device_mac = %q, want aa:bb:cc:dd:ee:02", response[1].DeviceMAC)
	}
	if response[1].Confidence != "suspected" {
		t.Fatalf("second confidence = %q, want suspected", response[1].Confidence)
	}
	if response[1].HintDomain != "" {
		t.Fatalf("second hint_domain = %q, want empty", response[1].HintDomain)
	}
	if response[1].PriorQueryCount == 0 {
		t.Fatalf("second prior_query_count = %d, want > 0", response[1].PriorQueryCount)
	}
}

func TestAPIBypassReturnsInternalErrorForStoreFailures(t *testing.T) {
	s := testServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/bypass", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
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
