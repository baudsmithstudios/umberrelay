package store

import (
	"context"
	"errors"
	"fmt"
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

func TestOpenAppliesPragmasOnAllConnections(t *testing.T) {
	db := testDB(t)
	ctx := context.Background()

	if stats := db.sql.Stats(); stats.MaxOpenConnections != 2 {
		t.Fatalf("MaxOpenConnections = %d, want 2", stats.MaxOpenConnections)
	}

	conn1, err := db.sql.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(1): %v", err)
	}
	defer conn1.Close()

	// Keep conn1 checked out so Conn(2) forces the pool to open another connection.
	var one int
	if err := conn1.QueryRowContext(ctx, `SELECT 1`).Scan(&one); err != nil {
		t.Fatalf("Conn(1) SELECT 1: %v", err)
	}

	conn2, err := db.sql.Conn(ctx)
	if err != nil {
		t.Fatalf("Conn(2): %v", err)
	}
	defer conn2.Close()

	var busyTimeout int
	if err := conn2.QueryRowContext(ctx, `PRAGMA busy_timeout`).Scan(&busyTimeout); err != nil {
		t.Fatalf("Conn(2) PRAGMA busy_timeout: %v", err)
	}
	if busyTimeout != 5000 {
		t.Fatalf("Conn(2) busy_timeout = %d, want 5000", busyTimeout)
	}

	var tempStore int
	if err := conn2.QueryRowContext(ctx, `PRAGMA temp_store`).Scan(&tempStore); err != nil {
		t.Fatalf("Conn(2) PRAGMA temp_store: %v", err)
	}
	if tempStore != 2 {
		t.Fatalf("Conn(2) temp_store = %d, want 2 (MEMORY)", tempStore)
	}

	var journalMode string
	if err := conn2.QueryRowContext(ctx, `PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("Conn(2) PRAGMA journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("Conn(2) journal_mode = %q, want wal", journalMode)
	}
}

func TestUpsertDeviceUpdatesIP(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.10", FirstSeen: now, LastSeen: now})
	db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:ff", IP: "192.168.1.20", LastSeen: now.Add(time.Minute)})
	devices, _ := db.ListDevicesWithStats()
	if devices[0].IP != "192.168.1.20" {
		t.Errorf("IP = %q, want 192.168.1.20", devices[0].IP)
	}
}

func TestUpsertDevicesBatchInsertsAndMergesPreservingLabel(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	if err := db.UpsertDevice(Device{MAC: "aa:bb:cc:dd:ee:01", IP: "192.168.1.11", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice(seed): %v", err)
	}
	if err := db.UpdateDeviceLabel("aa:bb:cc:dd:ee:01", "Kitchen Speaker"); err != nil {
		t.Fatalf("UpdateDeviceLabel: %v", err)
	}

	later := now.Add(time.Minute)
	batch := []Device{
		{MAC: "aa:bb:cc:dd:ee:01", Hostname: "kitchen-speaker", LastSeen: later},
		{MAC: "aa:bb:cc:dd:ee:02", IP: "192.168.1.12", FirstSeen: later, LastSeen: later},
	}
	if err := db.UpsertDevices(batch); err != nil {
		t.Fatalf("UpsertDevices: %v", err)
	}

	devices, err := db.ListDevicesWithStats()
	if err != nil {
		t.Fatalf("ListDevicesWithStats: %v", err)
	}
	if len(devices) != 2 {
		t.Fatalf("got %d devices, want 2", len(devices))
	}

	merged, err := db.GetDevice("aa:bb:cc:dd:ee:01")
	if err != nil {
		t.Fatalf("GetDevice: %v", err)
	}
	if merged.Label != "Kitchen Speaker" {
		t.Errorf("label = %q, want Kitchen Speaker (must not be overwritten)", merged.Label)
	}
	if merged.Hostname != "kitchen-speaker" {
		t.Errorf("hostname = %q, want kitchen-speaker (should be merged in)", merged.Hostname)
	}
	if merged.IP != "192.168.1.11" {
		t.Errorf("IP = %q, want 192.168.1.11 (empty incoming IP must not clobber)", merged.IP)
	}
}

func TestSetAndGetSourceLabel(t *testing.T) {
	db := testDB(t)

	if err := db.SetSourceLabel("10.44.0.7", "Living Room TV"); err != nil {
		t.Fatalf("SetSourceLabel(set): %v", err)
	}
	label, err := db.GetSourceLabel("10.44.0.7")
	if err != nil {
		t.Fatalf("GetSourceLabel(after set): %v", err)
	}
	if label != "Living Room TV" {
		t.Fatalf("label after set = %q, want %q", label, "Living Room TV")
	}

	if err := db.SetSourceLabel("10.44.0.7", "Kitchen Display"); err != nil {
		t.Fatalf("SetSourceLabel(update): %v", err)
	}
	label, err = db.GetSourceLabel("10.44.0.7")
	if err != nil {
		t.Fatalf("GetSourceLabel(after update): %v", err)
	}
	if label != "Kitchen Display" {
		t.Fatalf("label after update = %q, want %q", label, "Kitchen Display")
	}

	if err := db.SetSourceLabel("10.44.0.7", ""); err != nil {
		t.Fatalf("SetSourceLabel(clear): %v", err)
	}
	label, err = db.GetSourceLabel("10.44.0.7")
	if err != nil {
		t.Fatalf("GetSourceLabel(after clear): %v", err)
	}
	if label != "" {
		t.Fatalf("label after clear = %q, want empty", label)
	}

	label, err = db.GetSourceLabel("10.44.0.8")
	if err != nil {
		t.Fatalf("GetSourceLabel(nonexistent): %v", err)
	}
	if label != "" {
		t.Fatalf("label for nonexistent source = %q, want empty", label)
	}
}

func TestWriteAndQueryQueriesPreservesSourceIP(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	err := db.WriteQueries([]Query{{
		DeviceMAC: "",
		SourceIP:  "192.168.50.23",
		Domain:    "unmapped.example.com",
		QueryType: "A",
		Category:  "",
		Timestamp: now,
	}})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	results, err := db.QueryLog("", "", time.Time{}, now.Add(time.Minute), 100, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
	if results[0].SourceIP != "192.168.50.23" {
		t.Fatalf("SourceIP = %q, want %q", results[0].SourceIP, "192.168.50.23")
	}
}

func TestWriteQueriesUpdatesHourlyRollups(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC().Truncate(time.Hour)

	if err := db.WriteQueries([]Query{
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			SourceIP:  "192.168.1.10",
			Domain:    "ads.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(5 * time.Minute),
		},
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			SourceIP:  "192.168.1.10",
			Domain:    "api.example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(10 * time.Minute),
		},
		{
			DeviceMAC: "",
			SourceIP:  "10.44.0.7",
			Domain:    "unknown.example.com",
			QueryType: "A",
			Category:  "advertising",
			Timestamp: now.Add(15 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	var deviceTotal, deviceTracker int
	if err := db.sql.QueryRow(`
		SELECT total_count, tracker_count
		FROM query_rollups_hourly
		WHERE bucket_start = ? AND device_mac = ? AND source_ip = ''`,
		now.UnixNano(),
		"aa:bb:cc:dd:ee:ff",
	).Scan(&deviceTotal, &deviceTracker); err != nil {
		t.Fatalf("query device rollup: %v", err)
	}
	if deviceTotal != 2 || deviceTracker != 1 {
		t.Fatalf("device rollup total/tracker = %d/%d, want 2/1", deviceTotal, deviceTracker)
	}

	var sourceTotal, sourceTracker int
	if err := db.sql.QueryRow(`
		SELECT total_count, tracker_count
		FROM query_rollups_hourly
		WHERE bucket_start = ? AND device_mac = '' AND source_ip = ?`,
		now.UnixNano(),
		"10.44.0.7",
	).Scan(&sourceTotal, &sourceTracker); err != nil {
		t.Fatalf("query source rollup: %v", err)
	}
	if sourceTotal != 1 || sourceTracker != 1 {
		t.Fatalf("source rollup total/tracker = %d/%d, want 1/1", sourceTotal, sourceTracker)
	}
}

func TestOpenBackfillsHourlyRollupsWhenEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open(initial): %v", err)
	}

	now := time.Now().UTC().Truncate(time.Hour)
	if err := db.WriteQueries([]Query{
		{
			DeviceMAC: "aa:bb:cc:dd:ee:ff",
			SourceIP:  "192.168.1.10",
			Domain:    "ads.example.com",
			QueryType: "A",
			Category:  "tracking",
			Timestamp: now.Add(10 * time.Minute),
		},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
	if _, err := db.sql.Exec(`DELETE FROM query_rollups_hourly`); err != nil {
		t.Fatalf("DELETE rollups: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("Close(initial): %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open(reopen): %v", err)
	}
	t.Cleanup(func() { reopened.Close() })

	var count int
	if err := reopened.sql.QueryRow(`SELECT COUNT(*) FROM query_rollups_hourly`).Scan(&count); err != nil {
		t.Fatalf("count rollups: %v", err)
	}
	if count == 0 {
		t.Fatalf("rollups not backfilled on reopen")
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

	results, _ := db.QueryLog("aa:bb:cc:dd:ee:ff", "", time.Time{}, time.Now(), 100, 0)
	if len(results) != 1 {
		t.Errorf("device filter: got %d, want 1", len(results))
	}

	results, _ = db.QueryLog("", "ads.example.com", time.Time{}, time.Now(), 100, 0)
	if len(results) != 1 {
		t.Errorf("domain filter: got %d, want 1", len(results))
	}
}

func TestQueryFeedFiltersAndCursor(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	if err := db.UpsertDevice(Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice(device a): %v", err)
	}
	if err := db.UpsertDevice(Device{
		MAC:       "11:22:33:44:55:66",
		IP:        "192.168.1.11",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice(device b): %v", err)
	}

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-4 * time.Second)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "api.example.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(-3 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", SourceIP: "192.168.1.11", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Second)},
		{DeviceMAC: "", SourceIP: "10.44.0.7", Domain: "ads.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-1 * time.Second)},
		{DeviceMAC: "11:22:33:44:55:66", SourceIP: "192.168.1.11", Domain: "unknown.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-500 * time.Millisecond)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	results, err := db.QueryFeed(0, QueryFeedFilter{
		DeviceMAC: "aa:bb:cc:dd:ee:ff",
		Domain:    "ads.example.com",
		Category:  "tracking",
	}, 10)
	if err != nil {
		t.Fatalf("QueryFeed(device/domain/category): %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("device/domain/category filter returned %d rows, want 1", len(results))
	}
	if results[0].DeviceMAC != "aa:bb:cc:dd:ee:ff" || results[0].Domain != "ads.example.com" {
		t.Fatalf("unexpected row: %#v", results[0])
	}

	sourceResults, err := db.QueryFeed(0, QueryFeedFilter{
		SourceIP: "10.44.0.7",
	}, 10)
	if err != nil {
		t.Fatalf("QueryFeed(source): %v", err)
	}
	if len(sourceResults) != 1 {
		t.Fatalf("source filter returned %d rows, want 1", len(sourceResults))
	}
	if sourceResults[0].SourceIP != "10.44.0.7" || sourceResults[0].DeviceMAC != "" {
		t.Fatalf("unexpected source row: %#v", sourceResults[0])
	}

	afterID := results[0].ID
	cursorResults, err := db.QueryFeed(afterID, QueryFeedFilter{
		DeviceMAC: "aa:bb:cc:dd:ee:ff",
	}, 10)
	if err != nil {
		t.Fatalf("QueryFeed(afterID): %v", err)
	}
	if len(cursorResults) != 1 {
		t.Fatalf("afterID filter returned %d rows, want 1", len(cursorResults))
	}
	if cursorResults[0].Domain != "api.example.com" {
		t.Fatalf("afterID row domain = %q, want %q", cursorResults[0].Domain, "api.example.com")
	}
	if cursorResults[0].ID <= afterID {
		t.Fatalf("afterID row ID = %d, want > %d", cursorResults[0].ID, afterID)
	}

	unclassifiedResults, err := db.QueryFeed(0, QueryFeedFilter{
		Category: "uncategorized",
	}, 10)
	if err != nil {
		t.Fatalf("QueryFeed(uncategorized): %v", err)
	}
	if len(unclassifiedResults) != 2 {
		t.Fatalf("uncategorized filter returned %d rows, want 2", len(unclassifiedResults))
	}
	for _, row := range unclassifiedResults {
		if row.Category != "" && row.Category != "uncategorized" {
			t.Fatalf("unexpected category %q in uncategorized filter", row.Category)
		}
	}
}

func TestPurgeQueriesOlderThanChunk(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"
	if err := db.UpsertDevice(Device{
		MAC:       mac,
		IP:        "192.168.1.10",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	old := now.Add(-48 * time.Hour)
	var queries []Query
	for i := 0; i < 5; i++ {
		queries = append(queries, Query{
			DeviceMAC: mac,
			Domain:    fmt.Sprintf("old-%d.example.com", i),
			QueryType: "A",
			Timestamp: old.Add(time.Duration(i) * time.Second),
		})
	}
	for i := 0; i < 2; i++ {
		queries = append(queries, Query{
			DeviceMAC: mac,
			Domain:    fmt.Sprintf("new-%d.example.com", i),
			QueryType: "A",
			Timestamp: now.Add(time.Duration(i) * time.Second),
		})
	}
	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	cutoff := now.Add(-24 * time.Hour)
	deleted, err := db.PurgeQueriesOlderThanChunk(cutoff, 2)
	if err != nil {
		t.Fatalf("PurgeQueriesOlderThanChunk(first): %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted first chunk = %d, want 2", deleted)
	}

	deleted, err = db.PurgeQueriesOlderThanChunk(cutoff, 2)
	if err != nil {
		t.Fatalf("PurgeQueriesOlderThanChunk(second): %v", err)
	}
	if deleted != 2 {
		t.Fatalf("deleted second chunk = %d, want 2", deleted)
	}

	deleted, err = db.PurgeQueriesOlderThanChunk(cutoff, 2)
	if err != nil {
		t.Fatalf("PurgeQueriesOlderThanChunk(third): %v", err)
	}
	if deleted != 1 {
		t.Fatalf("deleted third chunk = %d, want 1", deleted)
	}

	deleted, err = db.PurgeQueriesOlderThanChunk(cutoff, 2)
	if err != nil {
		t.Fatalf("PurgeQueriesOlderThanChunk(final): %v", err)
	}
	if deleted != 0 {
		t.Fatalf("deleted final chunk = %d, want 0", deleted)
	}

	remaining, err := db.QueryLog("", "", time.Time{}, now.Add(24*time.Hour), 100, 0)
	if err != nil {
		t.Fatalf("QueryLog: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("remaining rows = %d, want 2", len(remaining))
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

func TestListRefreshStatusTracksAttemptSuccessAndFailure(t *testing.T) {
	db := testDB(t)

	initial, err := db.GetListRefreshStatus()
	if err != nil {
		t.Fatalf("GetListRefreshStatus(initial): %v", err)
	}
	if !initial.LastAttemptAt.IsZero() || !initial.LastSuccessAt.IsZero() || initial.LastError != "" {
		t.Fatalf("initial status = %#v, want zero values", initial)
	}

	firstAttempt := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	if err := db.RecordListRefreshAttempt(firstAttempt, errors.New("refresh failed")); err != nil {
		t.Fatalf("RecordListRefreshAttempt(failure): %v", err)
	}

	afterFailure, err := db.GetListRefreshStatus()
	if err != nil {
		t.Fatalf("GetListRefreshStatus(after failure): %v", err)
	}
	if !afterFailure.LastAttemptAt.Equal(firstAttempt) {
		t.Fatalf("LastAttemptAt = %v, want %v", afterFailure.LastAttemptAt, firstAttempt)
	}
	if !afterFailure.LastSuccessAt.IsZero() {
		t.Fatalf("LastSuccessAt = %v, want zero", afterFailure.LastSuccessAt)
	}
	if afterFailure.LastError == "" {
		t.Fatal("LastError = empty, want refresh error")
	}

	secondAttempt := firstAttempt.Add(2 * time.Hour)
	if err := db.RecordListRefreshAttempt(secondAttempt, nil); err != nil {
		t.Fatalf("RecordListRefreshAttempt(success): %v", err)
	}

	afterSuccess, err := db.GetListRefreshStatus()
	if err != nil {
		t.Fatalf("GetListRefreshStatus(after success): %v", err)
	}
	if !afterSuccess.LastAttemptAt.Equal(secondAttempt) {
		t.Fatalf("LastAttemptAt = %v, want %v", afterSuccess.LastAttemptAt, secondAttempt)
	}
	if !afterSuccess.LastSuccessAt.Equal(secondAttempt) {
		t.Fatalf("LastSuccessAt = %v, want %v", afterSuccess.LastSuccessAt, secondAttempt)
	}
	if afterSuccess.LastError != "" {
		t.Fatalf("LastError = %q, want empty", afterSuccess.LastError)
	}
}

func TestListDomainCache(t *testing.T) {
	db := testDB(t)
	id, err := db.AddList("https://example.com/list.txt", "Test List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}

	domains := map[string]string{
		"ads.example.com":     "tracking",
		"tracker.example.com": "tracking",
	}
	err = db.WriteListDomains(id, domains)
	if err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

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

func TestDashboardSummaryCountsUnattributedSourceIPsSeparately(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	if err := db.UpsertDevice(Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		Hostname:  "known-device",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "known.example.com", QueryType: "A", Timestamp: now},
		{DeviceMAC: "", SourceIP: "10.10.20.30", Domain: "unknown-a.example.com", QueryType: "A", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "", SourceIP: "10.10.20.31", Domain: "unknown-b.example.com", QueryType: "A", Timestamp: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	summary, err := db.DashboardSummaryAt(now.Add(5 * time.Second))
	if err != nil {
		t.Fatalf("DashboardSummaryAt: %v", err)
	}
	if summary.DeviceCount != 3 {
		t.Fatalf("DeviceCount = %d, want 3", summary.DeviceCount)
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
	// Ordered by query count desc
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

	queryTrend, trackerTrend, err := db.LoadTrendsAt(now, "")
	if err != nil {
		t.Fatalf("LoadTrendsAt: %v", err)
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

	queryTrend, trackerTrend, err := db.LoadTrendsAt(now, "")
	if err != nil {
		t.Fatalf("LoadTrendsAt: %v", err)
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

	queryTrend, trackerTrend, err := db.LoadTrendsAt(now, "")
	if err != nil {
		t.Fatalf("LoadTrendsAt: %v", err)
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

	queryTrend, trackerTrend, err := db.LoadTrendsAt(time.Now().UTC(), "")
	if err != nil {
		t.Fatalf("LoadTrendsAt: %v", err)
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

	queryTrend, trackerTrend, err := db.LoadTrendsAt(now, "device_mac = ?", "aa:bb:cc:dd:ee:ff")
	if err != nil {
		t.Fatalf("LoadTrendsAt: %v", err)
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

	results, err := db.ListDevicesWithTrendsAt(now)
	if err != nil {
		t.Fatalf("ListDevicesWithTrendsAt: %v", err)
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

	summary, err := db.DevicePrivacySummaryAt(mac, now)
	if err != nil {
		t.Fatalf("DevicePrivacySummaryAt: %v", err)
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

	summary, err := db.DevicePrivacySummaryAt(mac, now)
	if err != nil {
		t.Fatalf("DevicePrivacySummaryAt: %v", err)
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
			query:    "EXPLAIN QUERY PLAN SELECT bucket_start, SUM(total_count), SUM(tracker_count) FROM query_rollups_hourly WHERE bucket_start >= ? GROUP BY bucket_start ORDER BY bucket_start",
			args:     []any{time.Now().Add(-24 * time.Hour).UnixNano()},
			wantPlan: "idx_rollups_bucket",
		},
		{
			name:     "device activity",
			query:    "EXPLAIN QUERY PLAN SELECT bucket_start, SUM(total_count), SUM(tracker_count) FROM query_rollups_hourly WHERE bucket_start >= ? AND device_mac = ? GROUP BY bucket_start ORDER BY bucket_start",
			args:     []any{time.Now().Add(-24 * time.Hour).UnixNano(), "aa:bb:cc:dd:ee:ff"},
			wantPlan: "idx_rollups_device_bucket",
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
	if !errors.Is(err, ErrInvalidRange) {
		t.Fatalf("errors.Is(err, ErrInvalidRange) = false, err = %v", err)
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

func TestTopDomainsWithSourceCountsUnattributedSourcesSeparately(t *testing.T) {
	db := testDB(t)
	now := time.Now()

	if err := db.UpsertDevice(Device{
		MAC:       "aa:bb:cc:dd:ee:ff",
		IP:        "192.168.1.10",
		FirstSeen: now,
		LastSeen:  now,
	}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: "192.168.1.10", Domain: "shared.example.com", QueryType: "A", Timestamp: now},
		{DeviceMAC: "", SourceIP: "10.0.0.7", Domain: "shared.example.com", QueryType: "A", Timestamp: now.Add(time.Second)},
		{DeviceMAC: "", SourceIP: "10.0.0.8", Domain: "shared.example.com", QueryType: "A", Timestamp: now.Add(2 * time.Second)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	domains, err := db.TopDomainsWithSource(5)
	if err != nil {
		t.Fatalf("TopDomainsWithSource: %v", err)
	}
	if len(domains) == 0 {
		t.Fatalf("got no domains, want one")
	}
	if domains[0].Domain != "shared.example.com" {
		t.Fatalf("top domain = %q, want shared.example.com", domains[0].Domain)
	}
	if domains[0].DeviceCount != 3 {
		t.Fatalf("shared.example.com device_count = %d, want 3", domains[0].DeviceCount)
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

	domains, err := db.DeviceTopDomainsWithSourcePage(mac, 10, 0)
	if err != nil {
		t.Fatalf("DeviceTopDomainsWithSourcePage: %v", err)
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

func TestDeviceTopDomainsWithSourcePage(t *testing.T) {
	db := testDB(t)
	now := time.Now()
	mac := "aa:bb:cc:dd:ee:ff"

	if err := db.UpsertDevice(Device{MAC: mac, IP: "192.168.1.10", FirstSeen: now, LastSeen: now}); err != nil {
		t.Fatalf("UpsertDevice: %v", err)
	}

	queries := []Query{
		{DeviceMAC: mac, Domain: "a.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: mac, Domain: "b.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: mac, Domain: "c.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: mac, Domain: "d.example.com", QueryType: "A", Category: "", Timestamp: now},
		{DeviceMAC: mac, Domain: "e.example.com", QueryType: "A", Category: "", Timestamp: now},
	}
	if err := db.WriteQueries(queries); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	domains, err := db.DeviceTopDomainsWithSourcePage(mac, 2, 2)
	if err != nil {
		t.Fatalf("DeviceTopDomainsWithSourcePage: %v", err)
	}
	if len(domains) != 2 {
		t.Fatalf("got %d domains, want 2", len(domains))
	}
	if domains[0].Domain != "c.example.com" {
		t.Fatalf("first domain = %q, want c.example.com", domains[0].Domain)
	}
	if domains[1].Domain != "d.example.com" {
		t.Fatalf("second domain = %q, want d.example.com", domains[1].Domain)
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

func TestDeviceBypassSignals(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()

	likelyMAC := "aa:bb:cc:dd:ee:01"
	suspectedMAC := "aa:bb:cc:dd:ee:02"
	activeMAC := "aa:bb:cc:dd:ee:03"

	for _, dev := range []Device{
		{
			MAC:       likelyMAC,
			Hostname:  "living-room-tv",
			Vendor:    "TV Vendor",
			FirstSeen: now.Add(-5 * 24 * time.Hour),
			LastSeen:  now.Add(-5 * time.Minute),
		},
		{
			MAC:       suspectedMAC,
			Hostname:  "kitchen-speaker",
			Vendor:    "Speaker Vendor",
			FirstSeen: now.Add(-5 * 24 * time.Hour),
			LastSeen:  now.Add(-6 * time.Minute),
		},
		{
			MAC:       activeMAC,
			Hostname:  "office-laptop",
			Vendor:    "Laptop Vendor",
			FirstSeen: now.Add(-5 * 24 * time.Hour),
			LastSeen:  now.Add(-4 * time.Minute),
		},
	} {
		if err := db.UpsertDevice(dev); err != nil {
			t.Fatalf("UpsertDevice(%s): %v", dev.MAC, err)
		}
	}

	err := db.WriteQueries([]Query{
		{
			DeviceMAC: likelyMAC,
			Domain:    "dns.google.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-3 * 24 * time.Hour),
		},
		{
			DeviceMAC: likelyMAC,
			Domain:    "example.com.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-2 * time.Hour),
		},
		{
			DeviceMAC: suspectedMAC,
			Domain:    "example.org.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-2 * time.Hour),
		},
		{
			DeviceMAC: activeMAC,
			Domain:    "api.example.net.",
			QueryType: "A",
			Category:  "",
			Timestamp: now.Add(-15 * time.Minute),
		},
	})
	if err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}

	signals, err := db.DeviceBypassSignalsAt(now)
	if err != nil {
		t.Fatalf("DeviceBypassSignalsAt: %v", err)
	}

	if len(signals) != 2 {
		t.Fatalf("got %d bypass signals, want 2", len(signals))
	}

	byMAC := make(map[string]BypassSignal, len(signals))
	for _, signal := range signals {
		byMAC[signal.DeviceMAC] = signal
	}

	likely, ok := byMAC[likelyMAC]
	if !ok {
		t.Fatalf("missing likely signal for %s", likelyMAC)
	}
	if likely.Confidence != "likely" {
		t.Fatalf("likely confidence = %q, want %q", likely.Confidence, "likely")
	}
	if likely.HintDomain != "dns.google" {
		t.Fatalf("likely hint domain = %q, want %q", likely.HintDomain, "dns.google")
	}

	suspected, ok := byMAC[suspectedMAC]
	if !ok {
		t.Fatalf("missing suspected signal for %s", suspectedMAC)
	}
	if suspected.Confidence != "suspected" {
		t.Fatalf("suspected confidence = %q, want %q", suspected.Confidence, "suspected")
	}
	if suspected.HintDomain != "" {
		t.Fatalf("suspected hint domain = %q, want empty", suspected.HintDomain)
	}

	if _, ok := byMAC[activeMAC]; ok {
		t.Fatalf("active device %s should not be flagged", activeMAC)
	}
}

func TestSourceActorQueriesAndAggregations(t *testing.T) {
	db := testDB(t)
	now := time.Now().UTC()
	sourceA := "10.55.0.17"
	sourceB := "10.55.0.18"

	listID, err := db.AddList("https://example.com/tracking.txt", "Tracking List", "tracking")
	if err != nil {
		t.Fatalf("AddList: %v", err)
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"tracker.example.com": "tracking",
	}); err != nil {
		t.Fatalf("WriteListDomains: %v", err)
	}

	if err := db.WriteQueries([]Query{
		{DeviceMAC: "", SourceIP: sourceA, Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-48 * time.Hour)},
		{DeviceMAC: "", SourceIP: sourceA, Domain: "clean.example.com", QueryType: "A", Category: "", Timestamp: now.Add(-47 * time.Hour)},
		{DeviceMAC: "", SourceIP: sourceA, Domain: "tracker.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "", SourceIP: sourceA, Domain: "analytics.example.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-90 * time.Minute)},
		{DeviceMAC: "", SourceIP: sourceA, Domain: "clean.example.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(-80 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:ff", SourceIP: sourceA, Domain: "device-owned.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-70 * time.Minute)},
		{DeviceMAC: "", SourceIP: sourceB, Domain: "other.example.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-60 * time.Minute)},
	}); err != nil {
		t.Fatalf("WriteQueries: %v", err)
	}
	if err := db.SetSourceLabel(sourceA, "Kitchen Display"); err != nil {
		t.Fatalf("SetSourceLabel: %v", err)
	}

	logRows, err := db.QueryLogBySource(sourceA, "", time.Time{}, now.Add(time.Hour), 100, 0)
	if err != nil {
		t.Fatalf("QueryLogBySource: %v", err)
	}
	if len(logRows) != 5 {
		t.Fatalf("len(QueryLogBySource) = %d, want 5", len(logRows))
	}
	for _, row := range logRows {
		if row.DeviceMAC != "" || row.SourceIP != sourceA {
			t.Fatalf("unexpected row in source query log: %+v", row)
		}
	}

	hourly, err := db.SourceHourlyActivity(sourceA)
	if err != nil {
		t.Fatalf("SourceHourlyActivity: %v", err)
	}
	if len(hourly) != 24 {
		t.Fatalf("len(SourceHourlyActivity) = %d, want 24", len(hourly))
	}
	totalHourly := 0
	for _, bucket := range hourly {
		totalHourly += bucket.TotalCount
	}
	if totalHourly != 3 {
		t.Fatalf("hourly total = %d, want 3", totalHourly)
	}

	summary, err := db.SourcePrivacySummaryAt(sourceA, now)
	if err != nil {
		t.Fatalf("SourcePrivacySummaryAt: %v", err)
	}
	if summary.QueryCount != 3 {
		t.Fatalf("summary.QueryCount = %d, want 3", summary.QueryCount)
	}
	if summary.UniqueDomains != 3 {
		t.Fatalf("summary.UniqueDomains = %d, want 3", summary.UniqueDomains)
	}

	breakdown, err := db.SourceCategoryBreakdown(sourceA)
	if err != nil {
		t.Fatalf("SourceCategoryBreakdown: %v", err)
	}
	if len(breakdown) == 0 {
		t.Fatal("SourceCategoryBreakdown returned no rows")
	}

	ranged, err := db.SourceRangedActivity(sourceA, "7d")
	if err != nil {
		t.Fatalf("SourceRangedActivity(7d): %v", err)
	}
	if len(ranged) != 7 {
		t.Fatalf("len(SourceRangedActivity) = %d, want 7", len(ranged))
	}

	top, err := db.SourceTopDomainsWithSourcePage(sourceA, 10, 0)
	if err != nil {
		t.Fatalf("SourceTopDomainsWithSourcePage: %v", err)
	}
	if len(top) == 0 {
		t.Fatal("SourceTopDomainsWithSourcePage returned no rows")
	}
	foundTracker := false
	for _, domain := range top {
		if domain.Domain == "tracker.example.com" {
			foundTracker = true
			if domain.SourceList != "Tracking List" {
				t.Fatalf("tracker domain source_list = %q, want Tracking List", domain.SourceList)
			}
		}
	}
	if !foundTracker {
		t.Fatalf("tracker.example.com not found in SourceTopDomainsWithSourcePage: %+v", top)
	}

	trends, err := db.ListSourceWithTrendsAt(now)
	if err != nil {
		t.Fatalf("ListSourceWithTrendsAt: %v", err)
	}
	if len(trends) == 0 {
		t.Fatal("ListSourceWithTrendsAt returned no rows")
	}
	found := false
	for _, trend := range trends {
		if trend.SourceIP == sourceA {
			found = true
			if trend.QueryCount != 3 {
				t.Fatalf("QueryCount = %d, want 3", trend.QueryCount)
			}
			if trend.Label != "Kitchen Display" {
				t.Fatalf("Label = %q, want %q", trend.Label, "Kitchen Display")
			}
		}
	}
	if !found {
		t.Fatalf("source trend for %s not found", sourceA)
	}
}
