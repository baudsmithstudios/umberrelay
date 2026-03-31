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
			FirstSeen: now.Add(-10 * 24 * time.Hour),
			LastSeen:  now.Add(-10 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:02",
			IP:        "192.168.1.20",
			Hostname:  "iphone",
			Vendor:    "Apple",
			FirstSeen: now.Add(-10 * 24 * time.Hour),
			LastSeen:  now.Add(-5 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:03",
			IP:        "192.168.1.30",
			Hostname:  "echo-dot",
			Vendor:    "Amazon",
			FirstSeen: now.Add(-10 * 24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:04",
			IP:        "192.168.1.40",
			Hostname:  "macbook-pro",
			Vendor:    "Apple",
			FirstSeen: now.Add(-10 * 24 * time.Hour),
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

	queries := baselineQueries(now)
	queries = append(queries, spikeQueries(now)...)
	queries = append(queries, recentQueries(now)...)
	if err := db.WriteQueries(queries); err != nil {
		return err
	}

	if err := db.SetConfig("retention_days", "30"); err != nil {
		return err
	}
	if err := db.SetConfig("list_refresh_hours", "24"); err != nil {
		return err
	}

	if err := seedLists(db); err != nil {
		return err
	}

	return nil
}

// baselineQueries generates 8 days of steady traffic across all devices.
// Each device gets a handful of queries per day with a realistic mix of
// categories to establish a stable 7d rolling average for anomaly detection.
func baselineQueries(now time.Time) []store.Query {
	type pattern struct {
		mac      string
		domain   string
		category string
	}

	daily := []pattern{
		{"aa:bb:cc:dd:ee:01", "captive.roku.com", ""},
		{"aa:bb:cc:dd:ee:01", "scribe.logs.roku.com", "tracking"},
		{"aa:bb:cc:dd:ee:01", "ads.roku.com", "advertising"},
		{"aa:bb:cc:dd:ee:02", "api.apple.com", ""},
		{"aa:bb:cc:dd:ee:02", "icloud.com", ""},
		{"aa:bb:cc:dd:ee:02", "app-measurement.com", "analytics"},
		{"aa:bb:cc:dd:ee:03", "music.amazon.com", ""},
		{"aa:bb:cc:dd:ee:03", "device-metrics-us.amazon.com", "analytics"},
		{"aa:bb:cc:dd:ee:04", "github.com", ""},
		{"aa:bb:cc:dd:ee:04", "api.github.com", ""},
		{"aa:bb:cc:dd:ee:04", "sentry.io", "analytics"},
	}

	var out []store.Query
	for day := 8; day >= 2; day-- {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		for i, p := range daily {
			out = append(out, store.Query{
				DeviceMAC: p.mac,
				Domain:    p.domain,
				QueryType: "A",
				Category:  p.category,
				Timestamp: base.Add(time.Duration(i) * 20 * time.Minute),
			})
		}
		// Extra volume for Roku to make its baseline realistic
		for j := 0; j < 3; j++ {
			out = append(out, store.Query{
				DeviceMAC: "aa:bb:cc:dd:ee:01",
				Domain:    "captive.roku.com",
				QueryType: "AAAA",
				Category:  "",
				Timestamp: base.Add(time.Duration(12+j) * 20 * time.Minute),
			})
		}
	}
	return out
}

// spikeQueries generates a tracker spike for the Roku in the current 24h.
// The Roku's baseline is ~1 tracker per day out of ~4 queries (25%).
// This spike adds 12 tracker queries, pushing it well above the 5pp threshold.
func spikeQueries(now time.Time) []store.Query {
	spikeDomains := []struct {
		domain   string
		category string
	}{
		{"doubleclick.net", "advertising"},
		{"ads.example-cdn.net", "advertising"},
		{"fls-na.amazon.com", "tracking"},
		{"tracker.example.com", "tracking"},
	}

	var out []store.Query
	base := now.Add(-8 * time.Hour)
	for i := 0; i < 12; i++ {
		sd := spikeDomains[i%len(spikeDomains)]
		out = append(out, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:01",
			Domain:    sd.domain,
			QueryType: "A",
			Category:  sd.category,
			Timestamp: base.Add(time.Duration(i) * 10 * time.Minute),
		})
	}
	return out
}

// recentQueries adds current-day activity for all devices so the 24h view
// and overview stats have data.
func recentQueries(now time.Time) []store.Query {
	return []store.Query{
		// Roku — normal queries alongside the spike
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "captive.roku.com", QueryType: "A", Category: "", Timestamp: now.Add(-6 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:01", Domain: "logs.roku.com", QueryType: "AAAA", Category: "tracking", Timestamp: now.Add(-90 * time.Minute)},

		// iPhone — normal day
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "icloud.com", QueryType: "AAAA", Category: "", Timestamp: now.Add(-6 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "mzstatic.com", QueryType: "A", Category: "", Timestamp: now.Add(-4 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:02", Domain: "app-measurement.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-45 * time.Minute)},

		// Echo — normal day
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "music.amazon.com", QueryType: "A", Category: "", Timestamp: now.Add(-3 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "fls-na.amazon.com", QueryType: "A", Category: "tracking", Timestamp: now.Add(-70 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:03", Domain: "device-metrics-us.amazon.com", QueryType: "A", Category: "analytics", Timestamp: now.Add(-20 * time.Minute)},

		// MacBook — normal day
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "api.github.com", QueryType: "A", Category: "", Timestamp: now.Add(-2 * time.Hour)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "sentry.io", QueryType: "A", Category: "analytics", Timestamp: now.Add(-30 * time.Minute)},
		{DeviceMAC: "aa:bb:cc:dd:ee:04", Domain: "segment.io", QueryType: "A", Category: "analytics", Timestamp: now.Add(-15 * time.Minute)},
	}
}

func seedLists(db *store.DB) error {
	listID, err := db.AddList("https://demo.umberrelay.local/lists/privacy.txt", "Demo Privacy List", "tracking")
	if err != nil {
		return err
	}
	if err := db.WriteListDomains(listID, map[string]string{
		"scribe.logs.roku.com":         "tracking",
		"logs.roku.com":                "tracking",
		"ads.roku.com":                 "advertising",
		"doubleclick.net":              "advertising",
		"ads.example-cdn.net":          "advertising",
		"tracker.example.com":          "tracking",
		"app-measurement.com":          "analytics",
		"device-metrics-us.amazon.com": "analytics",
		"fls-na.amazon.com":            "tracking",
		"segment.io":                   "analytics",
		"sentry.io":                    "analytics",
	}); err != nil {
		return err
	}
	return db.UpdateListFetchTime(listID)
}
