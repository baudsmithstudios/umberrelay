package web

import (
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
