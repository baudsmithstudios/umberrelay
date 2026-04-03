package store

const schema = `
CREATE TABLE IF NOT EXISTS devices (
    mac        TEXT PRIMARY KEY,
    ip         TEXT,
    hostname   TEXT,
    vendor     TEXT,
    label      TEXT,
    first_seen INTEGER,
    last_seen  INTEGER
);

CREATE TABLE IF NOT EXISTS queries (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    device_mac TEXT REFERENCES devices(mac),
    source_ip  TEXT    NOT NULL DEFAULT '',
    domain     TEXT    NOT NULL,
    query_type TEXT    NOT NULL,
    category   TEXT    NOT NULL DEFAULT '',
    timestamp  INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_queries_device ON queries(device_mac, timestamp);
CREATE INDEX IF NOT EXISTS idx_queries_domain ON queries(domain, timestamp);
CREATE INDEX IF NOT EXISTS idx_queries_ts ON queries(timestamp);

CREATE TABLE IF NOT EXISTS lists (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    url        TEXT    UNIQUE NOT NULL,
    name       TEXT    NOT NULL,
    category   TEXT    NOT NULL,
    last_fetch INTEGER,
    enabled    INTEGER NOT NULL DEFAULT 1
);

CREATE TABLE IF NOT EXISTS list_domains (
    list_id  INTEGER NOT NULL REFERENCES lists(id) ON DELETE CASCADE,
    domain   TEXT    NOT NULL,
    category TEXT    NOT NULL,
    PRIMARY KEY (list_id, domain)
);
CREATE INDEX IF NOT EXISTS idx_list_domains_domain ON list_domains(domain);

CREATE TABLE IF NOT EXISTS config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS domain_overrides (
    domain     TEXT PRIMARY KEY,
    category   TEXT    NOT NULL,
    created_at INTEGER NOT NULL
);
`
