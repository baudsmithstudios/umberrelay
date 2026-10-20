package web

import (
	"fmt"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func seedPrivacyPageData(t *testing.T, s *Server, now time.Time) {
	t.Helper()

	devices := []store.Device{
		{
			MAC:       "aa:bb:cc:dd:ee:ff",
			IP:        "192.168.1.10",
			Hostname:  "roku-tv",
			Vendor:    "Roku",
			Label:     "Living Room TV",
			FirstSeen: now.Add(-14 * 24 * time.Hour),
			LastSeen:  now,
		},
		{
			MAC:       "11:22:33:44:55:66",
			IP:        "192.168.1.20",
			Hostname:  "ipad",
			Vendor:    "Apple",
			FirstSeen: now.Add(-10 * 24 * time.Hour),
			LastSeen:  now,
		},
	}
	for _, device := range devices {
		if err := s.db.UpsertDevice(device); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", device.MAC, err)
		}
		if device.Label != "" {
			if err := s.db.UpdateDeviceLabel(device.MAC, device.Label); err != nil {
				t.Fatalf("UpdateDeviceLabel(%s): %v", device.MAC, err)
			}
		}
	}

	listID, err := s.db.AddList("https://example.com/tracking.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := s.db.WriteListDomains(listID, map[string]string{
		"ads.example.com":     "tracking",
		"metrics.example.com": "analytics",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}
	if err := s.db.SetDomainOverride("manual.example.com", "telemetry"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	queries := []store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(-90 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "manual.example.com", QueryType: "A", Category: "telemetry", Timestamp: now.Add(-70 * time.Minute)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-55 * time.Minute)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "metrics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-50 * time.Minute)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-40 * time.Minute)},
	}
	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
}

