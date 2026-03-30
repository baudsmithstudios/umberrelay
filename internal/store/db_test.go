package store

import (
	"math"
	"path/filepath"
	"strings"
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
	if summary.UniqueDomainCount != 4 {
		t.Errorf("UniqueDomainCount = %d, want 4", summary.UniqueDomainCount)
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
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "example.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(time.Second)},
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
	if domains[0].Category != "tracking" {
		t.Errorf("top domain category = %q, want tracking", domains[0].Category)
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

func seedTrendTestDevice(t *testing.T, db *DB, mac, hostname string, now time.Time) {
	t.Helper()

	err := db.UpsertDevice(Device{
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

func almostEqualFloat64(got, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func TestDashboardTrends(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	seedTrendTestDevice(t, db, "aa:bb:cc:dd:ee:ff", "roku-tv", now)

	var queries []Query
	for i := 0; i < 7; i++ {
		ts := now.Add(-48 * time.Hour).Add(time.Duration(i) * time.Minute)
		queries = append(queries,
			Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-clean.example.com", QueryType: "A", Category: "", Timestamp: ts},
			Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: ts.Add(time.Second)},
		)
	}
	queries = append(queries,
		Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-clean-2.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-90 * time.Minute)},
		Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-tracker.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-time.Hour)},
	)

	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	queryTrend, trackerTrend, err := db.DashboardTrends()
	if err != nil {
		t.Fatalf("DashboardTrends: %v", err)
	}

	if queryTrend.Current != 3 {
		t.Fatalf("query current = %v, want 3", queryTrend.Current)
	}
	if queryTrend.Previous != 2 {
		t.Fatalf("query previous = %v, want 2", queryTrend.Previous)
	}
	if queryTrend.Change != 50 {
		t.Fatalf("query change = %v, want 50", queryTrend.Change)
	}
	if !queryTrend.HasPrior {
		t.Fatalf("query HasPrior = false, want true")
	}

	if trackerTrend.Current != 0 {
		t.Fatalf("tracker current = %v, want 0", trackerTrend.Current)
	}
	if trackerTrend.Previous != 50 {
		t.Fatalf("tracker previous = %v, want 50", trackerTrend.Previous)
	}
	if trackerTrend.Change != -50 {
		t.Fatalf("tracker change = %v, want -50", trackerTrend.Change)
	}
	if !trackerTrend.HasPrior {
		t.Fatalf("tracker HasPrior = false, want true")
	}
}

func TestDashboardTrendsNoPriorData(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	seedTrendTestDevice(t, db, "aa:bb:cc:dd:ee:ff", "roku-tv", now)

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	queryTrend, trackerTrend, err := db.DashboardTrends()
	if err != nil {
		t.Fatalf("DashboardTrends: %v", err)
	}

	if queryTrend.Current != 2 || queryTrend.Previous != 0 || queryTrend.Change != 0 || queryTrend.HasPrior {
		t.Fatalf("query trend = %#v, want current=2 previous=0 change=0 HasPrior=false", queryTrend)
	}
	if trackerTrend.Current != 50 || trackerTrend.Previous != 0 || trackerTrend.Change != 0 || trackerTrend.HasPrior {
		t.Fatalf("tracker trend = %#v, want current=50 previous=0 change=0 HasPrior=false", trackerTrend)
	}
}

func TestDashboardTrendsNoCurrentData(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	seedTrendTestDevice(t, db, "aa:bb:cc:dd:ee:ff", "roku-tv", now)

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-47 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	queryTrend, trackerTrend, err := db.DashboardTrends()
	if err != nil {
		t.Fatalf("DashboardTrends: %v", err)
	}

	if queryTrend.Current != 0 || queryTrend.Previous != (2.0/7.0) || !queryTrend.HasPrior {
		t.Fatalf("query trend = %#v, want current=0 previous=2/7 HasPrior=true", queryTrend)
	}
	if trackerTrend.Current != 0 || trackerTrend.Previous != 50 || trackerTrend.Change != 0 || trackerTrend.HasPrior {
		t.Fatalf("tracker trend = %#v, want current=0 previous=50 change=0 HasPrior=false", trackerTrend)
	}
}

func TestDashboardTrendsEmpty(t *testing.T) {
	db := testDB(t)

	queryTrend, trackerTrend, err := db.DashboardTrends()
	if err != nil {
		t.Fatalf("DashboardTrends: %v", err)
	}

	if queryTrend != (Trend{}) {
		t.Fatalf("query trend = %#v, want zero value", queryTrend)
	}
	if trackerTrend != (Trend{}) {
		t.Fatalf("tracker trend = %#v, want zero value", trackerTrend)
	}
}

func TestDeviceTrends(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	seedTrendTestDevice(t, db, "aa:bb:cc:dd:ee:ff", "roku-tv", now)
	seedTrendTestDevice(t, db, "11:22:33:44:55:66", "laptop", now)

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-47 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "other-device.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	queryTrend, trackerTrend, err := db.DeviceTrends("aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("DeviceTrends: %v", err)
	}

	if queryTrend.Current != 1 || queryTrend.Previous != (2.0/7.0) || !queryTrend.HasPrior {
		t.Fatalf("query trend = %#v, want current=1 previous=2/7 HasPrior=true", queryTrend)
	}
	if trackerTrend.Current != 100 || trackerTrend.Previous != 50 || !trackerTrend.HasPrior {
		t.Fatalf("tracker trend = %#v, want current=100 previous=50 HasPrior=true", trackerTrend)
	}
}

func TestListDevicesWithTrends(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	seedTrendTestDevice(t, db, "aa:bb:cc:dd:ee:ff", "roku-tv", now)
	seedTrendTestDevice(t, db, "11:22:33:44:55:66", "laptop", now)
	seedTrendTestDevice(t, db, "22:33:44:55:66:77", "tablet", now)

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "prior-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-47 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "current-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-time.Hour)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "current-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-3 * time.Hour)},
		{DeviceMAC: "22:33:44:55:66:77", Domain: "stale.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-9 * 24 * time.Hour)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	results, err := db.ListDevicesWithTrends()
	if err != nil {
		t.Fatalf("ListDevicesWithTrends: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("got %d devices, want 3", len(results))
	}

	if results[0].MAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("first device MAC = %q, want aa:bb:cc:dd:ee:ff", results[0].MAC)
	}
	if results[0].QueryCount != 2 || results[0].TrackerPercent != 50 {
		t.Fatalf("first device stats = %#v, want QueryCount=2 TrackerPercent=50", results[0])
	}
	if !results[0].QueryTrend.HasPrior || results[0].QueryTrend.Previous != (2.0/7.0) {
		t.Fatalf("first device query trend = %#v, want HasPrior=true Previous=2/7", results[0].QueryTrend)
	}
	if !results[0].TrackerTrend.HasPrior || results[0].TrackerTrend.Previous != 50 {
		t.Fatalf("first device tracker trend = %#v, want HasPrior=true Previous=50", results[0].TrackerTrend)
	}

	if results[1].MAC != "11:22:33:44:55:66" {
		t.Fatalf("second device MAC = %q, want 11:22:33:44:55:66", results[1].MAC)
	}
	if results[1].QueryCount != 1 || results[1].TrackerPercent != 0 {
		t.Fatalf("second device stats = %#v, want QueryCount=1 TrackerPercent=0", results[1])
	}
	if results[1].QueryTrend.HasPrior || results[1].TrackerTrend.HasPrior {
		t.Fatalf("second device trends = %#v / %#v, want no prior data", results[1].QueryTrend, results[1].TrackerTrend)
	}

	if results[2].MAC != "22:33:44:55:66:77" {
		t.Fatalf("third device MAC = %q, want 22:33:44:55:66:77", results[2].MAC)
	}
	if results[2].QueryCount != 0 || results[2].TrackerPercent != 0 {
		t.Fatalf("third device stats = %#v, want current counts zero", results[2])
	}
	if results[2].QueryTrend.HasPrior || results[2].TrackerTrend.HasPrior {
		t.Fatalf("third device trends = %#v / %#v, want zeroed 8-day window", results[2].QueryTrend, results[2].TrackerTrend)
	}
}

func TestDeviceCategoryBreakdown(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := db.UpsertDevice(Device{
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

	err = db.WriteQueries([]Query{
		{DeviceMAC: mac, Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: mac, Domain: "pixel.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(time.Second)},
		{DeviceMAC: mac, Domain: "stats.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: mac, Domain: "app.example.com", QueryType: "A", Category: "", Timestamp: now.Add(3 * time.Second)},
		{DeviceMAC: mac, Domain: "old.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(-25 * time.Hour)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "other.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	breakdown, err := db.DeviceCategoryBreakdown(mac)
	if err != nil {
		t.Fatalf("DeviceCategoryBreakdown: %v", err)
	}

	want := []CategoryCount{
		{Category: "tracking", Count: 2},
		{Category: "", Count: 1},
		{Category: "analytics", Count: 1},
	}
	if len(breakdown) != len(want) {
		t.Fatalf("got %d rows, want %d", len(breakdown), len(want))
	}
	for i := range want {
		if breakdown[i] != want[i] {
			t.Fatalf("row %d = %#v, want %#v", i, breakdown[i], want[i])
		}
	}
}

func TestDeviceCategoryBreakdownEmpty(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := db.UpsertDevice(Device{
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

	breakdown, err := db.DeviceCategoryBreakdown(mac)
	if err != nil {
		t.Fatalf("DeviceCategoryBreakdown: %v", err)
	}
	if len(breakdown) != 0 {
		t.Fatalf("got %d rows, want 0", len(breakdown))
	}
}

func TestDevicePrivacySummary(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := db.UpsertDevice(Device{
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

	err = db.WriteQueries([]Query{
		{DeviceMAC: mac, Domain: "shared.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: mac, Domain: "shared.example.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(time.Second)},
		{DeviceMAC: mac, Domain: "stats.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: mac, Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(3 * time.Second)},
		{DeviceMAC: mac, Domain: "old.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-25 * time.Hour)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "other.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	summary, err := db.DevicePrivacySummary(mac)
	if err != nil {
		t.Fatalf("DevicePrivacySummary: %v", err)
	}
	if summary.QueryCount != 4 {
		t.Errorf("QueryCount = %d, want 4", summary.QueryCount)
	}
	if summary.TrackerPercent != 25.0 {
		t.Errorf("TrackerPercent = %f, want 25.0", summary.TrackerPercent)
	}
	if summary.UniqueDomains != 3 {
		t.Errorf("UniqueDomains = %d, want 3", summary.UniqueDomains)
	}
	if summary.UniqueTrackerDomains != 1 {
		t.Errorf("UniqueTrackerDomains = %d, want 1", summary.UniqueTrackerDomains)
	}
}

func TestDevicePrivacySummaryEmpty(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	err := db.UpsertDevice(Device{
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

	summary, err := db.DevicePrivacySummary(mac)
	if err != nil {
		t.Fatalf("DevicePrivacySummary: %v", err)
	}
	if summary.QueryCount != 0 {
		t.Errorf("QueryCount = %d, want 0", summary.QueryCount)
	}
	if summary.TrackerPercent != 0 {
		t.Errorf("TrackerPercent = %f, want 0", summary.TrackerPercent)
	}
	if summary.UniqueDomains != 0 {
		t.Errorf("UniqueDomains = %d, want 0", summary.UniqueDomains)
	}
	if summary.UniqueTrackerDomains != 0 {
		t.Errorf("UniqueTrackerDomains = %d, want 0", summary.UniqueTrackerDomains)
	}
}

func TestHourlyActivity(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()
	currentHour := now.Truncate(time.Hour)
	oldestHour := currentHour.Add(-23 * time.Hour)

	deviceA := "aa:bb:cc:dd:ee:ff"
	deviceB := "11:22:33:44:55:66"
	for _, mac := range []string{deviceA, deviceB} {
		if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}

	queries := []Query{
		{DeviceMAC: deviceA, Domain: "oldest.example.com", QueryType: "A", Category: "", Timestamp: oldestHour},
		{DeviceMAC: deviceA, Domain: "mid-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: currentHour.Add(-5 * time.Hour).Add(15 * time.Minute)},
		{DeviceMAC: deviceA, Domain: "mid-clean.example.com", QueryType: "A", Category: "", Timestamp: currentHour.Add(-5 * time.Hour).Add(45 * time.Minute)},
		{DeviceMAC: deviceA, Domain: "current-tracker.example.com", QueryType: "A", Category: "analytics", Timestamp: currentHour},
		{DeviceMAC: deviceB, Domain: "other-device.example.com", QueryType: "A", Category: "tracking", Timestamp: currentHour.Add(-5 * time.Hour).Add(5 * time.Minute)},
		{DeviceMAC: deviceA, Domain: "outside-window.example.com", QueryType: "A", Category: "tracking", Timestamp: oldestHour.Add(-time.Nanosecond)},
	}
	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	buckets, err := db.HourlyActivity("")
	if err != nil {
		t.Fatalf("HourlyActivity: %v", err)
	}
	if len(buckets) != 24 {
		t.Fatalf("got %d buckets, want 24", len(buckets))
	}

	for i, bucket := range buckets {
		want := oldestHour.Add(time.Duration(i) * time.Hour)
		if !bucket.Timestamp.Equal(want) {
			t.Fatalf("bucket %d timestamp = %v, want %v", i, bucket.Timestamp, want)
		}
	}

	if buckets[0].TotalCount != 1 || buckets[0].TrackerCount != 0 {
		t.Fatalf("oldest bucket = %+v, want total=1 tracker=0", buckets[0])
	}

	midIndex := 18
	if !buckets[midIndex].Timestamp.Equal(currentHour.Add(-5 * time.Hour)) {
		t.Fatalf("mid bucket timestamp = %v, want %v", buckets[midIndex].Timestamp, currentHour.Add(-5*time.Hour))
	}
	if buckets[midIndex].TotalCount != 3 || buckets[midIndex].TrackerCount != 2 {
		t.Fatalf("mid bucket = %+v, want total=3 tracker=2", buckets[midIndex])
	}

	if buckets[len(buckets)-1].TotalCount != 1 || buckets[len(buckets)-1].TrackerCount != 0 {
		t.Fatalf("current bucket = %+v, want total=1 tracker=0", buckets[len(buckets)-1])
	}

	if buckets[1].TotalCount != 0 || buckets[1].TrackerCount != 0 {
		t.Fatalf("empty bucket = %+v, want zero-filled counts", buckets[1])
	}

	total := 0
	for _, bucket := range buckets {
		total += bucket.TotalCount
	}
	if total != 5 {
		t.Fatalf("sum of bucket totals = %d, want 5", total)
	}

	filtered, err := db.HourlyActivity(deviceA)
	if err != nil {
		t.Fatalf("HourlyActivity(device): %v", err)
	}
	if len(filtered) != 24 {
		t.Fatalf("filtered bucket count = %d, want 24", len(filtered))
	}
	if filtered[midIndex].TotalCount != 2 || filtered[midIndex].TrackerCount != 1 {
		t.Fatalf("filtered mid bucket = %+v, want total=2 tracker=1", filtered[midIndex])
	}

	filteredTotal := 0
	for _, bucket := range filtered {
		filteredTotal += bucket.TotalCount
	}
	if filteredTotal != 4 {
		t.Fatalf("filtered sum of bucket totals = %d, want 4", filteredTotal)
	}
}

func TestHourlyActivityUsesExpectedIndex(t *testing.T) {
	db := testDB(t)

	tests := []struct {
		name     string
		query    string
		args     []any
		wantPlan string
	}{
		{
			name:     "global activity",
			query:    "EXPLAIN QUERY PLAN SELECT timestamp / ? AS hour_key, COUNT(*), COALESCE(SUM(CASE WHEN category IN ('tracking', 'advertising', 'malware') THEN 1 ELSE 0 END), 0) FROM queries WHERE timestamp >= ? GROUP BY hour_key ORDER BY hour_key",
			args:     []any{int64(time.Hour), time.Now().Add(-24 * time.Hour).UnixNano()},
			wantPlan: "idx_queries_ts",
		},
		{
			name:     "device activity",
			query:    "EXPLAIN QUERY PLAN SELECT timestamp / ? AS hour_key, COUNT(*), COALESCE(SUM(CASE WHEN category IN ('tracking', 'advertising', 'malware') THEN 1 ELSE 0 END), 0) FROM queries WHERE timestamp >= ? AND device_mac = ? GROUP BY hour_key ORDER BY hour_key",
			args:     []any{int64(time.Hour), time.Now().Add(-24 * time.Hour).UnixNano(), "aa:bb:cc:dd:ee:ff"},
			wantPlan: "idx_queries_device",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows, err := db.sql.Query(tt.query, tt.args...)
			if err != nil {
				t.Fatalf("EXPLAIN QUERY PLAN: %v", err)
			}
			defer rows.Close()

			var details []string
			for rows.Next() {
				var id, parent, notUsed int
				var detail string
				if err := rows.Scan(&id, &parent, &notUsed, &detail); err != nil {
					t.Fatalf("rows.Scan: %v", err)
				}
				details = append(details, detail)
			}
			if err := rows.Err(); err != nil {
				t.Fatalf("rows.Err: %v", err)
			}

			plan := strings.Join(details, "\n")
			if !strings.Contains(plan, tt.wantPlan) {
				t.Fatalf("query plan %q does not mention %q", plan, tt.wantPlan)
			}
		})
	}
}

func TestDashboardSummaryUsesTrackingGroupAndUniqueDomainCount(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	if err := db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice(deviceA): %v", err)
	}
	if err := db.UpsertDevice(Device{MAC: "11:22:33:44:55:66", IP: "192.168.1.11", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice(deviceB): %v", err)
	}

	err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "malware.example.com", QueryType: "A", Category: "malware", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "analytics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(3 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "uncategorized.example.com", QueryType: "A", Category: "", Timestamp: now.Add(4 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "telemetry.example.com", QueryType: "A", Category: "telemetry", Timestamp: now.Add(5 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "tracker.example.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(6 * time.Second)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	summary, err := db.DashboardSummaryAt(now.Add(10 * time.Second))
	if err != nil {
		t.Fatalf("DashboardSummaryAt: %v", err)
	}
	if summary.TotalQueries != 7 {
		t.Fatalf("TotalQueries = %d, want 7", summary.TotalQueries)
	}
	if summary.UniqueDomainCount != 6 {
		t.Fatalf("UniqueDomainCount = %d, want 6", summary.UniqueDomainCount)
	}
	if !almostEqualFloat64(summary.TrackerPercent, 4.0/7.0*100) {
		t.Fatalf("TrackerPercent = %v, want %v", summary.TrackerPercent, 4.0/7.0*100)
	}
	if len(summary.TopDevices) != 2 {
		t.Fatalf("TopDevices count = %d, want 2", len(summary.TopDevices))
	}
	if !almostEqualFloat64(summary.TopDevices[0].TrackerPercent, 3.0/4.0*100) {
		t.Fatalf("top device TrackerPercent = %v, want %v", summary.TopDevices[0].TrackerPercent, 3.0/4.0*100)
	}
}

func TestNetworkCategoryBreakdown(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	err := db.WriteQueries([]Query{
		{DeviceMAC: mac, Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: mac, Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(time.Second)},
		{DeviceMAC: mac, Domain: "uncategorized.example.com", QueryType: "A", Category: "uncategorized", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: mac, Domain: "analytics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(3 * time.Second)},
		{DeviceMAC: mac, Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(4 * time.Second)},
		{DeviceMAC: mac, Domain: "telemetry.example.com", QueryType: "A", Category: "telemetry", Timestamp: now.Add(5 * time.Second)},
		{DeviceMAC: mac, Domain: "old.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-25 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	breakdown, err := db.NetworkCategoryBreakdown()
	if err != nil {
		t.Fatalf("NetworkCategoryBreakdown: %v", err)
	}

	want := []CategoryCount{
		{Category: "tracking", Count: 2},
		{Category: "unclassified", Count: 3},
		{Category: "analytics", Count: 1},
	}
	if len(breakdown) != len(want) {
		t.Fatalf("got %d rows, want %d", len(breakdown), len(want))
	}
	for i := range want {
		if breakdown[i] != want[i] {
			t.Fatalf("row %d = %#v, want %#v", i, breakdown[i], want[i])
		}
	}
}

func TestRangedActivity(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()
	deviceA := "aa:bb:cc:dd:ee:ff"
	deviceB := "11:22:33:44:55:66"

	for _, mac := range []string{deviceA, deviceB} {
		if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}

	err := db.WriteQueries([]Query{
		{DeviceMAC: deviceA, Domain: "current-tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: deviceA, Domain: "current-analytics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-time.Hour)},
		{DeviceMAC: deviceA, Domain: "day-two-tracker.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: deviceB, Domain: "day-two-clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-48*time.Hour + time.Minute)},
		{DeviceMAC: deviceA, Domain: "day-six-tracker.example.com", QueryType: "A", Category: "malware", Timestamp: now.Add(-6 * 24 * time.Hour)},
		{DeviceMAC: deviceA, Domain: "stale.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-31 * 24 * time.Hour)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	hourly, err := db.RangedActivity("", "24h")
	if err != nil {
		t.Fatalf("RangedActivity(24h): %v", err)
	}
	if len(hourly) != 24 {
		t.Fatalf("24h bucket count = %d, want 24", len(hourly))
	}
	hourlyTotal := 0
	hourlyTracker := 0
	for _, bucket := range hourly {
		hourlyTotal += bucket.TotalCount
		hourlyTracker += bucket.TrackerCount
	}
	if hourlyTotal != 2 || hourlyTracker != 1 {
		t.Fatalf("24h totals = %d/%d, want 2/1", hourlyTotal, hourlyTracker)
	}

	daily, err := db.RangedActivity("", "7d")
	if err != nil {
		t.Fatalf("RangedActivity(7d): %v", err)
	}
	if len(daily) != 7 {
		t.Fatalf("7d bucket count = %d, want 7", len(daily))
	}
	dailyTotal := 0
	dailyTracker := 0
	for _, bucket := range daily {
		dailyTotal += bucket.TotalCount
		dailyTracker += bucket.TrackerCount
	}
	if dailyTotal != 5 || dailyTracker != 3 {
		t.Fatalf("7d totals = %d/%d, want 5/3", dailyTotal, dailyTracker)
	}

	filtered, err := db.RangedActivity(deviceA, "7d")
	if err != nil {
		t.Fatalf("RangedActivity(device, 7d): %v", err)
	}
	filteredTotal := 0
	filteredTracker := 0
	for _, bucket := range filtered {
		filteredTotal += bucket.TotalCount
		filteredTracker += bucket.TrackerCount
	}
	if filteredTotal != 4 || filteredTracker != 3 {
		t.Fatalf("filtered totals = %d/%d, want 4/3", filteredTotal, filteredTracker)
	}
}

func TestRangedActivityRejectsInvalidRange(t *testing.T) {
	db := testDB(t)

	_, err := db.RangedActivity("", "bogus")
	if err == nil {
		t.Fatal("RangedActivity(bogus) error = nil, want error")
	}
}

func TestTopDomainsWithSource(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	for _, mac := range []string{"aa:bb:cc:dd:ee:ff", "11:22:33:44:55:66", "22:33:44:55:66:77"} {
		if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", mac, err)
		}
	}

	trackingListID, err := db.AddList("https://example.com/tracking.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList(tracking): %v", err)
	}
	backupListID, err := db.AddList("https://example.com/tracking-2.txt", "Tracking Backup", "tracking")
	if err != nil {
		t.Fatalf("AddList(tracking backup): %v", err)
	}
	analyticsListID, err := db.AddList("https://example.com/analytics.txt", "Analytics List", "analytics")
	if err != nil {
		t.Fatalf("AddList(analytics): %v", err)
	}

	if err := db.WriteListDomains(trackingListID, map[string]string{
		"ads.example.com": "advertising",
		"dup.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains(tracking): %v", err)
	}
	if err := db.WriteListDomains(backupListID, map[string]string{
		"dup.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains(backup): %v", err)
	}
	if err := db.WriteListDomains(analyticsListID, map[string]string{
		"stats.example.com": "analytics",
	}); err != nil {
		t.Fatalf("WriteListDomains(analytics): %v", err)
	}

	if err := db.SetDomainOverride("manual.example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	err = db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "ads.example.com", QueryType: "A", Category: "advertising", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "ads.example.com", QueryType: "AAAA", Category: "advertising", Timestamp: now.Add(2 * time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "manual.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(3 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "stats.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(4 * time.Second)},
		{DeviceMAC: "22:33:44:55:66:77", Domain: "stats.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(5 * time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "dup.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(6 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", Domain: "unknown.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(7 * time.Second)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	domains, err := db.TopDomainsWithSource(10)
	if err != nil {
		t.Fatalf("TopDomainsWithSource: %v", err)
	}
	if len(domains) < 5 {
		t.Fatalf("got %d domains, want at least 5", len(domains))
	}
	if domains[0].Domain != "ads.example.com" {
		t.Fatalf("top domain = %q, want ads.example.com", domains[0].Domain)
	}
	if domains[0].QueryCount != 3 || domains[0].DeviceCount != 2 {
		t.Fatalf("ads.example.com counts = %#v, want QueryCount=3 DeviceCount=2", domains[0])
	}
	if domains[0].SourceList != "Tracking List" {
		t.Fatalf("ads.example.com source = %q, want Tracking List", domains[0].SourceList)
	}

	sources := map[string]string{}
	for _, domain := range domains {
		sources[domain.Domain] = domain.SourceList
	}
	if sources["manual.example.com"] != "manual" {
		t.Fatalf("manual source = %q, want manual", sources["manual.example.com"])
	}
	if sources["unknown.example.com"] != "unknown" {
		t.Fatalf("unknown source = %q, want unknown", sources["unknown.example.com"])
	}
	if sources["dup.example.com"] != "Tracking List" {
		t.Fatalf("dup source = %q, want Tracking List", sources["dup.example.com"])
	}
}

func TestDeviceTopDomainsWithSource(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}
	listID, err := db.AddList("https://example.com/trackers.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"ads.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}
	if err := db.SetDomainOverride("manual.example.com", "tracking"); err != nil {
		t.Fatalf("SetDomainOverride: %v", err)
	}

	err = db.WriteQueries([]Query{
		{DeviceMAC: mac, Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now},
		{DeviceMAC: mac, Domain: "ads.example.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(time.Second)},
		{DeviceMAC: mac, Domain: "manual.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(2 * time.Second)},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	domains, err := db.DeviceTopDomainsWithSource(mac, 10)
	if err != nil {
		t.Fatalf("DeviceTopDomainsWithSource: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
	if domains[0].Domain != "ads.example.com" || domains[0].QueryCount != 2 {
		t.Fatalf("top domain = %#v, want ads.example.com with count 2", domains[0])
	}
	if domains[0].SourceList != "Tracking List" {
		t.Fatalf("ads source = %q, want Tracking List", domains[0].SourceList)
	}
	if domains[0].DeviceCount != 1 {
		t.Fatalf("ads device count = %d, want 1", domains[0].DeviceCount)
	}
	if domains[1].SourceList != "manual" {
		t.Fatalf("manual source = %q, want manual", domains[1].SourceList)
	}
}

func TestDeviceAnomalies(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	if err := db.UpsertDevice(Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "tracker-box",
		Vendor:    "Vendor",
		FirstSeen: now.Add(-8 * 24 * time.Hour),
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice(tracker): %v", err)
	}
	if err := db.UpsertDevice(Device{
		MAC:       "11:22:33:44:55:66",
		IP:        "192.168.1.11",
		Hostname:  "volume-box",
		Vendor:    "Vendor",
		FirstSeen: now.Add(-8 * 24 * time.Hour),
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice(volume): %v", err)
	}

	listID, err := db.AddList("https://example.com/trackers.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"spike.example.com":  "tracking",
		"burst.example.com":  "tracking",
		"steady.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	var queries []Query
	for day := 2; day <= 8; day++ {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		queries = append(queries,
			Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "steady.example.com", QueryType: "A", Category: "", Timestamp: base},
			Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "steady.example.com", QueryType: "AAAA", Category: "", Timestamp: base.Add(time.Minute)},
			Query{DeviceMAC: "aa:bb:cc:dd:ee:ff", Domain: "steady.example.com", QueryType: "TXT", Category: "", Timestamp: base.Add(2 * time.Minute)},
			Query{DeviceMAC: "11:22:33:44:55:66", Domain: "normal.example.com", QueryType: "A", Category: "tracking", Timestamp: base},
		)
	}
	for i := 0; i < 6; i++ {
		queries = append(queries, Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "spike.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(-2*time.Hour + time.Duration(i)*time.Minute),
		})
	}
	for i := 0; i < 2; i++ {
		queries = append(queries, Query{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			Domain:    "clean.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-90*time.Minute + time.Duration(i)*time.Minute),
		})
	}
	for i := 0; i < 7; i++ {
		queries = append(queries, Query{
			DeviceMAC: "11:22:33:44:55:66",
			Domain:    "burst.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(-3*time.Hour + time.Duration(i)*time.Minute),
		})
	}
	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	anomalies, err := db.DeviceAnomalies()
	if err != nil {
		t.Fatalf("DeviceAnomalies: %v", err)
	}
	if len(anomalies) != 2 {
		t.Fatalf("got %d anomalies, want 2", len(anomalies))
	}

	byType := map[string]Anomaly{}
	for _, anomaly := range anomalies {
		byType[anomaly.Type] = anomaly
	}

	tracker := byType["tracker_spike"]
	if tracker.DeviceMAC != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("tracker anomaly device = %q, want aa:bb:cc:dd:ee:ff", tracker.DeviceMAC)
	}
	if tracker.TopDomain != "spike.example.com" {
		t.Fatalf("tracker top domain = %q, want spike.example.com", tracker.TopDomain)
	}
	if tracker.TopDomainCategory != "tracking" {
		t.Fatalf("tracker top domain category = %q, want tracking", tracker.TopDomainCategory)
	}
	if tracker.TopDomainSourceList != "Tracking List" {
		t.Fatalf("tracker top domain source = %q, want Tracking List", tracker.TopDomainSourceList)
	}
	if tracker.Delta <= 5 {
		t.Fatalf("tracker delta = %v, want > 5", tracker.Delta)
	}

	volume := byType["volume_spike"]
	if volume.DeviceMAC != "11:22:33:44:55:66" {
		t.Fatalf("volume anomaly device = %q, want 11:22:33:44:55:66", volume.DeviceMAC)
	}
	if volume.TopDomain != "burst.example.com" {
		t.Fatalf("volume top domain = %q, want burst.example.com", volume.TopDomain)
	}
	if volume.TopDomainSourceList != "Tracking List" {
		t.Fatalf("volume top domain source = %q, want Tracking List", volume.TopDomainSourceList)
	}
	if volume.Delta <= 0 {
		t.Fatalf("volume delta = %v, want > 0", volume.Delta)
	}
}
