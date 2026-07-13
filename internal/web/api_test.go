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

func TestAPIUpdateSettingsReturnsInternalErrorWhenPersistenceFails(t *testing.T) {
	s := testServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	body := bytes.NewBufferString(`{"retention_days":7,"list_refresh_hours":12}`)
	req := httptest.NewRequest("PUT", "/api/settings", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestAPIMutationRejectsOversizedBody(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	hugeLabel := strings.Repeat("a", int(maxMutationBodyBytes)+1)
	body := bytes.NewBufferString(`{"label":"` + hugeLabel + `"}`)
	req := httptest.NewRequest("PUT", "/api/devices/aa:bb:cc:dd:ee:ff", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
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

func TestAPIListRefreshStatusReturnsPersistedValues(t *testing.T) {
	s := testServer(t)
	attemptAt := time.Date(2026, 4, 1, 14, 0, 0, 0, time.UTC)
	successAt := attemptAt.Add(-time.Hour)
	if err := s.db.SetConfig("list_refresh_last_attempt_at", strconv.FormatInt(attemptAt.UnixNano(), 10)); err != nil {
		t.Fatalf("SetConfig(last_attempt): %v", err)
	}
	if err := s.db.SetConfig("list_refresh_last_success_at", strconv.FormatInt(successAt.UnixNano(), 10)); err != nil {
		t.Fatalf("SetConfig(last_success): %v", err)
	}
	if err := s.db.SetConfig("list_refresh_last_error", "refresh failed"); err != nil {
		t.Fatalf("SetConfig(last_error): %v", err)
	}

	req := httptest.NewRequest("GET", "/api/lists/status", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response struct {
		LastAttemptAt int64  `json:"last_attempt_at"`
		LastSuccessAt int64  `json:"last_success_at"`
		LastError     string `json:"last_error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if response.LastAttemptAt != attemptAt.Unix() {
		t.Fatalf("last_attempt_at = %d, want %d", response.LastAttemptAt, attemptAt.Unix())
	}
	if response.LastSuccessAt != successAt.Unix() {
		t.Fatalf("last_success_at = %d, want %d", response.LastSuccessAt, successAt.Unix())
	}
	if response.LastError != "refresh failed" {
		t.Fatalf("last_error = %q, want %q", response.LastError, "refresh failed")
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
		Key      string `json:"key"`
		Type     string `json:"type"`
		SourceIP string `json:"source_ip"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}

	foundDevice := false
	foundSource := false
	for _, actor := range response {
		if actor.Key == "device:aa:bb:cc:dd:ee:ff" && actor.Type == "device" {
			foundDevice = true
		}
		if actor.Key == "source:10.55.0.17" && actor.Type == "source" && actor.SourceIP == "10.55.0.17" {
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
}

func TestAPIAddListRejectsLocalURL(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"url":"http://localhost/list.txt","name":"Local","category":"tracking"}`)
	req := httptest.NewRequest("POST", "/api/lists", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIAddListReturnsInternalErrorWhenPersistenceFails(t *testing.T) {
	s := testServer(t)
	if err := s.db.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	body := bytes.NewBufferString(`{"url":"https://93.184.216.34/list.txt","name":"Example","category":"tracking"}`)
	req := httptest.NewRequest("POST", "/api/lists", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestAPIUpdateListEnabled(t *testing.T) {
	s := testServer(t)
	id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatal(err)
	}

	body := bytes.NewBufferString(`{"enabled":false}`)
	req := httptest.NewRequest("PUT", "/api/lists/"+strconv.FormatInt(id, 10), body)
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

func TestAPIListListsReturnsJSON(t *testing.T) {
	s := testServer(t)
	id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/lists", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []struct {
		ID      int64  `json:"id"`
		URL     string `json:"url"`
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("len(response) = %d, want 1", len(response))
	}
	if response[0].ID != id {
		t.Fatalf("id = %d, want %d", response[0].ID, id)
	}
	if response[0].Name != "Example" || response[0].URL != "https://example.com/list.txt" {
		t.Fatalf("response[0] = %#v, want Example list fields", response[0])
	}
}

func TestAPISetOverrideAcceptsJSON(t *testing.T) {
	s := testServer(t)
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

func TestAPISetOverrideRejectsInvalidCategory(t *testing.T) {
	s := testServer(t)
	body := bytes.NewBufferString(`{"category":"not-a-real-category"}`)
	req := httptest.NewRequest("PUT", "/api/overrides/example.com", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestAPIDeleteOverrideRemovesOverride(t *testing.T) {
	s := testServer(t)
	if err := s.classify.SetOverride("example.com", "tracking"); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	req := httptest.NewRequest("DELETE", "/api/overrides/example.com", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
	if got := s.classify.Classify("example.com."); got != "" {
		t.Fatalf("override category = %q, want empty", got)
	}
}

func TestAPIQueriesUsesSourceActorFilter(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "", SourceIP: "10.55.0.17", Domain: "source-only.example.com", QueryType: "A", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "10.55.0.17", Domain: "device-attributed.example.com", QueryType: "A", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "", SourceIP: "10.55.0.18", Domain: "other-source.example.com", QueryType: "A", Timestamp: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/queries?actor=source:10.55.0.17", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var response []store.Query
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("len(response) = %d, want 1", len(response))
	}
	if response[0].Domain != "source-only.example.com" {
		t.Fatalf("domain = %q, want %q", response[0].Domain, "source-only.example.com")
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
		{DeviceMAC: "11:22:33:44:55:66", SourceIP: "192.168.1.11", Domain: "ignore.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-1 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 250*time.Millisecond)
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
	if err := s.db.WriteListDomains(listID, map[string]string{"ads.example.com": "advertising"}); err != nil {
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
			SourceList  string `json:"source_list"`
			DeviceCount int    `json:"device_count"`
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
	if response.Domains[0].SourceList != "Tracking List" || response.Domains[0].DeviceCount != 2 {
		t.Fatalf("top domain = %#v, want Tracking List with 2 devices", response.Domains[0])
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
	if err := s.db.WriteListDomains(listID, map[string]string{"spike.example.com": "tracking"}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	var queries []store.Query
	for day := 2; day <= 8; day++ {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		queries = append(queries,
			store.Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "baseline.example.com", QueryType: "A", Category: "", Timestamp: base},
			store.Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "baseline.example.com", QueryType: "AAAA", Category: "", Timestamp: base.Add(time.Minute)},
			store.Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "baseline.example.com", QueryType: "TXT", Category: "", Timestamp: base.Add(2 * time.Minute)},
		)
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
		DeviceMAC string `json:"device_mac"`
		Type      string `json:"type"`
		TopDomain string `json:"top_domain"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 1 {
		t.Fatalf("anomaly count = %d, want 1", len(response))
	}
	if response[0].DeviceMAC != "aa:bb:cc:dd:ee:ff" || response[0].Type != "tracker_spike" || response[0].TopDomain != "spike.example.com" {
		t.Fatalf("anomaly row = %#v, want expected tracker spike", response[0])
	}
}

func TestAPIBypassSignals(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()

	if err := s.db.UpsertDevice(store.Device{MAC: "aa:bb:cc:dd:ee:01", Hostname: "living-room-tv", FirstSeen: now.Add(-3 * 24 * time.Hour), LastSeen: now.Add(-5 * time.Minute)}); err != nil {
		t.Fatalf("UpsertDevice(likely): %v", err)
	}
	if err := s.db.UpsertDevice(store.Device{MAC: "aa:bb:cc:dd:ee:02", Hostname: "kitchen-speaker", FirstSeen: now.Add(-3 * 24 * time.Hour), LastSeen: now.Add(-6 * time.Minute)}); err != nil {
		t.Fatalf("UpsertDevice(suspected): %v", err)
	}
	if err := s.db.UpsertDevice(store.Device{MAC: "aa:bb:cc:dd:ee:03", Hostname: "office-laptop", FirstSeen: now.Add(-3 * 24 * time.Hour), LastSeen: now.Add(-4 * time.Minute)}); err != nil {
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
		DeviceMAC  string `json:"device_mac"`
		Confidence string `json:"confidence"`
		HintDomain string `json:"hint_domain"`
	}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if len(response) != 2 {
		t.Fatalf("bypass signal count = %d, want 2", len(response))
	}
	if response[0].DeviceMAC != "aa:bb:cc:dd:ee:01" || response[0].Confidence != "likely" || response[0].HintDomain != "dns.google" {
		t.Fatalf("first signal = %#v, want likely dns.google row", response[0])
	}
	if response[1].DeviceMAC != "aa:bb:cc:dd:ee:02" || response[1].Confidence != "suspected" {
		t.Fatalf("second signal = %#v, want suspected row", response[1])
	}
}

func TestQueryMatchesFeedFilter(t *testing.T) {
	tests := []struct {
		name   string
		query  store.Query
		filter store.QueryFeedFilter
		want   bool
	}{
		{
			name:   "source filter matches unattributed source",
			query:  store.Query{DeviceMAC: "", SourceIP: "10.0.0.7", Domain: "a.example.com"},
			filter: store.QueryFeedFilter{SourceIP: "10.0.0.7"},
			want:   true,
		},
		{
			name:   "source filter rejects attributed device",
			query:  store.Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "10.0.0.7", Domain: "a.example.com"},
			filter: store.QueryFeedFilter{SourceIP: "10.0.0.7"},
			want:   false,
		},
		{
			name:   "uncategorized filter matches empty category",
			query:  store.Query{Domain: "a.example.com", Category: ""},
			filter: store.QueryFeedFilter{Category: "uncategorized"},
			want:   true,
		},
		{
			name:   "category filter rejects mismatch",
			query:  store.Query{Domain: "a.example.com", Category: "tracking"},
			filter: store.QueryFeedFilter{Category: "analytics"},
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryMatchesFeedFilter(tt.query, tt.filter)
			if got != tt.want {
				t.Fatalf("queryMatchesFeedFilter() = %t, want %t", got, tt.want)
			}
		})
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