func TestDeviceDetailPageSourceRoute(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	if err := s.db.WriteQueries([]store.Query{{
		DeviceMAC: "",
		SourceIP:  "10.44.0.7",
		Domain:    "unknown.example.com",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-5 * time.Minute),
	}}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
	if err := s.db.SetSourceLabel("10.44.0.7", "Kitchen Display"); err != nil {
		t.Fatalf("SetSourceLabel: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices/source:10.44.0.7", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{"<h1>Kitchen Display · 10.44.0.7</h1>", "/ui/sources/10.44.0.7/label"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestDeviceDetailPageDomainPagination(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	mac := "aa:bb:cc:dd:ee:ff"

	if err := s.db.UpsertDevice(store.Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		Hostname:  "tracker-box",
		FirstSeen: now.Add(-24 * time.Hour),
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	queries := make([]store.Query, 0, 25)
	for i := 1; i <= 25; i++ {
		queries = append(queries, store.Query{
			DeviceMAC: mac,
			Domain:    fmt.Sprintf("domain-%02d.example.com", i),
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-time.Duration(i) * time.Minute),
		})
	}
	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices/"+mac+"?tab=domains&page=2", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"showing 21-25",
		"Page 2 of 2",
		`href="?tab=domains&page=1"`,
		`id="tab-domains" role="tab" aria-selected="true"`,
		"domain-21.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
	if strings.Contains(body, "domain-01.example.com") {
		t.Fatalf("response unexpectedly contains page-1 domain")
	}
}

func TestDeviceDetailUnknownDeviceRedirectsToActivity(t *testing.T) {
	s := testServer(t)

	req := httptest.NewRequest("GET", "/devices/ff:ee:dd:cc:bb:aa", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/" {
		t.Fatalf("Location = %q, want %q", location, "/")
	}
}

func TestSettingsPageIncludesListRefreshStatus(t *testing.T) {
	s := testServer(t)
	attemptAt := time.Date(2026, 4, 2, 12, 34, 0, 0, time.UTC)
	if err := s.db.SetConfig("list_refresh_last_attempt_at", strconv.FormatInt(attemptAt.UnixNano(), 10)); err != nil {
		t.Fatalf("SetConfig(last_attempt): %v", err)
	}
	if err := s.db.SetConfig("list_refresh_last_error", "network timeout"); err != nil {
		t.Fatalf("SetConfig(last_error): %v", err)
	}

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "network timeout") {
		t.Fatalf("settings page missing refresh error status: %q", body)
	}
}

func TestUIUpdateSettingsRedirectsBackToSettings(t *testing.T) {
	s := testServer(t)
	form := url.Values{
		"retention_days":     {"7"},
		"list_refresh_hours": {"12"},
	}
	req := httptest.NewRequest("POST", "/ui/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/settings" {
		t.Fatalf("Location = %q, want %q", location, "/settings")
	}

	retention, err := s.db.GetConfig("retention_days")
	if err != nil {
		t.Fatalf("GetConfig(retention_days): %v", err)
	}
	if retention != "7" {
		t.Fatalf("retention_days = %q, want %q", retention, "7")
	}

	refreshHours, err := s.db.GetConfig("list_refresh_hours")
	if err != nil {
		t.Fatalf("GetConfig(list_refresh_hours): %v", err)
	}
	if refreshHours != "12" {
		t.Fatalf("list_refresh_hours = %q, want %q", refreshHours, "12")
	}
}

func TestUIMutationRejectsOversizedBody(t *testing.T) {
	s := testServer(t)
	huge := strings.Repeat("a", int(maxMutationBodyBytes)+1)
	form := url.Values{
		"retention_days":     {"7"},
		"list_refresh_hours": {"12"},
		"padding":            {huge},
	}
	req := httptest.NewRequest("POST", "/ui/settings", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusRequestEntityTooLarge)
	}
}

func TestUIListActionsRedirectBackToSettings(t *testing.T) {
	tests := []struct {
		name     string
		makeReq  func(t *testing.T, s *Server) *http.Request
		assertDB func(t *testing.T, s *Server)
	}{
		{
			name: "toggle enabled",
			makeReq: func(t *testing.T, s *Server) *http.Request {
				t.Helper()
				id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
				if err != nil {
					t.Fatalf("AddList: %v", err)
				}
				form := url.Values{"enabled": {"false"}}
				req := httptest.NewRequest("POST", "/ui/lists/"+strconv.FormatInt(id, 10)+"/enabled", strings.NewReader(form.Encode()))
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
				return req
			},
			assertDB: func(t *testing.T, s *Server) {
				t.Helper()
				lists, err := s.db.ListLists()
				if err != nil {
					t.Fatalf("ListLists: %v", err)
				}
				if len(lists) != 1 || lists[0].Enabled {
					t.Fatalf("enabled = %v, want false", lists[0].Enabled)
				}
			},
		},
		{
			name: "delete list",
			makeReq: func(t *testing.T, s *Server) *http.Request {
				t.Helper()
				id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
				if err != nil {
					t.Fatalf("AddList: %v", err)
				}
				return httptest.NewRequest("POST", "/ui/lists/"+strconv.FormatInt(id, 10)+"/delete", nil)
			},
			assertDB: func(t *testing.T, s *Server) {
				t.Helper()
				lists, err := s.db.ListLists()
				if err != nil {
					t.Fatalf("ListLists: %v", err)
				}
				if len(lists) != 0 {
					t.Fatalf("lists = %#v, want empty", lists)
				}
			},
		},
		{
			name: "refresh lists",
			makeReq: func(_ *testing.T, _ *Server) *http.Request {
				return httptest.NewRequest("POST", "/ui/lists/refresh", nil)
			},
			assertDB: func(_ *testing.T, _ *Server) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testServer(t)
			req := tt.makeReq(t, s)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusSeeOther {
				t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
			}
			if location := w.Header().Get("Location"); location != "/settings" {
				t.Fatalf("Location = %q, want %q", location, "/settings")
			}
			tt.assertDB(t, s)
		})
	}
}

func TestUIUpdateDeviceLabelRedirectsBackToPrivacyPage(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(store.Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		FirstSeen: time.Now(),
		LastSeen:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"label": {"Living Room TV"}}
	req := httptest.NewRequest("POST", "/ui/devices/aa:bb:cc:dd:ee:ff/label", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/devices/aa:bb:cc:dd:ee:ff" {
		t.Fatalf("Location = %q, want %q", location, "/devices/aa:bb:cc:dd:ee:ff")
	}
}

func TestDevicesListPageShowsUpdatedSourceLabel(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	if err := s.db.WriteQueries([]store.Query{{
		DeviceMAC: "",
		SourceIP:  "10.44.0.7",
		Domain:    "unknown.example.com",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-5 * time.Minute),
	}}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	form := url.Values{"label": {"Kitchen Display"}}
	updateReq := httptest.NewRequest("POST", "/ui/sources/10.44.0.7/label", strings.NewReader(form.Encode()))
	updateReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	updateRes := httptest.NewRecorder()
	s.Handler().ServeHTTP(updateRes, updateReq)
	if updateRes.Code != http.StatusSeeOther {
		t.Fatalf("update status = %d, want %d", updateRes.Code, http.StatusSeeOther)
	}
	if location := updateRes.Header().Get("Location"); location != "/devices/source:10.44.0.7" {
		t.Fatalf("Location = %q, want %q", location, "/devices/source:10.44.0.7")
	}

	listReq := httptest.NewRequest("GET", "/", nil)
	listRes := httptest.NewRecorder()
	s.Handler().ServeHTTP(listRes, listReq)
	if listRes.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", listRes.Code, http.StatusOK)
	}

	body := html.UnescapeString(listRes.Body.String())
	if !strings.Contains(body, "Kitchen Display · 10.44.0.7") {
		t.Fatalf("response missing updated source label: %q", body)
	}
}

func TestUIOverrideReturnsUpdatedDomainRow(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	form := url.Values{
		"category": {"tracking"},
		"scope":    {"network"},
	}
	req := httptest.NewRequest("POST", "/ui/overrides/manual.example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "manual.example.com") {
		t.Fatalf("response missing updated domain")
	}
	if !strings.Contains(body, "tracking · manual") {
		t.Fatalf("response missing updated override category/source: %s", body)
	}
	if !strings.Contains(body, `<span class="elum-status-label" data-tone="error">tracking · manual</span>`) {
		t.Fatalf("override row class is not a toned status label: %s", body)
	}
}

func TestUIOverrideRejectsInvalidCategory(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	form := url.Values{
		"category": {"not-a-real-category"},
		"scope":    {"network"},
	}
	req := httptest.NewRequest("POST", "/ui/overrides/manual.example.com", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStaticServesElumKit(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/static/elumkit/index.css", "/static/elumkit/tabs.js"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d, want 200", path, w.Code)
		}
		if path == "/static/elumkit/index.css" && !strings.Contains(w.Body.String(), "@import") {
			t.Fatalf("index.css missing @import: %q", w.Body.String())
		}
	}
}

