package web

import (
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
		"Network Privacy",
		"Top Domains",
		"ads.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
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
		"Devices",
		`id="device-list"`,
		"Living Room TV",
		`href="/devices/`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
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
	for _, want := range []string{"Source Detail", "10.44.0.7", "Kitchen Display"} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
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
}
