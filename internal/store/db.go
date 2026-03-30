package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const trackingCategorySQL = "('tracking', 'advertising', 'malware')"

// Device represents a discovered network device.
type Device struct {
	MAC       string
	IP        string
	Hostname  string
	Vendor    string
	Label     string
	FirstSeen time.Time
	LastSeen  time.Time
}

// Query represents a logged DNS query.
type Query struct {
	ID        int64
	DeviceMAC string
	Domain    string
	QueryType string
	Category  string
	Timestamp time.Time
}

// HourlyBucket holds aggregate activity for a calendar-hour bucket.
type HourlyBucket struct {
	Timestamp    time.Time
	TotalCount   int
	TrackerCount int
}

// DomainWithSource holds aggregate domain data with source attribution.
type DomainWithSource struct {
	Domain      string `json:"domain"`
	Category    string `json:"category"`
	QueryCount  int    `json:"query_count"`
	DeviceCount int    `json:"device_count"`
	SourceList  string `json:"source_list"`
}

// Anomaly holds a per-device anomaly for the privacy attention feed.
type Anomaly struct {
	DeviceMAC           string
	DeviceName          string
	Type                string
	CurrentValue        float64
	AverageValue        float64
	Delta               float64
	TopDomain           string
	TopDomainCategory   string
	TopDomainSourceList string
}

// Trend holds current and prior-period comparison data.
type Trend struct {
	Current  float64
	Previous float64
	Change   float64
	HasPrior bool
}

// DB wraps the SQLite connection.
type DB struct {
	sql *sql.DB
}

// ErrNotFound indicates the requested row does not exist.
var ErrNotFound = errors.New("not found")

