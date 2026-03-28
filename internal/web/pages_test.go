package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"scrye/internal/store"
)

func TestDashboardPage(t *testing.T) {
	s := testServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
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
