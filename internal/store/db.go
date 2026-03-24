package store

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

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

// DB wraps the SQLite connection.
type DB struct {
	sql *sql.DB
}

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
	_, err := d.sql.Exec(`UPDATE devices SET label = ? WHERE mac = ?`, label, mac)
	return err
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
	_, err := d.sql.Exec(`DELETE FROM lists WHERE id = ?`, id)
	return err
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
