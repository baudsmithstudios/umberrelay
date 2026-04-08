package web

import (
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

func TestDeviceTrendDisplayName(t *testing.T) {
	tests := []struct {
		name string
		row  deviceTrendRow
		want string
	}{
		{
			name: "source actor",
			row: deviceTrendRow{
				ActorType: actorTypeSource,
				SourceIP:  "10.55.0.17",
			},
			want: "Unattributed \u00b7 10.55.0.17",
		},
		{
			name: "device label",
			row: deviceTrendRow{
				ActorType: actorTypeDevice,
				Label:     "Living Room TV",
				MAC:       "aa:bb:cc:dd:ee:ff",
			},
			want: "Living Room TV",
		},
		{
			name: "device fallback mac",
			row: deviceTrendRow{
				ActorType: actorTypeDevice,
				MAC:       "aa:bb:cc:dd:ee:ff",
			},
			want: "aa:bb:cc:dd:ee:ff",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deviceTrendDisplayName(tt.row); got != tt.want {
				t.Fatalf("deviceTrendDisplayName() = %q, want %q", got, tt.want)
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
	for _, avoid := range []string{
		"/static/css/devices.css",
		"/static/css/device_detail.css",
	} {
		if strings.Contains(body, avoid) {
			t.Fatalf("response should not include %q", avoid)
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

func TestHomePageTopDomainsUseClassificationPillClasses(t *testing.T) {
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
	for _, want := range []string{
		`class="classification-pill classification-pill-tracking"`,
		`class="classification-pill classification-pill-analytics"`,
		`class="classification-pill classification-pill-unclassified"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing classification pill class %q", want)
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
	for _, avoid := range []string{
		"/static/css/home.css",
		"/static/css/device_detail.css",
	} {
		if strings.Contains(body, avoid) {
			t.Fatalf("response should not include %q", avoid)
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

func TestDomainsRouteRedirectsToDevices(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/domains", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusMovedPermanently {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusMovedPermanently)
	}
	if location := w.Header().Get("Location"); location != "/devices" {
		t.Fatalf("Location = %q, want %q", location, "/devices")
	}
}

func TestDevicesListSortsDevicesByTrackerPercentDescending(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	// high-rate device (100% tracker) should appear before low-rate (20% tracker)
	highIdx := strings.Index(body, "high-rate")
	lowIdx := strings.Index(body, "low-rate")
	if highIdx < 0 || lowIdx < 0 {
		t.Fatalf("response missing expected device names")
	}
	if highIdx > lowIdx {
		t.Fatalf("high-rate device should appear before low-rate device (sorted by tracker %%)")
	}
}

func TestDeviceDetailPageRendersAllSections(t *testing.T) {
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
		"Device Detail",
		"Living Room TV",
		"All Devices",
		"/static/js/charts.js",
		"/static/js/feed.js",
		"/static/css/device_detail.css",
		"device-detail-page",
		"Domains",
		"Live Query Stream",
		`data-actor-key="device:aa:bb:cc:dd:ee:ff"`,
		"ads.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
	for _, avoid := range []string{
		"/static/css/home.css",
		"/static/css/devices.css",
	} {
		if strings.Contains(body, avoid) {
			t.Fatalf("response should not include %q", avoid)
		}
	}
}

func TestDeviceDetailPageSourceRoute(t *testing.T) {
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
		"Kitchen Display",
		"device-detail-page",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
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

func TestDeviceDetailIncludesLiveQueryStreamUI(t *testing.T) {
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
		`/static/js/feed.js`,
		`/static/js/charts.js`,
		`data-actor-key="device:aa:bb:cc:dd:ee:ff"`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestDevicesListIncludesSourceFallbackActors(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "10.44.0.7") {
		t.Fatalf("devices list should include source fallback actor")
	}
	if !strings.Contains(body, "Unattributed") {
		t.Fatalf("devices list should show unattributed label for source actor")
	}
}

func TestDevicesListIncludesBypassSignals(t *testing.T) {
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

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "anomaly-bypass") {
		t.Fatalf("devices list should show bypass anomaly badge")
	}
}

func TestDeviceDetailUnknownDeviceRedirectsToDevicesList(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	s.now = func() time.Time { return now }
	seedPrivacyPageData(t, s, now)

	req := httptest.NewRequest("GET", "/devices/ff:ee:dd:cc:bb:aa", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusSeeOther)
	}
	if location := w.Header().Get("Location"); location != "/devices" {
		t.Fatalf("Location = %q, want %q", location, "/devices")
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

func TestUIListActionsRedirectBackToSettings(t *testing.T) {
	tests := []struct {
		name       string
		withClassy bool
		makeReq    func(t *testing.T, s *Server) *http.Request
		assertDB   func(t *testing.T, s *Server)
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
				req := httptest.NewRequest("POST", "/ui/lists/"+itoa(id)+"/enabled", strings.NewReader(form.Encode()))
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
				return httptest.NewRequest("POST", "/ui/lists/"+itoa(id)+"/delete", nil)
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
			name:       "refresh lists",
			withClassy: true,
			makeReq: func(_ *testing.T, _ *Server) *http.Request {
				return httptest.NewRequest("POST", "/ui/lists/refresh", nil)
			},
			assertDB: func(_ *testing.T, _ *Server) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s *Server
			if tt.withClassy {
				s = testServerWithClassify(t)
			} else {
				s = testServer(t)
			}
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

func TestDeviceDetailReturnsFragmentForHTMXRequest(t *testing.T) {
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
