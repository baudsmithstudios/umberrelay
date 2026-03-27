package store

import (
	"path/filepath"
	"testing"
	"time"
)

func testDB(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestOpenAndClose(t *testing.T) {
	db := testDB(t)
	if db == nil {
		t.Fatal("db is nil")
	}
}

func TestUpsertAndListDevices(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	err := db.UpsertDevice(Device{
		MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10",
		Hostname: "roku-tv", Vendor: "Roku",
		FirstSeen: now, LastSeen: now,
	})
	if err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	devices, err := db.ListDevices()
	if err != nil {
		t.Fatalf("ListDevices: %v", err)
	}
	if len(devices) != 1 {
		t.Fatalf("got %d devices, want 1", len(devices))
	}
	if devices[0].Hostname != "roku-tv" {
		t.Errorf("hostname = %q, want roku-tv", devices[0].Hostname)
	}
}

func TestUpsertDeviceUpdatesIP(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.20", LastSeen: now.Add(time.Minute)})
	devices, _ := db.ListDevices()
	if devices[0].IP != "192.168.1.20" {
		t.Errorf("IP = %q, want 192.168.1.20", devices[0].IP)
	}
}

func TestUpdateDeviceLabel(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})
	err := db.UpdateDeviceLabel("aa:bb:cc:dd:ee:ff", "Living Room TV")
	if err != nil {
		t.Fatalf("UpdateDeviceLabel: %v", err)
	}
	devices, _ := db.ListDevices()
	if devices[0].Label != "Living Room TV" {
		t.Errorf("label = %q, want Living Room TV", devices[0].Label)
	}
}

func TestWriteAndQueryQueries(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})

	queries := []Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "api.example.com", QueryType: "A", Category: "", Timestamp: now.Add(time.Second)},
	}
	err := db.WriteQueries(queries)
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	results, err := db.QueryLog("", "", time.Time{}, now.Add(time.Minute), 100, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestQueryLogFilters(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "11:22:33:44:55:66", IP: "192.168.1.11", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "api.example.com", QueryType: "A", Category: "", Timestamp: now},
	})

	// Filter by device
	results, _ := db.QueryLog("aa:bb:cc:dd:ee:ff", "", time.Time{}, time.Now(), 100, 0)
	if len(results) != 1 {
		t.Errorf("device filter: got %d, want 1", len(results))
	}

	// Filter by domain
	results, _ = db.QueryLog("", "ads.example.com", time.Time{}, time.Now(), 100, 0)
	if len(results) != 1 {
		t.Errorf("domain filter: got %d, want 1", len(results))
	}
}

func TestPurgeQueries(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})

	old := now.Add(-48 * time.Hour)
	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "old.com", QueryType: "A", Timestamp: old},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "new.com", QueryType: "A", Timestamp: now},
	})

	err := db.PurgeQueriesOlderThan(now.Add(-24 * time.Hour))
	if err != nil {
		t.Fatalf("PurgeQueriesOlderThan: %v", err)
	}

	results, _ := db.QueryLog("", "", time.Time{}, time.Now(), 100, 0)
	if len(results) != 1 {
		t.Fatalf("got %d after purge, want 1", len(results))
	}
	if results[0].Domain != "new.com" {
		t.Errorf("remaining domain = %q, want new.com", results[0].Domain)
	}
}

func TestConfigGetSet(t *testing.T) {
	db := testDB(t)
	err := db.SetConfig("retention_days", "30")
	if err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	val, err := db.GetConfig("retention_days")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "30" {
		t.Errorf("got %q, want 30", val)
	}
}

func TestConfigGetMissing(t *testing.T) {
	db := testDB(t)
	val, err := db.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if val != "" {
		t.Errorf("got %q, want empty string", val)
	}
}

