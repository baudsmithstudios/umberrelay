package web

import (
	"encoding/json"
	"html"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"scrye/internal/store"
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

func TestDashboardPage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestDashboardPageTrends(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	mac := "aa:bb:cc:dd:ee:ff"

	seedTrendPageDevice(t, s, mac, "roku-tv", now)

	var queries []store.Query
	for i := 0; i < 7; i++ {
		queries = append(queries, store.Query{
			DeviceMAC: mac,
			Domain:    "prior.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-48 * time.Hour).Add(time.Duration(i) * time.Minute),
		})
	}
	queries = append(queries,
		store.Query{DeviceMAC: mac, Domain: "current-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		store.Query{DeviceMAC: mac, Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-time.Hour)},
	)

	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	for _, want := range []string{
		`class="trend-up">+100%</small>`,
		`class="trend-up">+50pp</small>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestDashboardPageUsesSharedTrendWindow(t *testing.T) {
	s := testServer(t)
	now := time.Date(2026, 3, 27, 12, 0, 0, 0, time.UTC)
	s.now = func() time.Time { return now }

	mac := "aa:bb:cc:dd:ee:ff"
	seedTrendPageDevice(t, s, mac, "roku-tv", now)

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: mac, Domain: "prior.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-24*time.Hour - time.Nanosecond)},
		{DeviceMAC: mac, Domain: "current.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-24*time.Hour + time.Nanosecond)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	if !strings.Contains(body, `>1<small class="trend-up">+600%</small>`) {
		t.Fatalf("response missing query value/trend pair from shared window: %s", body)
	}
	if !strings.Contains(body, `>100%<small class="trend-up">+100pp</small>`) {
		t.Fatalf("response missing tracker value/trend pair from shared window: %s", body)
	}
}

func TestDevicesPage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestDevicesPageTrends(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	trendingMAC := "aa:bb:cc:dd:ee:ff"
	currentOnlyMAC := "11:22:33:44:55:66"

	seedTrendPageDevice(t, s, trendingMAC, "roku-tv", now)
	seedTrendPageDevice(t, s, currentOnlyMAC, "laptop", now)

	var queries []store.Query
	for i := 0; i < 7; i++ {
		queries = append(queries, store.Query{
			DeviceMAC: trendingMAC,
			Domain:    "prior.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-48 * time.Hour).Add(time.Duration(i) * time.Minute),
		})
	}
	queries = append(queries,
		store.Query{DeviceMAC: trendingMAC, Domain: "current-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		store.Query{DeviceMAC: trendingMAC, Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-time.Hour)},
		store.Query{DeviceMAC: currentOnlyMAC, Domain: "current-only.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-3 * time.Hour)},
	)

	if err := s.db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := html.UnescapeString(w.Body.String())
	if !strings.Contains(body, `class="trend-up">+100%</small>`) {
		t.Fatalf("response missing query trend")
	}
	if !strings.Contains(body, `class="trend-up">+50pp</small>`) {
		t.Fatalf("response missing tracker trend")
	}
}

func TestDomainsPage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/domains", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
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

func TestDeviceDetailPage(t *testing.T) {
	s := testServer(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := s.db.UpsertDevice(store.Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		Hostname:  "roku-tv",
		Vendor:    "Roku",
		Label:     "Living Room TV",
		FirstSeen: now,
		LastSeen:  now,
	})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	err = s.db.WriteQueries([]store.Query{
		{DeviceMAC: mac, Domain: "shared.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: mac, Domain: "shared.example.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(time.Second)},
		{DeviceMAC: mac, Domain: "stats.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: mac, Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(3 * time.Second)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices/"+mac, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		"Privacy Summary",
		"Total Queries",
		"Unique Domains",
		"Unique Tracker Domains",
		"Category Breakdown",
		"tracking",
		"analytics",
		"<em>none</em>",
		"50.0%",
		"25.0%",
		"shared.example.com",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
	}
}

func TestDeviceDetailPageTrendSuppression(t *testing.T) {
	s := testServer(t)
	now := time.Now().UTC()
	mac := "aa:bb:cc:dd:ee:ff"

	seedTrendPageDevice(t, s, mac, "roku-tv", now)

	if err := s.db.WriteQueries([]store.Query{
		{DeviceMAC: mac, Domain: "prior-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: mac, Domain: "prior-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-47 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices/"+mac, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, `class="trend-down">-100%</small>`) {
		t.Fatalf("response missing query trend")
	}
	if strings.Contains(body, "pp</small>") {
		t.Fatalf("response should suppress tracker trend when current period has no queries")
	}
}

func TestDeviceDetailPageNotFound(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/devices/aa:bb:cc:dd:ee:ff", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDeviceDetailPageZeroQuery(t *testing.T) {
	s := testServer(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := s.db.UpsertDevice(store.Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		Hostname:  "roku-tv",
		Vendor:    "Roku",
		FirstSeen: now,
		LastSeen:  now,
	})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	req := httptest.NewRequest("GET", "/devices/"+mac, nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	body := w.Body.String()
	for _, want := range []string{
		"Privacy Summary",
		"0.0%",
		"Category Breakdown",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("response missing %q", want)
		}
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
				if bucket.Total != 2 || bucket.Tracker != 2 {
					t.Fatalf("target bucket = %+v, want total=2 tracker=2", bucket)
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