// Open opens (or creates) the SQLite database and applies the schema.
func Open(path string) (*DB, error) {
	conn, err := sql.Open("sqlite", path+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if _, err := conn.Exec(schema); err != nil {
		conn.Close()
		return nil, fmt.Errorf("apply schema: %w", err)
	}
	return &DB{sql: conn}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error {
	return d.sql.Close()
}

// UpsertDevice inserts or updates a device record.
// On conflict, updates IP, hostname, vendor, and last_seen. Never overwrites label.
func (d *DB) UpsertDevice(dev Device) error {
	_, err := d.sql.Exec(`
        INSERT INTO devices (mac, ip, hostname, vendor, label, first_seen, last_seen)
        VALUES (?, ?, ?, ?, '', ?, ?)
        ON CONFLICT(mac) DO UPDATE SET
            ip        = COALESCE(NULLIF(excluded.ip, ''), devices.ip),
            hostname  = COALESCE(NULLIF(excluded.hostname, ''), devices.hostname),
            vendor    = COALESCE(NULLIF(excluded.vendor, ''), devices.vendor),
            last_seen = excluded.last_seen`,
		dev.MAC, dev.IP, dev.Hostname, dev.Vendor,
		dev.FirstSeen.UnixNano(), dev.LastSeen.UnixNano(),
	)
	return err
}

// UpdateDeviceLabel sets the user-assigned label for a device.
func (d *DB) UpdateDeviceLabel(mac, label string) error {
	result, err := d.sql.Exec(`UPDATE devices SET label = ? WHERE mac = ?`, label, mac)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ListDevices returns all known devices.
func (d *DB) ListDevices() ([]Device, error) {
	rows, err := d.sql.Query(`SELECT mac, ip, hostname, vendor, label, first_seen, last_seen FROM devices ORDER BY last_seen DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Device
	for rows.Next() {
		var dev Device
		var firstSeen, lastSeen int64
		var label sql.NullString
		if err := rows.Scan(&dev.MAC, &dev.IP, &dev.Hostname, &dev.Vendor, &label, &firstSeen, &lastSeen); err != nil {
			return nil, err
		}
		dev.Label = label.String
		dev.FirstSeen = time.Unix(0, firstSeen)
		dev.LastSeen = time.Unix(0, lastSeen)
		out = append(out, dev)
	}
	return out, rows.Err()
}

// GetDevice returns a single device by MAC.
func (d *DB) GetDevice(mac string) (Device, error) {
	var dev Device
	var firstSeen, lastSeen int64
	var label sql.NullString
	err := d.sql.QueryRow(
		`SELECT mac, ip, hostname, vendor, label, first_seen, last_seen FROM devices WHERE mac = ?`, mac,
	).Scan(&dev.MAC, &dev.IP, &dev.Hostname, &dev.Vendor, &label, &firstSeen, &lastSeen)
	if err != nil {
		return dev, err
	}
	dev.Label = label.String
	dev.FirstSeen = time.Unix(0, firstSeen)
	dev.LastSeen = time.Unix(0, lastSeen)
	return dev, nil
}

// WriteQueries inserts a batch of DNS query records.
func (d *DB) WriteQueries(queries []Query) error {
	if len(queries) == 0 {
		return nil
	}
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO queries (device_mac, domain, query_type, category, timestamp) VALUES (?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, q := range queries {
		if _, err := stmt.Exec(q.DeviceMAC, q.Domain, q.QueryType, q.Category, q.Timestamp.UnixNano()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// QueryLog returns queries matching the given filters, newest first.
func (d *DB) QueryLog(deviceMAC, domain string, from, to time.Time, limit, offset int) ([]Query, error) {
	var conditions []string
	var args []any

	if deviceMAC != "" {
		conditions = append(conditions, "device_mac = ?")
		args = append(args, deviceMAC)
	}
	if domain != "" {
		conditions = append(conditions, "domain = ?")
		args = append(args, domain)
	}
	if !from.IsZero() {
		conditions = append(conditions, "timestamp >= ?")
		args = append(args, from.UnixNano())
	}
	if !to.IsZero() {
		conditions = append(conditions, "timestamp <= ?")
		args = append(args, to.UnixNano())
	}

	query := `SELECT id, device_mac, domain, query_type, category, timestamp FROM queries`
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY timestamp DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Query
	for rows.Next() {
		var q Query
		var ts int64
		if err := rows.Scan(&q.ID, &q.DeviceMAC, &q.Domain, &q.QueryType, &q.Category, &ts); err != nil {
			return nil, err
		}
		q.Timestamp = time.Unix(0, ts)
		out = append(out, q)
	}
	return out, rows.Err()
}

// HourlyActivity returns 24 UTC calendar-hour buckets, oldest first.
func (d *DB) HourlyActivity(mac string) ([]HourlyBucket, error) {
	const bucketCount = 24

	currentHour := time.Now().UTC().Truncate(time.Hour)
	oldestHour := currentHour.Add(-(bucketCount - 1) * time.Hour)
	hourNS := int64(time.Hour)

	query := `
		SELECT timestamp / ? AS hour_key,
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN category IN ` + trackingCategorySQL + ` THEN 1 ELSE 0 END), 0)
		FROM queries
		WHERE timestamp >= ?`
	args := []any{hourNS, oldestHour.UnixNano()}
	if mac != "" {
		query += ` AND device_mac = ?`
		args = append(args, mac)
	}
	query += ` GROUP BY hour_key ORDER BY hour_key`

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	countsByHour := make(map[int64]HourlyBucket, bucketCount)
	for rows.Next() {
		var hourKey int64
		var bucket HourlyBucket
		if err := rows.Scan(&hourKey, &bucket.TotalCount, &bucket.TrackerCount); err != nil {
			return nil, err
		}
		countsByHour[hourKey] = bucket
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	buckets := make([]HourlyBucket, 0, bucketCount)
	for i := 0; i < bucketCount; i++ {
		ts := oldestHour.Add(time.Duration(i) * time.Hour)
		hourKey := ts.UnixNano() / hourNS
		bucket := countsByHour[hourKey]
		bucket.Timestamp = ts
		buckets = append(buckets, bucket)
	}

	return buckets, nil
}

// PurgeQueriesOlderThan deletes query records older than cutoff.
func (d *DB) PurgeQueriesOlderThan(cutoff time.Time) error {
	_, err := d.sql.Exec(`DELETE FROM queries WHERE timestamp < ?`, cutoff.UnixNano())
	return err
}

// SetConfig inserts or updates a config key-value pair.
func (d *DB) SetConfig(key, value string) error {
	_, err := d.sql.Exec(
		`INSERT INTO config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetConfig returns the value for a config key, or empty string if not set.
func (d *DB) GetConfig(key string) (string, error) {
	var value string
	err := d.sql.QueryRow(`SELECT value FROM config WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// AddList inserts a classification list and returns its ID.
func (d *DB) AddList(url, name, category string) (int64, error) {
	res, err := d.sql.Exec(
		`INSERT INTO lists (url, name, category) VALUES (?, ?, ?)`,
		url, name, category,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// ListEntry represents a classification list row.
type ListEntry struct {
	ID        int64
	URL       string
	Name      string
	Category  string
	LastFetch *time.Time
	Enabled   bool
}

// ListLists returns all classification lists.
func (d *DB) ListLists() ([]ListEntry, error) {
	rows, err := d.sql.Query(`SELECT id, url, name, category, last_fetch, enabled FROM lists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []ListEntry
	for rows.Next() {
		var l ListEntry
		var lastFetch sql.NullInt64
		if err := rows.Scan(&l.ID, &l.URL, &l.Name, &l.Category, &lastFetch, &l.Enabled); err != nil {
			return nil, err
		}
		if lastFetch.Valid {
			t := time.Unix(0, lastFetch.Int64)
			l.LastFetch = &t
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// DeleteList removes a classification list and its cached domains.
func (d *DB) DeleteList(id int64) error {
	result, err := d.sql.Exec(`DELETE FROM lists WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateListEnabled toggles whether a classification list participates in lookups.
func (d *DB) UpdateListEnabled(id int64, enabled bool) error {
	result, err := d.sql.Exec(`UPDATE lists SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateListFetchTime marks a list as recently fetched.
func (d *DB) UpdateListFetchTime(id int64) error {
	_, err := d.sql.Exec(`UPDATE lists SET last_fetch = ? WHERE id = ?`, time.Now().UnixNano(), id)
	return err
}

// WriteListDomains replaces the cached domains for a list.
func (d *DB) WriteListDomains(listID int64, domains map[string]string) error {
	tx, err := d.sql.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`DELETE FROM list_domains WHERE list_id = ?`, listID)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`INSERT INTO list_domains (list_id, domain, category) VALUES (?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for domain, category := range domains {
		if _, err := stmt.Exec(listID, domain, category); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadCachedDomains returns all cached domains from enabled lists.
func (d *DB) LoadCachedDomains() (map[string]string, error) {
	rows, err := d.sql.Query(`
        SELECT ld.domain, ld.category
        FROM list_domains ld
        JOIN lists l ON l.id = ld.list_id
        WHERE l.enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var domain, category string
		if err := rows.Scan(&domain, &category); err != nil {
			return nil, err
		}
		out[domain] = category
	}
	return out, rows.Err()
}

// SetDomainOverride persists a domain classification override.
func (d *DB) SetDomainOverride(domain, category string) error {
	_, err := d.sql.Exec(
		`INSERT INTO domain_overrides (domain, category, created_at) VALUES (?, ?, ?)
         ON CONFLICT(domain) DO UPDATE SET category = excluded.category`,
		domain, category, time.Now().UnixNano(),
	)
	return err
}

// DeleteDomainOverride removes a persisted domain override.
func (d *DB) DeleteDomainOverride(domain string) error {
	_, err := d.sql.Exec(`DELETE FROM domain_overrides WHERE domain = ?`, domain)
	return err
}

// ListDomainOverrides returns all persisted domain overrides.
func (d *DB) ListDomainOverrides() (map[string]string, error) {
	rows, err := d.sql.Query(`SELECT domain, category FROM domain_overrides`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]string)
	for rows.Next() {
		var domain, category string
		if err := rows.Scan(&domain, &category); err != nil {
			return nil, err
		}
		out[domain] = category
	}
	return out, rows.Err()
}

// DashboardStats holds summary data for the dashboard page.
type DashboardStats struct {
	TotalQueries      int
	TrackerPercent    float64
	DeviceCount       int
	UniqueDomainCount int
	TopDevices        []DeviceSummary
}

// DeviceSummary holds per-device stats for the dashboard.
type DeviceSummary struct {
	MAC            string
	Hostname       string
	Vendor         string
	Label          string
	QueryCount     int
	TrackerPercent float64
}

// DashboardSummary returns aggregate stats for the dashboard (last 24 hours).
func (d *DB) DashboardSummary() (DashboardStats, error) {
	return d.DashboardSummaryAt(time.Now())
}

func (d *DB) DashboardSummaryAt(now time.Time) (DashboardStats, error) {
	cutoff := now.Add(-24 * time.Hour).UnixNano()
	var stats DashboardStats

	var trackerCount int
	err := d.sql.QueryRow(`
        SELECT COUNT(*),
               COALESCE(SUM(CASE WHEN category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0),
               COUNT(DISTINCT domain)
        FROM queries WHERE timestamp >= ?`, cutoff,
	).Scan(&stats.TotalQueries, &trackerCount, &stats.UniqueDomainCount)
	if err != nil {
		return stats, err
	}
	if stats.TotalQueries > 0 {
		stats.TrackerPercent = float64(trackerCount) / float64(stats.TotalQueries) * 100
	}

	err = d.sql.QueryRow(`
        SELECT COUNT(DISTINCT device_mac) FROM queries WHERE timestamp >= ?`, cutoff,
	).Scan(&stats.DeviceCount)
	if err != nil {
		return stats, err
	}

	rows, err := d.sql.Query(`
        SELECT q.device_mac,
               COALESCE(dev.hostname, ''),
               COALESCE(dev.vendor, ''),
               COALESCE(dev.label, ''),
               COUNT(*) as cnt,
               COALESCE(SUM(CASE WHEN q.category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0) as tracker_cnt
        FROM queries q
        LEFT JOIN devices dev ON dev.mac = q.device_mac
        WHERE q.timestamp >= ?
        GROUP BY q.device_mac
        ORDER BY cnt DESC
        LIMIT 5`, cutoff)
	if err != nil {
		return stats, err
	}
	defer rows.Close()

	for rows.Next() {
		var ds DeviceSummary
		var trackerCnt int
		if err := rows.Scan(&ds.MAC, &ds.Hostname, &ds.Vendor, &ds.Label, &ds.QueryCount, &trackerCnt); err != nil {
			return stats, err
		}
		if ds.QueryCount > 0 {
			ds.TrackerPercent = float64(trackerCnt) / float64(ds.QueryCount) * 100
		}
		stats.TopDevices = append(stats.TopDevices, ds)
	}
	return stats, rows.Err()
}

// DomainSummary holds aggregate data for the domains page.
type DomainSummary struct {
	Domain      string
	Category    string
	QueryCount  int
	DeviceCount int
}

// TopDomains returns the most-queried domains in the last 24 hours.
func (d *DB) TopDomains(limit int) ([]DomainSummary, error) {
	domains, err := d.TopDomainsWithSource(limit)
	if err != nil {
		return nil, err
	}

	out := make([]DomainSummary, 0, len(domains))
	for _, domain := range domains {
		out = append(out, DomainSummary{
			Domain:      domain.Domain,
			Category:    domain.Category,
			QueryCount:  domain.QueryCount,
			DeviceCount: domain.DeviceCount,
		})
	}
	return out, nil
}

// DeviceDomainSummary holds per-domain stats for a device detail page.
type DeviceDomainSummary struct {
	Domain   string
	Category string
	Count    int
}

// CategoryCount holds per-category query counts for a device detail page.
type CategoryCount struct {
	Category string
	Count    int
}

// DeviceTopDomains returns the most-queried domains for a specific device (last 24 hours).
func (d *DB) DeviceTopDomains(mac string, limit int) ([]DeviceDomainSummary, error) {
	domains, err := d.DeviceTopDomainsWithSource(mac, limit)
	if err != nil {
		return nil, err
	}

	out := make([]DeviceDomainSummary, 0, len(domains))
	for _, domain := range domains {
		out = append(out, DeviceDomainSummary{
			Domain:   domain.Domain,
			Category: domain.Category,
			Count:    domain.QueryCount,
		})
	}
	return out, nil
}

// DeviceCategoryBreakdown returns per-category query counts for a specific device (last 24 hours).
func (d *DB) DeviceCategoryBreakdown(mac string) ([]CategoryCount, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixNano()
	rows, err := d.sql.Query(`
        SELECT COALESCE(category, ''), COUNT(*) as cnt
        FROM queries
        WHERE device_mac = ? AND timestamp >= ?
        GROUP BY category
        ORDER BY cnt DESC, category ASC`, mac, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CategoryCount
	for rows.Next() {
		var cc CategoryCount
		if err := rows.Scan(&cc.Category, &cc.Count); err != nil {
			return nil, err
		}
		out = append(out, cc)
	}
	return out, rows.Err()
}

// DeviceWithStats holds a device record with its query stats for the last 24 hours.
type DeviceWithStats struct {
	Device
	QueryCount     int
	TrackerPercent float64
}

// DeviceWithTrends holds a device record with current stats and trend data.
type DeviceWithTrends struct {
	Device
	QueryCount     int
	TrackerPercent float64
	QueryTrend     Trend
	TrackerTrend   Trend
}

// DevicePrivacySummary holds privacy summary data for a device detail page.
type DevicePrivacySummary struct {
	QueryCount           int
	TrackerPercent       float64
	UniqueDomains        int
	UniqueTrackerDomains int
}

// ListDevicesWithStats returns all devices with their 24-hour query stats in a single query.
func (d *DB) ListDevicesWithStats() ([]DeviceWithStats, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixNano()
	rows, err := d.sql.Query(`
		SELECT d.mac, d.ip, d.hostname, d.vendor, COALESCE(d.label, ''),
		       d.first_seen, d.last_seen,
		       COALESCE(q.cnt, 0),
		       COALESCE(q.tracker_cnt, 0)
		FROM devices d
		LEFT JOIN (
		    SELECT device_mac,
		           COUNT(*) as cnt,
		           SUM(CASE WHEN category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END) as tracker_cnt
		    FROM queries
		    WHERE timestamp >= ?
		    GROUP BY device_mac
		) q ON q.device_mac = d.mac
		ORDER BY q.cnt DESC, d.last_seen DESC`, cutoff)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceWithStats
	for rows.Next() {
		var dws DeviceWithStats
		var firstSeen, lastSeen int64
		var trackerCount int
		if err := rows.Scan(
			&dws.MAC, &dws.IP, &dws.Hostname, &dws.Vendor, &dws.Label,
			&firstSeen, &lastSeen,
			&dws.QueryCount, &trackerCount,
		); err != nil {
			return nil, err
		}
		dws.FirstSeen = time.Unix(0, firstSeen)
		dws.LastSeen = time.Unix(0, lastSeen)
		if dws.QueryCount > 0 {
			dws.TrackerPercent = float64(trackerCount) / float64(dws.QueryCount) * 100
		}
		out = append(out, dws)
	}
	return out, rows.Err()
}

func trendWindowUTC() (time.Time, time.Time, time.Time) {
	return trendWindowAt(time.Now())
}

func trendWindowAt(now time.Time) (time.Time, time.Time, time.Time) {
	now = now.UTC()
	currentStart := now.Add(-24 * time.Hour)
	priorStart := now.Add(-8 * 24 * time.Hour)
	return priorStart, currentStart, now
}

func queryCountTrend(currentCount, priorCount int) Trend {
	trend := Trend{Current: float64(currentCount)}
	if priorCount == 0 {
		return trend
	}

	trend.Previous = float64(priorCount) / 7
	trend.HasPrior = true
	trend.Change = (trend.Current - trend.Previous) / trend.Previous * 100
	return trend
}

func trackerPercentTrend(currentCount, currentTrackerCount, priorCount, priorTrackerCount int) Trend {
	trend := Trend{}
	if currentCount > 0 {
		trend.Current = float64(currentTrackerCount) / float64(currentCount) * 100
	}
	if priorCount == 0 {
		return trend
	}

	trend.Previous = float64(priorTrackerCount) / float64(priorCount) * 100
	if currentCount == 0 {
		return trend
	}

	trend.Change = trend.Current - trend.Previous
	trend.HasPrior = true
	return trend
}

func (d *DB) loadTrends(whereClause string, args ...any) (Trend, Trend, error) {
	return d.LoadTrendsAt(time.Now(), whereClause, args...)
}

func (d *DB) LoadTrendsAt(now time.Time, whereClause string, args ...any) (Trend, Trend, error) {
	priorStart, currentStart, now := trendWindowAt(now)

	query := `
		SELECT
			COALESCE(SUM(CASE WHEN timestamp >= ? AND timestamp < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN timestamp >= ? AND timestamp < ? AND category IN ` + trackingCategorySQL + ` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN timestamp >= ? AND timestamp < ? THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN timestamp >= ? AND timestamp < ? AND category IN ` + trackingCategorySQL + ` THEN 1 ELSE 0 END), 0)
		FROM queries
		WHERE timestamp >= ? AND timestamp < ?`
	params := []any{
		currentStart.UnixNano(), now.UnixNano(),
		currentStart.UnixNano(), now.UnixNano(),
		priorStart.UnixNano(), currentStart.UnixNano(),
		priorStart.UnixNano(), currentStart.UnixNano(),
		priorStart.UnixNano(), now.UnixNano(),
	}
	if whereClause != "" {
		query += " AND " + whereClause
		params = append(params, args...)
	}

	var currentCount, currentTrackerCount, priorCount, priorTrackerCount int
	err := d.sql.QueryRow(query, params...).Scan(
		&currentCount,
		&currentTrackerCount,
		&priorCount,
		&priorTrackerCount,
	)
	if err != nil {
		return Trend{}, Trend{}, err
	}

	return queryCountTrend(currentCount, priorCount), trackerPercentTrend(currentCount, currentTrackerCount, priorCount, priorTrackerCount), nil
}

// DashboardTrends returns period-over-period trend data for the dashboard.
func (d *DB) DashboardTrends() (Trend, Trend, error) {
	return d.loadTrends("")
}

// DeviceTrends returns period-over-period trend data for a device.
func (d *DB) DeviceTrends(mac string) (Trend, Trend, error) {
	return d.loadTrends("device_mac = ?", mac)
}

// ListDevicesWithTrends returns all devices with their 24-hour query stats and trend data.
func (d *DB) ListDevicesWithTrends() ([]DeviceWithTrends, error) {
	return d.ListDevicesWithTrendsAt(time.Now())
}

func (d *DB) ListDevicesWithTrendsAt(now time.Time) ([]DeviceWithTrends, error) {
	priorStart, currentStart, now := trendWindowAt(now)

	rows, err := d.sql.Query(`
		SELECT d.mac, d.ip, d.hostname, d.vendor, COALESCE(d.label, ''),
		       d.first_seen, d.last_seen,
		       COALESCE(q.cur_cnt, 0),
		       COALESCE(q.cur_tracker, 0),
		       COALESCE(q.prev_cnt, 0),
		       COALESCE(q.prev_tracker, 0)
		FROM devices d
		LEFT JOIN (
		    SELECT device_mac,
		           SUM(CASE WHEN timestamp >= ? AND timestamp < ? THEN 1 ELSE 0 END) as cur_cnt,
		           SUM(CASE WHEN timestamp >= ? AND timestamp < ? AND category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END) as cur_tracker,
		           SUM(CASE WHEN timestamp < ? THEN 1 ELSE 0 END) as prev_cnt,
		           SUM(CASE WHEN timestamp < ? AND category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END) as prev_tracker
		    FROM queries
		    WHERE timestamp >= ? AND timestamp < ?
		    GROUP BY device_mac
		) q ON q.device_mac = d.mac
		ORDER BY q.cur_cnt DESC, d.last_seen DESC`,
		currentStart.UnixNano(), now.UnixNano(),
		currentStart.UnixNano(), now.UnixNano(),
		currentStart.UnixNano(),
		currentStart.UnixNano(),
		priorStart.UnixNano(), now.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DeviceWithTrends
	for rows.Next() {
		var dwt DeviceWithTrends
		var firstSeen, lastSeen int64
		var currentTrackerCount, priorCount, priorTrackerCount int
		if err := rows.Scan(
			&dwt.MAC, &dwt.IP, &dwt.Hostname, &dwt.Vendor, &dwt.Label,
			&firstSeen, &lastSeen,
			&dwt.QueryCount, &currentTrackerCount, &priorCount, &priorTrackerCount,
		); err != nil {
			return nil, err
		}
		dwt.FirstSeen = time.Unix(0, firstSeen)
		dwt.LastSeen = time.Unix(0, lastSeen)
		if dwt.QueryCount > 0 {
			dwt.TrackerPercent = float64(currentTrackerCount) / float64(dwt.QueryCount) * 100
		}
		dwt.QueryTrend = queryCountTrend(dwt.QueryCount, priorCount)
		dwt.TrackerTrend = trackerPercentTrend(dwt.QueryCount, currentTrackerCount, priorCount, priorTrackerCount)
		out = append(out, dwt)
	}
	return out, rows.Err()
}

// DevicePrivacySummary returns privacy summary stats for a specific device (last 24 hours).
func (d *DB) DevicePrivacySummary(mac string) (DevicePrivacySummary, error) {
	return d.DevicePrivacySummaryAt(mac, time.Now())
}

func (d *DB) DevicePrivacySummaryAt(mac string, now time.Time) (DevicePrivacySummary, error) {
	cutoff := now.Add(-24 * time.Hour).UnixNano()
	var summary DevicePrivacySummary
	var trackerCount int

	err := d.sql.QueryRow(`
        SELECT COUNT(*),
               COALESCE(SUM(CASE WHEN category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0),
               COUNT(DISTINCT domain),
               COUNT(DISTINCT CASE WHEN category IN `+trackingCategorySQL+` THEN domain END)
        FROM queries
        WHERE device_mac = ? AND timestamp >= ?`, mac, cutoff,
	).Scan(
		&summary.QueryCount,
		&trackerCount,
		&summary.UniqueDomains,
		&summary.UniqueTrackerDomains,
	)
	if err != nil {
		return summary, err
	}
	if summary.QueryCount > 0 {
		summary.TrackerPercent = float64(trackerCount) / float64(summary.QueryCount) * 100
	}
	return summary, nil
}

// NetworkCategoryBreakdown returns per-group query counts for the last 24 hours.
func (d *DB) NetworkCategoryBreakdown() ([]CategoryCount, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixNano()

	var trackingCount, analyticsCount, unclassifiedCount int
	err := d.sql.QueryRow(`
		SELECT
			COALESCE(SUM(CASE WHEN category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN category = 'analytics' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN category NOT IN `+trackingCategorySQL+` AND category != 'analytics' THEN 1 ELSE 0 END), 0)
		FROM queries
		WHERE timestamp >= ?`, cutoff,
	).Scan(&trackingCount, &analyticsCount, &unclassifiedCount)
	if err != nil {
		return nil, err
	}

	out := make([]CategoryCount, 0, 3)
	if trackingCount > 0 {
		out = append(out, CategoryCount{Category: "tracking", Count: trackingCount})
	}
	if unclassifiedCount > 0 {
		out = append(out, CategoryCount{Category: "unclassified", Count: unclassifiedCount})
	}
	if analyticsCount > 0 {
		out = append(out, CategoryCount{Category: "analytics", Count: analyticsCount})
	}
	return out, nil
}

// RangedActivity returns activity buckets for the requested range.
func (d *DB) RangedActivity(deviceMAC string, timeRange string) ([]HourlyBucket, error) {
	switch timeRange {
	case "", "24h":
		return d.HourlyActivity(deviceMAC)
	case "7d":
		return d.dailyActivity(deviceMAC, 7)
	case "30d":
		return d.dailyActivity(deviceMAC, 30)
	default:
		return nil, fmt.Errorf("invalid range %q", timeRange)
	}
}

func (d *DB) dailyActivity(deviceMAC string, bucketCount int) ([]HourlyBucket, error) {
	currentDay := time.Now().UTC().Truncate(24 * time.Hour)
	oldestDay := currentDay.AddDate(0, 0, -(bucketCount - 1))
	dayNS := int64(24 * time.Hour)

	query := `
		SELECT timestamp / ? AS day_key,
		       COUNT(*),
		       COALESCE(SUM(CASE WHEN category IN ` + trackingCategorySQL + ` THEN 1 ELSE 0 END), 0)
		FROM queries
		WHERE timestamp >= ?`
	args := []any{dayNS, oldestDay.UnixNano()}
	if deviceMAC != "" {
		query += ` AND device_mac = ?`
		args = append(args, deviceMAC)
	}
	query += ` GROUP BY day_key ORDER BY day_key`

	rows, err := d.sql.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	countsByDay := make(map[int64]HourlyBucket, bucketCount)
	for rows.Next() {
		var dayKey int64
		var bucket HourlyBucket
		if err := rows.Scan(&dayKey, &bucket.TotalCount, &bucket.TrackerCount); err != nil {
			return nil, err
		}
		countsByDay[dayKey] = bucket
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	buckets := make([]HourlyBucket, 0, bucketCount)
	for i := 0; i < bucketCount; i++ {
		ts := oldestDay.AddDate(0, 0, i)
		dayKey := ts.UnixNano() / dayNS
		bucket := countsByDay[dayKey]
		bucket.Timestamp = ts
		buckets = append(buckets, bucket)
	}

	return buckets, nil
}

// TopDomainsWithSource returns the most-queried domains in the last 24 hours with source attribution.
func (d *DB) TopDomainsWithSource(limit int) ([]DomainWithSource, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixNano()
	rows, err := d.sql.Query(`
		SELECT summary.domain,
		       summary.category,
		       summary.cnt,
		       summary.dev_cnt,
		       COALESCE(
		           (
		               SELECT l.name
		               FROM list_domains ld
		               JOIN lists l ON l.id = ld.list_id
		               WHERE ld.domain = summary.domain AND ld.category = summary.category AND l.enabled = 1
		               ORDER BY l.id ASC
		               LIMIT 1
		           ),
		           CASE
		               WHEN EXISTS (
		                   SELECT 1
		                   FROM domain_overrides do
		                   WHERE do.domain = summary.domain AND do.category = summary.category
		               ) THEN 'manual'
		               ELSE 'unknown'
		           END
		       ) as source_list
		FROM (
		    SELECT domain,
		           MAX(COALESCE(category, '')) as category,
		           COUNT(*) as cnt,
		           COUNT(DISTINCT device_mac) as dev_cnt
		    FROM queries
		    WHERE timestamp >= ?
		    GROUP BY domain
		) summary
		ORDER BY (summary.dev_cnt * summary.cnt) DESC, summary.cnt DESC, summary.domain ASC
		LIMIT ?`, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DomainWithSource
	for rows.Next() {
		var domain DomainWithSource
		if err := rows.Scan(&domain.Domain, &domain.Category, &domain.QueryCount, &domain.DeviceCount, &domain.SourceList); err != nil {
			return nil, err
		}
		out = append(out, domain)
	}
	return out, rows.Err()
}

// DeviceTopDomainsWithSource returns the most-queried domains for a specific device in the last 24 hours with source attribution.
func (d *DB) DeviceTopDomainsWithSource(mac string, limit int) ([]DomainWithSource, error) {
	cutoff := time.Now().Add(-24 * time.Hour).UnixNano()
	rows, err := d.sql.Query(`
		SELECT summary.domain,
		       summary.category,
		       summary.cnt,
		       1 as dev_cnt,
		       COALESCE(
		           (
		               SELECT l.name
		               FROM list_domains ld
		               JOIN lists l ON l.id = ld.list_id
		               WHERE ld.domain = summary.domain AND ld.category = summary.category AND l.enabled = 1
		               ORDER BY l.id ASC
		               LIMIT 1
		           ),
		           CASE
		               WHEN EXISTS (
		                   SELECT 1
		                   FROM domain_overrides do
		                   WHERE do.domain = summary.domain AND do.category = summary.category
		               ) THEN 'manual'
		               ELSE 'unknown'
		           END
		       ) as source_list
		FROM (
		    SELECT domain,
		           MAX(COALESCE(category, '')) as category,
		           COUNT(*) as cnt
		    FROM queries
		    WHERE device_mac = ? AND timestamp >= ?
		    GROUP BY domain
		) summary
		ORDER BY summary.cnt DESC, summary.domain ASC
		LIMIT ?`, mac, cutoff, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []DomainWithSource
	for rows.Next() {
		var domain DomainWithSource
		if err := rows.Scan(&domain.Domain, &domain.Category, &domain.QueryCount, &domain.DeviceCount, &domain.SourceList); err != nil {
			return nil, err
		}
		out = append(out, domain)
	}
	return out, rows.Err()
}

// DeviceAnomalies returns per-device tracker and volume anomalies for the last 24 hours versus the prior 7 days.
func (d *DB) DeviceAnomalies() ([]Anomaly, error) {
	now := time.Now().UTC()
	currentStart := now.Add(-24 * time.Hour)
	priorStart := now.Add(-8 * 24 * time.Hour)

	rows, err := d.sql.Query(`
		SELECT d.mac,
		       COALESCE(NULLIF(d.label, ''), NULLIF(d.hostname, ''), NULLIF(d.vendor, ''), d.mac),
		       COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? THEN 1 ELSE 0 END), 0) as current_count,
		       COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? AND q.category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0) as current_tracker,
		       COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? THEN 1 ELSE 0 END), 0) as prior_count,
		       COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? AND q.category IN `+trackingCategorySQL+` THEN 1 ELSE 0 END), 0) as prior_tracker
		FROM devices d
		LEFT JOIN queries q ON q.device_mac = d.mac AND q.timestamp >= ? AND q.timestamp < ?
		GROUP BY d.mac, d.label, d.hostname, d.vendor
		HAVING current_count > 0`,
		currentStart.UnixNano(), now.UnixNano(),
		currentStart.UnixNano(), now.UnixNano(),
		priorStart.UnixNano(), currentStart.UnixNano(),
		priorStart.UnixNano(), currentStart.UnixNano(),
		priorStart.UnixNano(), now.UnixNano(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Anomaly
	for rows.Next() {
		var deviceMAC, deviceName string
		var currentCount, currentTracker, priorCount, priorTracker int
		if err := rows.Scan(&deviceMAC, &deviceName, &currentCount, &currentTracker, &priorCount, &priorTracker); err != nil {
			return nil, err
		}

		currentTrackerPct := 0.0
		if currentCount > 0 {
			currentTrackerPct = float64(currentTracker) / float64(currentCount) * 100
		}

		averageTrackerPct := 0.0
		if priorCount > 0 {
			averageTrackerPct = float64(priorTracker) / float64(priorCount) * 100
		}
		if currentTrackerPct > averageTrackerPct+5 {
			topDomain, err := d.deviceTopTrackerDomain(deviceMAC, currentStart, now)
			if err != nil {
				return nil, err
			}
			out = append(out, Anomaly{
				DeviceMAC:           deviceMAC,
				DeviceName:          deviceName,
				Type:                "tracker_spike",
				CurrentValue:        currentTrackerPct,
				AverageValue:        averageTrackerPct,
				Delta:               currentTrackerPct - averageTrackerPct,
				TopDomain:           topDomain.Domain,
				TopDomainCategory:   topDomain.Category,
				TopDomainSourceList: topDomain.SourceList,
			})
		}

		if priorCount == 0 {
			continue
		}
		averageQueryCount := float64(priorCount) / 7
		if averageQueryCount > 0 && float64(currentCount) > averageQueryCount*3 {
			topDomain, err := d.deviceTopVolumeSpikeDomain(deviceMAC, currentStart, now, priorStart)
			if err != nil {
				return nil, err
			}
			out = append(out, Anomaly{
				DeviceMAC:           deviceMAC,
				DeviceName:          deviceName,
				Type:                "volume_spike",
				CurrentValue:        float64(currentCount),
				AverageValue:        averageQueryCount,
				Delta:               float64(currentCount) - averageQueryCount,
				TopDomain:           topDomain.Domain,
				TopDomainCategory:   topDomain.Category,
				TopDomainSourceList: topDomain.SourceList,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return out, nil
}

func (d *DB) deviceTopTrackerDomain(mac string, currentStart, now time.Time) (DomainWithSource, error) {
	rows, err := d.sql.Query(`
		SELECT q.domain,
		       q.category,
		       COUNT(*) as cnt,
		       1 as dev_cnt,
		       COALESCE(
		           (
		               SELECT l.name
		               FROM list_domains ld
		               JOIN lists l ON l.id = ld.list_id
		               WHERE ld.domain = q.domain AND ld.category = q.category AND l.enabled = 1
		               ORDER BY l.id ASC
		               LIMIT 1
		           ),
		           CASE
		               WHEN EXISTS (
		                   SELECT 1
		                   FROM domain_overrides do
		                   WHERE do.domain = q.domain AND do.category = q.category
		               ) THEN 'manual'
		               ELSE 'unknown'
		           END
		       ) as source_list
		FROM queries q
		WHERE q.device_mac = ? AND q.timestamp >= ? AND q.timestamp < ? AND q.category IN `+trackingCategorySQL+`
		GROUP BY q.domain, q.category
		ORDER BY cnt DESC, q.domain ASC
		LIMIT 1`, mac, currentStart.UnixNano(), now.UnixNano())
	if err != nil {
		return DomainWithSource{}, err
	}
	defer rows.Close()

	if rows.Next() {
		var domain DomainWithSource
		if err := rows.Scan(&domain.Domain, &domain.Category, &domain.QueryCount, &domain.DeviceCount, &domain.SourceList); err != nil {
			return DomainWithSource{}, err
		}
		return domain, rows.Err()
	}
	return DomainWithSource{}, nil
}

func (d *DB) deviceTopVolumeSpikeDomain(mac string, currentStart, now, priorStart time.Time) (DomainWithSource, error) {
	rows, err := d.sql.Query(`
		SELECT q.domain,
		       q.category,
		       COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? THEN 1 ELSE 0 END), 0) as current_count,
		       1 as dev_cnt,
		       COALESCE(
		           (
		               SELECT l.name
		               FROM list_domains ld
		               JOIN lists l ON l.id = ld.list_id
		               WHERE ld.domain = q.domain AND ld.category = q.category AND l.enabled = 1
		               ORDER BY l.id ASC
		               LIMIT 1
		           ),
		           CASE
		               WHEN EXISTS (
		                   SELECT 1
		                   FROM domain_overrides do
		                   WHERE do.domain = q.domain AND do.category = q.category
		               ) THEN 'manual'
		               ELSE 'unknown'
		           END
		       ) as source_list,
		       (
		           COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? THEN 1 ELSE 0 END), 0) -
		           (COALESCE(SUM(CASE WHEN q.timestamp >= ? AND q.timestamp < ? THEN 1 ELSE 0 END), 0) / 7.0)
		       ) as delta
		FROM queries q
		WHERE q.device_mac = ? AND q.timestamp >= ? AND q.timestamp < ?
		GROUP BY q.domain, q.category
		ORDER BY delta DESC, current_count DESC, q.domain ASC
		LIMIT 1`,
		currentStart.UnixNano(), now.UnixNano(),
		currentStart.UnixNano(), now.UnixNano(),
		priorStart.UnixNano(), currentStart.UnixNano(),
		mac, priorStart.UnixNano(), now.UnixNano(),
	)
	if err != nil {
		return DomainWithSource{}, err
	}
	defer rows.Close()

	if rows.Next() {
		var domain DomainWithSource
		var delta float64
		if err := rows.Scan(&domain.Domain, &domain.Category, &domain.QueryCount, &domain.DeviceCount, &domain.SourceList, &delta); err != nil {
			return DomainWithSource{}, err
		}
		return domain, rows.Err()
	}
	return DomainWithSource{}, nil
}