func TestListDomainCache(t *testing.T) {
	db := testDB(t)
	// Add a list
	id, err := db.AddList("https://example.com/list.txt", "Test List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	// Write cached domains
	domains := map[string]string{
		"ads.example.com":     "tracking",
		"tracker.example.com": "tracking",
	}
	err = db.WriteListDomains(id, domains)
	if err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	// Read cached domains
	cached, err := db.LoadCachedDomains()
	if err != nil {
		t.Fatalf("LoadCachedDomains: %v", err)
	}
	if len(cached) != 2 {
		t.Fatalf("got %d cached domains, want 2", len(cached))
	}
	if cached["ads.example.com"] != "tracking" {
		t.Errorf("ads.example.com category = %q, want tracking", cached["ads.example.com"])
	}
}

func TestDomainOverrides(t *testing.T) {
	db := testDB(t)
	err := db.SetDomainOverride("ads.example.com", "telemetry")
	if err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	overrides, err := db.ListDomainOverrides()
	if err != nil {
		t.Fatalf("ListDomainOverrides: %v", err)
	}
	if len(overrides) != 1 {
		t.Fatalf("got %d overrides, want 1", len(overrides))
	}
	if overrides["ads.example.com"] != "telemetry" {
		t.Errorf("override = %q, want telemetry", overrides["ads.example.com"])
	}

	err = db.DeleteDomainOverride("ads.example.com")
	if err != nil {
		t.Fatalf("DeleteDomainOverride: %v", err)
	}
	overrides, _ = db.ListDomainOverrides()
	if len(overrides) != 0 {
		t.Errorf("got %d overrides after delete, want 0", len(overrides))
	}
}

func TestDashboardSummary(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", Hostname: "roku-tv", Vendor: "Roku", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "11:22:33:44:55:66", IP: "192.168.1.11", Hostname: "laptop", Vendor: "Dell", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "api.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now},
	})

	summary, err := db.DashboardSummary()
	if err != nil {
		t.Fatalf("DashboardSummary: %v", err)
	}
	if summary.TotalQueries != 4 {
		t.Errorf("TotalQueries = %d, want 4", summary.TotalQueries)
	}
	if summary.TrackerPercent != 50.0 {
		t.Errorf("TrackerPercent = %f, want 50.0", summary.TrackerPercent)
	}
	if summary.DeviceCount != 2 {
		t.Errorf("DeviceCount = %d, want 2", summary.DeviceCount)
	}
	if len(summary.TopDevices) != 2 {
		t.Fatalf("TopDevices count = %d, want 2", len(summary.TopDevices))
	}
	if summary.TopDevices[0].QueryCount != 3 {
		t.Errorf("top device query count = %d, want 3", summary.TopDevices[0].QueryCount)
	}
}

func TestTopDomains(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "11:22:33:44:55:66", IP: "192.168.1.11", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "popular.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "popular.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
	})

	domains, err := db.TopDomains(10)
	if err != nil {
		t.Fatalf("TopDomains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
	if domains[0].Domain != "popular.com" {
		t.Errorf("top domain = %q, want popular.com", domains[0].Domain)
	}
	if domains[0].QueryCount != 2 {
		t.Errorf("query count = %d, want 2", domains[0].QueryCount)
	}
	if domains[0].DeviceCount != 2 {
		t.Errorf("device count = %d, want 2", domains[0].DeviceCount)
	}
}

func TestDeviceTopDomains(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
	})

	domains, err := db.DeviceTopDomains("aa:bb:cc:dd:ee:ff", 10)
	if err != nil {
		t.Fatalf("DeviceTopDomains: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
	if domains[0].Domain != "example.com" || domains[0].Count != 2 {
		t.Errorf("top domain = %q count = %d, want example.com/2", domains[0].Domain, domains[0].Count)
	}
}

func TestListDevicesWithStats(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", Hostname: "roku-tv", Vendor: "Roku", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "11:22:33:44:55:66", IP: "192.168.1.11", Hostname: "laptop", Vendor: "Dell", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now},
	})

	results, err := db.ListDevicesWithStats()
	if err != nil {
		t.Fatalf("ListDevicesWithStats: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d devices, want 2", len(results))
	}
	// Ordered by query count desc — roku-tv has 2 queries
	if results[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Errorf("first device MAC = %q, want aa:bb:cc:dd:ee:ff", results[0].MAC)
	}
	if results[0].QueryCount != 2 {
		t.Errorf("first device QueryCount = %d, want 2", results[0].QueryCount)
	}
	if results[0].TrackerPercent != 50.0 {
		t.Errorf("first device TrackerPercent = %f, want 50.0", results[0].TrackerPercent)
	}
}

func TestDeviceStats(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})

	db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now},
	})

	stats, err := db.DeviceStats("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("DeviceStats: %v", err)
	}
	if stats.QueryCount != 2 {
		t.Errorf("QueryCount = %d, want 2", stats.QueryCount)
	}
	if stats.TrackerPercent != 50.0 {
		t.Errorf("TrackerPercent = %f, want 50.0", stats.TrackerPercent)
	}
}
