package demo

import (
	"time"

	"umberrelay/internal/store"
)

// Seed populates an empty database with representative demo data for local UI work.
// If queries already exist, it leaves the database unchanged.
func Seed(db *store.DB, now time.Time) error {
	existing, err := db.QueryLog("", "", time.Time{}, now.Add(24*time.Hour), 1, 0)
	if err != nil {
		return err
	}
	if len(existing) > 0 {
		return nil
	}

	devices := []store.Device{
		{
			MAC:       "aa:bb:cc:dd:ee:01",
			IP:        "192.168.1.10",
			Hostname:  "roku-tv",
			Vendor:    "Roku",
			FirstSeen: now.Add(-72 * time.Hour),
			LastSeen:  now.Add(-10 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:02",
			IP:        "192.168.1.20",
			Hostname:  "iphone",
			Vendor:    "Apple",
			FirstSeen: now.Add(-72 * time.Hour),
			LastSeen:  now.Add(-5 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:03",
			IP:        "192.168.1.30",
			Hostname:  "echo-dot",
			Vendor:    "Amazon",
			FirstSeen: now.Add(-72 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:04",
			IP:        "192.168.1.40",
			Hostname:  "macbook-pro",
			Vendor:    "Apple",
			FirstSeen: now.Add(-72 * time.Hour),
			LastSeen:  now.Add(-1 * time.Minute),
		},
	}
	for _, dev := range devices {
		if err := db.UpsertDevice(dev); err != nil {
			return err
		}
	}

	labels := map[string]string{
		"aa:bb:cc:dd:ee:01": "Living Room TV",
		"aa:bb:cc:dd:ee:02": "Thom iPhone",
		"aa:bb:cc:dd:ee:03": "Kitchen Echo",
		"aa:bb:cc:dd:ee:04": "Work Laptop",
	}
	for mac, label := range labels {
		if err := db.UpdateDeviceLabel(mac, label); err != nil {
			return err
		}
	}

	queries := []store.Query{
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "scribe.logs.roku.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-47 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "captive.roku.com", QueryType: "A", Category: "", Timestamp: now.Add(-46 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "api.apple.com", QueryType: "A", Category: "", Timestamp: now.Add(-44 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "app-measurement.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-42 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "device-metrics-us.amazon.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-40 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "github.com", QueryType: "A", Category: "", Timestamp: now.Add(-38 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "segment.io", QueryType: "A", Category: "analytics", Timestamp: now.Add(-36 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "doubleclick.net", QueryType: "A", Category: "advertising", Timestamp: now.Add(-24 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "ads.example-cdn.net", QueryType: "A", Category: "advertising", Timestamp: now.Add(-8 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "logs.roku.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(-90 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "icloud.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(-6 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "mzstatic.com", QueryType: "A", Category: "", Timestamp: now.Add(-4 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "app-measurement.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-45 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "music.amazon.com", QueryType: "A", Category: "", Timestamp: now.Add(-3 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "fls-na.amazon.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-70 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "device-metrics-us.amazon.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-20 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "api.github.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "sentry.io", QueryType: "A", Category: "analytics", Timestamp: now.Add(-30 * time.Minute)},
	}
	if err := db.WriteQueries(queries); err != nil {
		return err
	}

	if err := db.SetConfig("retention_days", "30"); err != nil {
		return err
	}
	if err := db.SetConfig("list_refresh_hours", "24"); err != nil {
		return err
	}

	listID, err := db.AddList("https://demo.umberrelay.local/lists/privacy.txt", "Demo Privacy List", "tracking")
	if err != nil {
		return err
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"scribe.logs.roku.com":           "tracking",
		"logs.roku.com":                  "tracking",
		"doubleclick.net":                "advertising",
		"ads.example-cdn.net":            "advertising",
		"app-measurement.com":            "analytics",
		"device-metrics-us.amazon.com":   "analytics",
		"fls-na.amazon.com":              "tracking",
		"segment.io":                     "analytics",
		"sentry.io":                      "analytics",
	}); err != nil {
		return err
	}
	if err := db.UpdateListFetchTime(listID); err != nil {
		return err
	}

	return nil
}
