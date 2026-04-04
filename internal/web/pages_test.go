package web

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"umberrelay/internal/store"
)

func seedTrendPageDevice(t *testing.T, s *Server, mac, hostname string, now time.Time) {
	t.Helper()

	err := s.db.UpsertDevice(store.Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		Hostname:  hostname,
		Vendor:    "Vendor",
		FirstSeen: now.Add(-8 * 24 * time.Hour),
		LastSeen:  now,
	})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
}

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

func TestFormatTrend(t *testing.T) {
	tests := []struct {
		name         string
		trend        store.Trend
		isTrackerPct bool
		want         TrendDisplay
	}{
		{
			name:  "unavailable",
			trend: store.Trend{},
			want:  TrendDisplay{},
		},
		{
			name:  "query up",
			trend: store.Trend{Change: 15.4, HasPrior: true},
			want:  TrendDisplay{Text: "+15%", Class: "trend-up"},
		},
		{
			name:  "query down",
			trend: store.Trend{Change: -3.2, HasPrior: true},
			want:  TrendDisplay{Text: "-3%", Class: "trend-down"},
		},
		{
			name:  "query flat",
			trend: store.Trend{Change: 0.49, HasPrior: true},
			want:  TrendDisplay{Text: "0%", Class: "trend-flat"},
		},
		{
			name:         "tracker up",
			trend:        store.Trend{Change: 3.2, HasPrior: true},
			isTrackerPct: true,
			want:         TrendDisplay{Text: "+3pp", Class: "trend-up"},
		},
		{
			name:         "tracker down",
			trend:        store.Trend{Change: -2.6, HasPrior: true},
			isTrackerPct: true,
			want:         TrendDisplay{Text: "-3pp", Class: "trend-down"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatTrend(tt.trend, tt.isTrackerPct)
			if got != tt.want {
				t.Fatalf("formatTrend() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHomePage(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"<title>Umberrelay",
		`class="umberrelay-brand" href="/">`,
		`class="umberrelay-brand-umber">Umber</span><span class="umberrelay-brand-relay">relay</span>`,
		"Home",
		"Devices",
		"Settings",
		"/static/css/home.css",
		"/static/js/charts.js",
		"Network Privacy",
		"Tracker Rate Over Time",
		"Needs Attention",
		"Top Domains",
		"ads.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestHomePageTopDomainsShowReach(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		`data-label="Domain"`,
		`data-label="Classification"`,
		`data-label="Queries"`,
		`data-label="Reach"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing mobile cell label %q", want)
		}
	}
}

func TestHomePageAnomalyLinksToDeviceDetail(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `href="/devices/`) {
		t.Fatalf("anomaly items should link to device detail pages")
	}
	if !strings.Contains(body, "Investigate") {
		t.Fatalf("response missing investigate link text")
	}
}

func TestDevicesListPage(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"<title>Umberrelay",
		"Devices",
		"/static/css/devices.css",
		"/static/js/devices.js",
		`id="device-search"`,
		`id="device-sort"`,
		`id="device-list"`,
		"Living Room TV",
		`data-tracker-percent=`,
		`data-query-count=`,
		`href="/devices/`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestDevicesListPageSortsDevicesByTrackerRate(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	// Both devices should appear with links to their detail pages
	if !strings.Contains(body, `href="/devices/device:aa:bb:cc:dd:ee:ff"`) {
		t.Fatalf("response missing link to first device")
	}
	if !strings.Contains(body, `href="/devices/device:11:22:33:44:55:66"`) {
		t.Fatalf("response missing link to second device")
	}
}

func TestPrivacyPage(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/domains", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"<title>Umberrelay",
		"/static/css/privacy.css",
		"All Devices",
		"Network Domains",
		"ads.example.com",
		"tracking · Tracking List",
		"manual.example.com",
		"telemetry · manual",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestPrivacyPageRoutesWithoutSelectionRenderNetworkView(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	for _, path := range []string{"/domains"} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest("GET", path, nil)
			w := httptest.NewRecorder()
			s.Handler().ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200", w.Code)
			}

			body := html.UnescapeString(w.Body.String())
			for _, want := range []string{"Overview", "Investigation", "Network Domains"} {
				if !strings.Contains(body, want) {
					t.Fatalf("response missing %q", want)
				}
			}
			if strings.Contains(body, "Device Detail") {
				t.Fatalf("response should render the network view for %s", path)
			}
		})
	}
}

func TestPrivacyPageDataSortsDevicesByTrackerPercentDescending(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	devices := []store.Device{
		{
			MAC:       "aa:aa:aa:aa:aa:aa",
			IP:        "192.168.1.10",
			Hostname:  "low-rate",
			FirstSeen: now.Add(-14 * 24 * time.Hour),
			LastSeen:  now,
		},
		{
			MAC:       "bb:bb:bb:bb:bb:bb",
			IP:        "192.168.1.11",
			Hostname:  "high-rate",
			FirstSeen: now.Add(-14 * 24 * time.Hour),
			LastSeen:  now,
		},
	}
	for _, device := range devices {
		if err := s.db.UpsertDevice(device); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", device.MAC, err)
		}
	}

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: "aa:aa:aa:aa:aa:aa", Domain: "tracker-1.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:aa:aa:aa:aa:aa", Domain: "clean-1.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-90 * time.Minute)},
		{DeviceMAC: "aa:aa:aa:aa:aa:aa", Domain: "clean-2.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-80 * time.Minute)},
		{DeviceMAC: "aa:aa:aa:aa:aa:aa", Domain: "clean-3.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-70 * time.Minute)},
		{DeviceMAC: "aa:aa:aa:aa:aa:aa", Domain: "clean-4.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-60 * time.Minute)},
		{DeviceMAC: "bb:bb:bb:bb:bb:bb", Domain: "tracker-2.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "bb:bb:bb:bb:bb:bb", Domain: "tracker-3.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-90 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	view, err := s.privacyPageData(now, "")
	if err != nil {
		t.Fatalf("privacyPageData: %v", err)
	}
	if len(view.Devices) != 2 {
		t.Fatalf("len(view.Devices) = %d, want 2", len(view.Devices))
	}
	if got := view.Devices[0].MAC; got != "bb:bb:bb:bb:bb:bb" {
		t.Fatalf("first device MAC = %q, want %q", got, "bb:bb:bb:bb:bb:bb")
	}
}

func TestPrivacyPageDeviceDeepLinkSelectsDevice(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		"Device Detail",
		"Living Room TV",
		"192.168.1.10",
		"Edit Label",
		"tracking · Tracking List",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestPrivacyPageDeviceDetailIncludesLiveQueryStreamUI(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		"Live Query Stream",
		`id="live-query-domain-filter"`,
		`id="live-query-category-filter"`,
		`id="live-query-feed"`,
		`/static/js/privacy.js`,
		`data-actor-key="device:aa:bb:cc:dd:ee:ff"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestPrivacyPageDataIncludesSourceFallbackActors(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	if err := s.db.WriteQueries([]store.Query{
		{
			DeviceMAC: "",
			SourceIP:  "10.44.0.7",
			Domain:    "unknown.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-5 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	view, err := s.privacyPageData(now, "")
	if err != nil {
		t.Fatalf("privacyPageData: %v", err)
	}

	found := false
	for _, actor := range view.Devices {
		if actor.ActorType == "source" && actor.SourceIP == "10.44.0.7" {
			found = true
			if actor.MAC != "" {
				t.Fatalf("source actor MAC = %q, want empty", actor.MAC)
			}
			break
		}
	}
	if !found {
		t.Fatalf("expected source fallback actor in privacy view: %#v", view.Devices)
	}
}

func TestPrivacyPageLoadsExternalPrivacyScript(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/domains", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `/static/js/privacy.js`) {
		t.Fatalf("response missing external privacy script reference")
	}
	if strings.Contains(body, "function chartColor(") {
		t.Fatalf("privacy page should not inline large script logic")
	}
}

func TestPrivacyPageDataIncludesBypassSignals(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	mac := "aa:bb:cc:dd:ee:ff"
	if err := s.db.UpsertDevice(store.Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		Hostname:  "living-room-tv",
		FirstSeen: now.Add(-7 * 24 * time.Hour),
		LastSeen:  now.Add(-5 * time.Minute),
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := s.db.WriteQueries([]store.Query{
		{
			DeviceMAC: mac,
			Domain:    "dns.google.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-3 * 24 * time.Hour),
		},
		{
			DeviceMAC: mac,
			Domain:    "example.com.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-26 * time.Hour),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	view, err := s.privacyPageData(now, "")
	if err != nil {
		t.Fatalf("privacyPageData: %v", err)
	}

	foundAttention := false
	for _, anomaly := range view.Anomalies {
		if anomaly.DeviceMAC == mac {
			foundAttention = true
			if anomaly.Class != "anomaly-bypass" {
				t.Fatalf("anomaly class = %q, want %q", anomaly.Class, "anomaly-bypass")
			}
		}
	}
	if !foundAttention {
		t.Fatalf("expected bypass signal in attention feed: %#v", view.Anomalies)
	}

	foundRow := false
	for _, device := range view.Devices {
		if device.MAC == mac {
			foundRow = true
			if device.AnomalyClass != "anomaly-bypass" {
				t.Fatalf("device anomaly class = %q, want %q", device.AnomalyClass, "anomaly-bypass")
			}
		}
	}
	if !foundRow {
		t.Fatalf("expected device row for %s", mac)
	}
}

func TestPrivacyPageSourceDeepLinkSelectsSourceDetail(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }

	if err := s.db.WriteQueries([]store.Query{
		{
			DeviceMAC: "",
			SourceIP:  "10.44.0.7",
			Domain:    "unknown.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-5 * time.Minute),
		},
	}); err != nil {
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
	for _, want := range []string{
		"Source Detail",
		"10.44.0.7",
		"Kitchen Display · 10.44.0.7",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
	if strings.Contains(body, "Unattributed source · 10.44.0.7") {
		t.Fatalf("response should not include unattributed subtitle when label is set")
	}
}

func TestPrivacyPageUnknownDeviceFallsBackToNetworkView(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/ff:ee:dd:cc:bb:aa", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Network Domains") {
		t.Fatalf("response should fall back to network view: %s", body)
	}
	if strings.Contains(body, "Device Detail") {
		t.Fatalf("response should not render a missing device detail view")
	}
}

func TestSettingsPage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestSettingsPagePostsToUIAction(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `hx-post="/ui/settings"`) {
		t.Fatalf("settings page should post to UI action route")
	}
	if strings.Contains(w.Body.String(), `hx-put="/api/settings"`) {
		t.Fatalf("settings page should not post forms directly to /api/settings")
	}
}

func TestSettingsPagePostsListActionsToUIRoutes(t *testing.T) {
	s := testServer(t)
	_, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("GET", "/settings", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		`hx-post="/ui/lists/1/enabled"`,
		`hx-post="/ui/lists/1/delete"`,
		`hx-post="/ui/lists"`,
		`hx-post="/ui/lists/refresh"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("settings page missing %q", want)
		}
	}

	for _, avoid := range []string{
		`hx-put="/api/lists/1"`,
		`hx-delete="/api/lists/1"`,
		`hx-post="/api/lists"`,
		`hx-post="/api/lists/refresh"`,
	} {
		if strings.Contains(body, avoid) {
			t.Fatalf("settings page should not use %q", avoid)
		}
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

func TestUIAddListRedirectsBackToSettings(t *testing.T) {
	s := testServer(t)
	form := url.Values{
		"url":      {"https://93.184.216.34/list.txt"},
		"name":     {"Example"},
		"category": {"tracking"},
	}
	req := httptest.NewRequest("POST", "/ui/lists", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/settings" {
		t.Fatalf("Location = %q, want %q", location, "/settings")
	}

	lists, err := s.db.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Name != "Example" {
		t.Fatalf("lists = %#v, want added list", lists)
	}
}

func TestUIToggleListRedirectsBackToSettings(t *testing.T) {
	s := testServer(t)
	id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{"enabled": {"false"}}
	req := httptest.NewRequest("POST", "/ui/lists/"+itoa(id)+"/enabled", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/settings" {
		t.Fatalf("Location = %q, want %q", location, "/settings")
	}

	lists, err := s.db.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 1 || lists[0].Enabled {
		t.Fatalf("enabled = %v, want false", lists[0].Enabled)
	}
}

func TestUIDeleteListRedirectsBackToSettings(t *testing.T) {
	s := testServer(t)
	id, err := s.db.AddList("https://example.com/list.txt", "Example", "tracking")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", "/ui/lists/"+itoa(id)+"/delete", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/settings" {
		t.Fatalf("Location = %q, want %q", location, "/settings")
	}

	lists, err := s.db.ListLists()
	if err != nil {
		t.Fatal(err)
	}
	if len(lists) != 0 {
		t.Fatalf("lists = %#v, want empty", lists)
	}
}

func TestUIRefreshListsRedirectsBackToSettings(t *testing.T) {
	s := testServerWithClassify(t)
	req := httptest.NewRequest("POST", "/ui/lists/refresh", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/settings" {
		t.Fatalf("Location = %q, want %q", location, "/settings")
	}
}

func TestPrivacyPagePostsLabelToUIAction(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `hx-post="/ui/devices/aa:bb:cc:dd:ee:ff/label"`) {
		t.Fatalf("privacy page should post label form to UI action route")
	}
	if strings.Contains(w.Body.String(), `hx-put="/api/devices/aa:bb:cc:dd:ee:ff"`) {
		t.Fatalf("privacy page should not submit label form directly to the API")
	}
	if strings.Contains(w.Body.String(), `<option value="" selected>unclassified</option>`) {
		t.Fatalf("privacy page should not submit an empty override category for unclassified")
	}
	if !strings.Contains(w.Body.String(), `<option value="uncategorized">unclassified</option>`) {
		t.Fatalf("privacy page should submit uncategorized for the unclassified override option")
	}
	if strings.Contains(w.Body.String(), `<option value="uncategorized">uncategorized</option>`) {
		t.Fatalf("privacy page should not render a duplicate uncategorized override option")
	}
}

func TestUIUpdateDeviceLabelRedirectsBackToPrivacyPage(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
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

	dev, err := s.db.GetDevice("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatal(err)
	}
	if dev.Label != "Living Room TV" {
		t.Fatalf("label = %q, want %q", dev.Label, "Living Room TV")
	}
}

func TestUIUpdateDeviceLabelReturnsFragmentForHTMX(t *testing.T) {
	s := testServer(t)
	if err := s.db.UpsertDevice(deviceFixture()); err != nil {
		t.Fatal(err)
	}

	form := url.Values{"label": {"Living Room TV"}}
	req := httptest.NewRequest("POST", "/ui/devices/aa:bb:cc:dd:ee:ff/label", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	if !strings.Contains(w.Body.String(), "Living Room TV") {
		t.Fatalf("fragment response should include updated label")
	}
}

func TestUIUpdateSourceLabelRedirectsBackToPrivacyPage(t *testing.T) {
	s := testServer(t)

	form := url.Values{"label": {"Kitchen Display"}}
	req := httptest.NewRequest("POST", "/ui/sources/10.44.0.7/label", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/devices/source:10.44.0.7" {
		t.Fatalf("Location = %q, want %q", location, "/devices/source:10.44.0.7")
	}

	label, err := s.db.GetSourceLabel("10.44.0.7")
	if err != nil {
		t.Fatalf("GetSourceLabel: %v", err)
	}
	if label != "Kitchen Display" {
		t.Fatalf("label = %q, want %q", label, "Kitchen Display")
	}
}

func TestUIUpdateSourceLabelReturnsFragmentForHTMX(t *testing.T) {
	s := testServer(t)

	form := url.Values{"label": {"Kitchen Display"}}
	req := httptest.NewRequest("POST", "/ui/sources/10.44.0.7/label", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	if !strings.Contains(w.Body.String(), "Kitchen Display · 10.44.0.7") {
		t.Fatalf("fragment response should include updated source display name")
	}
	if !strings.Contains(w.Body.String(), `hx-post="/ui/sources/10.44.0.7/label"`) {
		t.Fatalf("fragment response should include source label form action")
	}
}

func TestHandlePrivacyReturnsFragmentForHTMXDeviceRequest(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/aa:bb:cc:dd:ee:ff", nil)
	req.Header.Set("HX-Request", "true")
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if strings.Contains(w.Body.String(), "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	if !strings.Contains(w.Body.String(), "Device Detail") {
		t.Fatalf("fragment response should include device detail")
	}
}

func TestPrivacyDevicePartial(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/ui/privacy/device/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	for _, want := range []string{
		"Device Detail",
		"Living Room TV",
		"ads.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestPrivacyDeviceAllPartial(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/ui/privacy/device-all", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	for _, want := range []string{
		"Network Domains",
		"ads.example.com",
		"2 of 2",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestPrivacyDomainRowsIncludeMobileCellLabels(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/domains", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		`data-label="Domain"`,
		`data-label="Classification"`,
		`data-label="Queries"`,
		`data-label="Reach"`,
		`data-label="Actions"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing mobile cell label %q", want)
		}
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
	if strings.Contains(body, "<html") {
		t.Fatalf("fragment response should not include layout")
	}
	if !strings.Contains(body, "manual.example.com") {
		t.Fatalf("response missing updated domain")
	}
	if !strings.Contains(body, "tracking · manual") {
		t.Fatalf("response missing updated override category/source: %s", body)
	}
}

func TestUIOverrideAcceptsUncategorizedValue(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	form := url.Values{
		"category": {"uncategorized"},
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
	if !strings.Contains(body, "uncategorized · manual") {
		t.Fatalf("response missing updated uncategorized override: %s", body)
	}
}

func TestAPIActivity(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	currentHour := now.Truncate(time.Hour)
	macA := "aa:bb:cc:dd:ee:ff"
	macB := "11:22:33:44:55:66"

	for _, mac := range []string{macA, macB} {
		if err := s.db.UpsertDevice(store.Device{
			MAC:       mac,
			IP:        "192.168.1.10",
			FirstSeen: now,
			LastSeen:  now,
		}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: macA, Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: currentHour.Add(-2 * time.Hour).Add(5 * time.Minute)},
		{DeviceMAC: macA, Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: currentHour},
		{DeviceMAC: macB, Domain: "other.example.com", QueryType: "A", Category: "analytics", Timestamp: currentHour.Add(-2 * time.Hour).Add(10 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	t.Run("all devices", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/activity", nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var buckets []struct {
			Timestamp int64 `json:"timestamp"`
			Total     int   `json:"total"`
			Tracker   int   `json:"tracker"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &buckets); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if len(buckets) != 24 {
			t.Fatalf("got %d buckets, want 24", len(buckets))
		}

		targetHour := currentHour.Add(-2 * time.Hour).Unix()
		currentBucket := currentHour.Unix()
		var foundTarget, foundCurrent bool
		for _, bucket := range buckets {
			switch bucket.Timestamp {
			case targetHour:
				foundTarget = true
				if bucket.Total != 2 || bucket.Tracker != 1 {
					t.Fatalf("target bucket = %+v, want total=2 tracker=1", bucket)
				}
			case currentBucket:
				foundCurrent = true
				if bucket.Total != 1 || bucket.Tracker != 0 {
					t.Fatalf("current bucket = %+v, want total=1 tracker=0", bucket)
				}
			}
		}
		if !foundTarget || !foundCurrent {
			t.Fatalf("missing expected buckets in response")
		}
	})

	t.Run("filtered device", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/api/activity?device="+macA, nil)
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}

		var buckets []struct {
			Timestamp int64 `json:"timestamp"`
			Total     int   `json:"total"`
			Tracker   int   `json:"tracker"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &buckets); err != nil {
			t.Fatalf("json.Unmarshal: %v", err)
		}
		if len(buckets) != 24 {
			t.Fatalf("got %d buckets, want 24", len(buckets))
		}

		targetHour := currentHour.Add(-2 * time.Hour).Unix()
		var foundTarget bool
		for _, bucket := range buckets {
			if bucket.Timestamp == targetHour {
				foundTarget = true
				if bucket.Total != 1 || bucket.Tracker != 1 {
					t.Fatalf("filtered bucket = %+v, want total=1 tracker=1", bucket)
				}
			}
		}
		if !foundTarget {
			t.Fatalf("target bucket at %d not found in filtered response", targetHour)
		}
	})
}