func TestSettingsPageUsesElumKitLayout(t *testing.T) {
	s := testServer(t)
	if _, err := s.db.AddList("https://example.com/tracking.txt", "Tracking List", "tracking"); err != nil {
		t.Fatalf("AddList: %v", err)
	}
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/settings", nil))
	body := w.Body.String()
	for _, want := range []string{
		"/static/elumkit/index.css",
		"/static/css/app.css",
		`class="elum-navbar"`,
		`href="/settings" aria-current="page"`,
		`class="elum-switch"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("settings page missing %q", want)
		}
	}
	if strings.Contains(body, "pico.min.css") {
		t.Errorf("settings page still loads pico.min.css")
	}
}

func getPage(t *testing.T, s *Server, path string) string {
	t.Helper()
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s status = %d, want 200", path, w.Code)
	}
	return html.UnescapeString(w.Body.String())
}

// rowFor returns the roster row link whose href is exactly href.
func rowFor(t *testing.T, body, href string) string {
	t.Helper()
	start := strings.Index(body, `<a class="elum-row" href="`+href+`"`)
	if start < 0 {
		t.Fatalf("no roster row with href %q", href)
	}
	end := strings.Index(body[start:], "</a>")
	return body[start : start+end]
}

func TestActivityNetworkSelected(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/")
	if row := rowFor(t, body, "/"); !strings.Contains(row, `data-selected="true"`) || !strings.Contains(row, "Whole network") {
		t.Fatalf("network row not selected: %q", row)
	}
	for _, want := range []string{"Living Room TV", ">Devices<", "of DNS queries hit known trackers"} {
		if !strings.Contains(body, want) {
			t.Errorf("activity page missing %q", want)
		}
	}
	if strings.Contains(body, "Unattributed sources") {
		t.Errorf("activity page shows a sources group with no sources")
	}
}

func TestActivityDeviceSelected(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/devices/device:aa:bb:cc:dd:ee:ff")
	row := rowFor(t, body, "/devices/device:aa:bb:cc:dd:ee:ff")
	if !strings.Contains(row, `data-selected="true"`) {
		t.Fatalf("device row not selected: %q", row)
	}
	for _, want := range []string{`data-actor-key="device:aa:bb:cc:dd:ee:ff"`, `data-tracker-percent="66.67"`, `data-query-count="3"`, `data-name="Living Room TV"`} {
		if !strings.Contains(row, want) {
			t.Errorf("device row missing %s", want)
		}
	}
	if row := rowFor(t, body, "/"); strings.Contains(row, `data-selected="true"`) {
		t.Fatalf("network row selected on a device page: %q", row)
	}
	for _, want := range []string{"<h1>Living Room TV</h1>", "/ui/devices/aa:bb:cc:dd:ee:ff/label", "roku-tv"} {
		if !strings.Contains(body, want) {
			t.Errorf("device pane missing %q", want)
		}
	}
}

func TestActivitySourceIPv6Selected(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	if err := s.db.WriteQueries([]store.Query{{
		SourceIP:  "fe80::1",
		Domain:    "v6.example.com",
		QueryType: "AAAA",
		Timestamp: now.Add(-5 * time.Minute),
	}}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	body := getPage(t, s, "/devices/source:fe80::1")
	if !strings.Contains(body, "Unattributed sources") {
		t.Fatalf("sources group missing")
	}
	if row := rowFor(t, body, "/devices/source:fe80::1"); !strings.Contains(row, `data-selected="true"`) {
		t.Fatalf("source row not selected: %q", row)
	}
}

func TestDevicesListRouteIs404(t *testing.T) {
	s := testServer(t)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/devices", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", w.Code)
	}
}

func TestRosterMiniMeterWidthRenders(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/")
	if strings.Contains(body, "ZgotmplZ") {
		t.Fatalf("template sanitized a value: %q", body)
	}
	if !strings.Contains(rowFor(t, body, "/devices/device:aa:bb:cc:dd:ee:ff"), `<i style="width: 66.7%">`) {
		t.Fatalf("mini-meter width missing")
	}
}

func TestOverviewShowsTrackerMeterAndMatches(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/")
	if !strings.Contains(body, `role="meter"`) {
		t.Fatalf("tracker meter missing")
	}
	start := strings.Index(body, "Matched a list")
	end := strings.Index(body, "queries matched no list")
	if start < 0 || end < start {
		t.Fatalf("matches table missing")
	}
	matches := body[start:end]
	for _, want := range []string{">tracking<", ">analytics<", ">telemetry<"} {
		if !strings.Contains(matches, want) {
			t.Errorf("matches table missing %q", want)
		}
	}
	if strings.Contains(matches, "unclassified") || strings.Contains(matches, "uncategorized") {
		t.Errorf("matches table lists unmatched queries as a category")
	}
}

func TestOverviewEmptyDatabase(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/")
	for _, want := range []string{"0.0%", "No queries matched a list in the last 24 hours"} {
		if !strings.Contains(body, want) {
			t.Errorf("empty overview missing %q", want)
		}
	}
	for _, bad := range []string{"NaN", "Inf"} {
		if strings.Contains(body, bad) {
			t.Errorf("empty overview contains %q", bad)
		}
	}
}

func TestDeviceOverviewReferenceIsNetwork(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/devices/device:aa:bb:cc:dd:ee:ff")
	if !strings.Contains(body, "network 50.0%") {
		t.Fatalf("device meter missing network reference")
	}
}

func TestParseTab(t *testing.T) {
	for raw, want := range map[string]string{"domains": "domains", "live": "live", "bogus": "overview", "": "overview"} {
		if got := parseTab(raw); got != want {
			t.Errorf("parseTab(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestTabQuerySelectsDomains(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/?tab=domains")
	if !strings.Contains(body, `id="tab-domains" role="tab" aria-selected="true"`) {
		t.Fatalf("domains tab not selected")
	}
	if !strings.Contains(body, `id="tab-overview" role="tab" aria-selected="false"`) {
		t.Fatalf("overview tab still selected")
	}
}

func TestNetworkOverviewNeedsAttentionEmpty(t *testing.T) {
	s := testServer(t)
	if body := getPage(t, s, "/"); !strings.Contains(body, "No unusual activity") {
		t.Fatalf("needs attention empty state missing")
	}
}

func TestNetworkOverviewNeedsAttentionShowsFlags(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/")
	if !strings.Contains(body, `<a href="/devices/device:aa:bb:cc:dd:ee:ff">Living Room TV</a> · tracker spike.`) {
		t.Fatalf("needs attention missing the tracker spike flag")
	}
}

func TestDomainsTabPageBeyondRangeClamps(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	mac := "aa:bb:cc:dd:ee:ff"
	if err := s.db.UpsertDevice(store.Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	queries := make([]store.Query, 0, 25)
	for i := 1; i <= 25; i++ {
		queries = append(queries, store.Query{DeviceMAC: mac, Domain: fmt.Sprintf("domain-%02d.example.com", i), QueryType: "A", Timestamp: now.Add(-time.Duration(i) * time.Minute)})
	}
	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	body := getPage(t, s, "/devices/"+mac+"?tab=domains&page=999")
	for _, want := range []string{"Page 2 of 2", `id="tab-domains" role="tab" aria-selected="true"`} {
		if !strings.Contains(body, want) {
			t.Errorf("clamped page missing %q", want)
		}
	}
}

func TestNetworkDomainsTabHasOverrides(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/?tab=domains")
	if !strings.Contains(body, `hx-post="/ui/overrides/ads.example.com"`) {
		t.Fatalf("network domains tab missing override form")
	}
}

func TestIdentificationNote(t *testing.T) {
	full := identificationNote(store.Device{MAC: "aa", Hostname: "roku-tv", Vendor: "Roku"})
	if full != "MAC from the ARP table. Hostname roku-tv from DHCP. Vendor Roku from the MAC prefix." {
		t.Errorf("full note = %q", full)
	}
	if bare := identificationNote(store.Device{MAC: "aa"}); bare != "MAC from the ARP table." {
		t.Errorf("bare note = %q", bare)
	}
}

func TestLiveTabMarkup(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/?tab=live")
	if !strings.Contains(body, `<ul class="elum-feed" id="live-query-feed"`) {
		t.Fatalf("live feed list missing")
	}
	if !strings.Contains(body, `id="live-query-domain-filter"`) {
		t.Fatalf("live feed filter missing")
	}
}

func TestUnclassifiedDomainRowPreselectsUncategorized(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/?tab=domains")
	start := strings.Index(body, `id="domain-row-clean.example.com"`)
	if start < 0 {
		t.Fatalf("clean.example.com row missing")
	}
	row := body[start : start+strings.Index(body[start:], "</tr>")]
	if !strings.Contains(row, `<option value="uncategorized" selected>`) {
		t.Fatalf("unclassified row does not preselect uncategorized: %q", row)
	}
}

func TestNetworkRowDeviceCountMatchesHeadline(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)
	if err := s.db.UpsertDevice(store.Device{MAC: "99:99:99:99:99:99", IP: "192.168.1.99", FirstSeen: now.Add(-30 * 24 * time.Hour), LastSeen: now.Add(-30 * 24 * time.Hour)}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	body := getPage(t, s, "/")
	if !strings.Contains(body, "across 2 devices") {
		t.Fatalf("headline device count changed")
	}
	if row := rowFor(t, body, "/"); !strings.Contains(row, "2 devices ·") {
		t.Fatalf("network row count differs from headline: %q", row)
	}
}

func TestNoPicoAssetsServed(t *testing.T) {
	s := testServer(t)
	for _, path := range []string{"/static/css/pico.min.css", "/static/js/uplot.min.js"} {
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("GET %s status = %d, want 404", path, w.Code)
		}
	}
}

func TestNetworkMeterOmitsReferenceWithoutPriorData(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	body := getPage(t, s, "/")
	if strings.Contains(body, "prior 7d") && !strings.Contains(body, "vs prior 7d") {
		t.Fatalf("meter claims a prior-7d reference with no prior data")
	}
	if strings.Contains(body, `class="tick"`) {
		t.Fatalf("meter draws a reference tick with no prior data")
	}
}

func TestLiveStreamCarriesCategoryTones(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/?tab=live")
	if !strings.Contains(body, `data-category-tones="{`) || !strings.Contains(body, `"tracking":"error"`) {
		t.Fatalf("live stream missing category tone map")
	}
}

func TestRosterFilterIsABareField(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/")
	if !strings.Contains(body, `<form class="elum-field" role="search" aria-label="Filter devices">`) {
		t.Fatalf("roster filter should be a bare label and input, not a bordered query card")
	}
}

func TestMainIsNotALiveRegion(t *testing.T) {
	s := testServer(t)
	body := getPage(t, s, "/")
	if strings.Contains(body, `<main id="main" aria-live`) {
		t.Fatalf("main is a live region, so every live feed row is announced")
	}
}
