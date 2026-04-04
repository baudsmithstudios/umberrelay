package demo

import (
	"math/rand"
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

	for _, dev := range demoDevices(now) {
		if err := db.UpsertDevice(dev); err != nil {
			return err
		}
	}
	for mac, label := range demoLabels() {
		if err := db.UpdateDeviceLabel(mac, label); err != nil {
			return err
		}
	}

	// Source-only actors (devices we see queries from but have no MAC/hostname)
	for ip, label := range demoSourceLabels() {
		if err := db.SetSourceLabel(ip, label); err != nil {
			return err
		}
	}

	rng := rand.New(rand.NewSource(42))
	queries := baselineQueries(now, rng)
	queries = append(queries, spikeQueries(now, rng)...)
	queries = append(queries, recentQueries(now, rng)...)
	queries = append(queries, sourceQueries(now, rng)...)
	queries = append(queries, bypassCandidateQueries(now)...)
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

	// Manual override to show the feature in the UI
	if err := db.SetDomainOverride("ntp.ubuntu.com", "uncategorized"); err != nil {
		return err
	}

	return nil
}

func demoDevices(now time.Time) []store.Device {
	return []store.Device{
		{
			MAC:       "aa:bb:cc:dd:ee:01",
			IP:        "192.168.1.10",
			Hostname:  "roku-tv",
			Vendor:    "Roku",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-10 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:02",
			IP:        "192.168.1.20",
			Hostname:  "wireless-phone",
			Vendor:    "Apple",
			FirstSeen: now.Add(-28 * 24 * time.Hour),
			LastSeen:  now.Add(-5 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:03",
			IP:        "192.168.1.30",
			Hostname:  "echo-dot",
			Vendor:    "Amazon",
			FirstSeen: now.Add(-25 * 24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:04",
			IP:        "192.168.1.40",
			Hostname:  "work-laptop",
			Vendor:    "Apple",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-1 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:05",
			IP:        "192.168.1.50",
			Hostname:  "living-room-speaker",
			Vendor:    "Google",
			FirstSeen: now.Add(-20 * 24 * time.Hour),
			LastSeen:  now.Add(-3 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:06",
			IP:        "192.168.1.60",
			Hostname:  "bedroom-tv",
			Vendor:    "Samsung",
			FirstSeen: now.Add(-18 * 24 * time.Hour),
			LastSeen:  now.Add(-8 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:07",
			IP:        "192.168.1.70",
			Hostname:  "tablet",
			Vendor:    "Apple",
			FirstSeen: now.Add(-15 * 24 * time.Hour),
			LastSeen:  now.Add(-12 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:08",
			IP:        "192.168.1.80",
			Hostname:  "gaming-console",
			Vendor:    "Sony",
			FirstSeen: now.Add(-22 * 24 * time.Hour),
			LastSeen:  now.Add(-20 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:09",
			IP:        "192.168.1.90",
			Hostname:  "thermostat",
			Vendor:    "Ecobee",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-4 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:0a",
			IP:        "192.168.1.100",
			Hostname:  "doorbell-cam",
			Vendor:    "Ring",
			FirstSeen: now.Add(-26 * 24 * time.Hour),
			LastSeen:  now.Add(-6 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:0b",
			IP:        "192.168.1.110",
			Hostname:  "robot-vacuum",
			Vendor:    "iRobot",
			FirstSeen: now.Add(-12 * 24 * time.Hour),
			LastSeen:  now.Add(-15 * time.Minute),
		},
		{
			MAC:       "aa:bb:cc:dd:ee:0c",
			IP:        "192.168.1.120",
			Hostname:  "desktop-pc",
			Vendor:    "Dell",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-7 * time.Minute),
		},
		// Bypass candidate: recently seen on LAN but no DNS queries in last 45 min.
		// Has historical queries including an encrypted DNS domain to trigger "likely" confidence.
		{
			MAC:       "aa:bb:cc:dd:ee:0d",
			IP:        "192.168.1.130",
			Hostname:  "guest-laptop",
			Vendor:    "Lenovo",
			FirstSeen: now.Add(-5 * 24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
	}
}

func demoLabels() map[string]string {
	return map[string]string{
		"aa:bb:cc:dd:ee:01": "Living Room TV",
		"aa:bb:cc:dd:ee:02": "Main Phone",
		"aa:bb:cc:dd:ee:03": "Kitchen Echo",
		"aa:bb:cc:dd:ee:04": "Work Laptop",
		"aa:bb:cc:dd:ee:05": "Living Room Speaker",
		"aa:bb:cc:dd:ee:06": "Bedroom TV",
		"aa:bb:cc:dd:ee:07": "Tablet",
		"aa:bb:cc:dd:ee:08": "Gaming Console",
		"aa:bb:cc:dd:ee:09": "Thermostat",
		"aa:bb:cc:dd:ee:0a": "Front Door Cam",
		"aa:bb:cc:dd:ee:0b": "Robot Vacuum",
		"aa:bb:cc:dd:ee:0c": "Desktop PC",
		"aa:bb:cc:dd:ee:0d": "Guest Laptop",
	}
}

func demoSourceLabels() map[string]string {
	return map[string]string{
		"10.0.0.50": "Wireguard Tunnel",
	}
}

// deviceTraffic defines the per-device query patterns used to generate baseline data.
type deviceTraffic struct {
	mac     string
	queries []queryPattern
}

type queryPattern struct {
	domain   string
	category string
	weight   int // relative frequency (1 = 1x per day, 3 = 3x per day)
}

func trafficPatterns() []deviceTraffic {
	return []deviceTraffic{
		{mac: "aa:bb:cc:dd:ee:01", queries: []queryPattern{
			{"captive.roku.com", "", 4},
			{"scribe.logs.roku.com", "tracking", 3},
			{"ads.roku.com", "advertising", 2},
			{"cooper.logs.roku.com", "tracking", 2},
			{"cloudservices.roku.com", "", 3},
			{"content-delivery.roku.com", "", 5},
		}},
		{mac: "aa:bb:cc:dd:ee:02", queries: []queryPattern{
			{"api.apple.com", "", 6},
			{"icloud.com", "", 5},
			{"gsp-ssl.ls.apple.com", "", 3},
			{"app-measurement.com", "analytics", 2},
			{"weather.apple.com", "", 2},
			{"cdn.syndication.twimg.com", "", 3},
			{"graph.instagram.com", "", 2},
			{"app.adjust.com", "tracking", 1},
		}},
		{mac: "aa:bb:cc:dd:ee:03", queries: []queryPattern{
			{"music.amazon.com", "", 4},
			{"device-metrics-us.amazon.com", "analytics", 3},
			{"alexa.amazon.com", "", 5},
			{"unagi-na.amazon.com", "telemetry", 2},
			{"mads-adz.amazon.com", "advertising", 1},
		}},
		{mac: "aa:bb:cc:dd:ee:04", queries: []queryPattern{
			{"github.com", "", 8},
			{"api.github.com", "", 6},
			{"copilot-proxy.githubusercontent.com", "", 4},
			{"sentry.io", "analytics", 2},
			{"segment.io", "analytics", 1},
			{"ntp.ubuntu.com", "", 2},
			{"registry.npmjs.org", "", 3},
			{"fonts.googleapis.com", "", 2},
		}},
		{mac: "aa:bb:cc:dd:ee:05", queries: []queryPattern{
			{"clients4.google.com", "", 3},
			{"play.googleapis.com", "", 2},
			{"connectivitycheck.gstatic.com", "", 4},
			{"firebaselogging-pa.googleapis.com", "analytics", 2},
			{"adservice.google.com", "advertising", 1},
		}},
		{mac: "aa:bb:cc:dd:ee:06", queries: []queryPattern{
			{"samsungcloudsolution.com", "", 3},
			{"cdn.samsungcloudsolution.com", "", 2},
			{"samsung-ads.com", "advertising", 3},
			{"config.samsungads.com", "advertising", 2},
			{"gpm.samsungqbe.com", "telemetry", 2},
			{"log-config.samsungacr.com", "tracking", 2},
		}},
		{mac: "aa:bb:cc:dd:ee:07", queries: []queryPattern{
			{"api.apple.com", "", 3},
			{"identity.apple.com", "", 2},
			{"news-events.apple.com", "", 2},
			{"app-measurement.com", "analytics", 1},
			{"icloud.com", "", 3},
		}},
		{mac: "aa:bb:cc:dd:ee:08", queries: []queryPattern{
			{"ps5.np.playstation.net", "", 5},
			{"telemetry.api.playstation.com", "telemetry", 2},
			{"store.playstation.com", "", 3},
			{"gs-sec.ww.np.dl.playstation.net", "", 2},
		}},
		{mac: "aa:bb:cc:dd:ee:09", queries: []queryPattern{
			{"api.ecobee.com", "", 6},
			{"mqtt.ecobee.com", "", 8},
			{"weather.ecobee.com", "", 4},
		}},
		{mac: "aa:bb:cc:dd:ee:0a", queries: []queryPattern{
			{"fw.ring.com", "", 4},
			{"app-analytics.ring.com", "analytics", 3},
			{"ntp-g7g.amazon.com", "", 2},
			{"prod.api.ring.com", "", 5},
			{"ring-account.ring.com", "", 2},
		}},
		{mac: "aa:bb:cc:dd:ee:0b", queries: []queryPattern{
			{"disc-prod.iot.irobotapi.com", "", 3},
			{"data.iot.irobotapi.com", "telemetry", 2},
			{"api.irobot.com", "", 2},
		}},
		{mac: "aa:bb:cc:dd:ee:0c", queries: []queryPattern{
			{"github.com", "", 5},
			{"cdn.jsdelivr.net", "", 3},
			{"steamcommunity.com", "", 2},
			{"store.steampowered.com", "", 2},
			{"tracking.epicgames.com", "tracking", 1},
			{"sentry.io", "analytics", 1},
			{"fonts.googleapis.com", "", 2},
			{"ntp.ubuntu.com", "", 1},
		}},
	}
}

// baselineQueries generates 8 days of steady traffic across all devices.
// Each device gets queries per day proportional to the weight field, with
// slight random jitter in timing to make the data look natural.
func baselineQueries(now time.Time, rng *rand.Rand) []store.Query {
	patterns := trafficPatterns()
	var out []store.Query

	for day := 8; day >= 2; day-- {
		dayStart := now.Add(-time.Duration(day) * 24 * time.Hour)
		for _, device := range patterns {
			minuteOffset := 0
			for _, q := range device.queries {
				for rep := 0; rep < q.weight; rep++ {
					jitter := time.Duration(rng.Intn(40)) * time.Minute
					ts := dayStart.Add(time.Duration(minuteOffset)*time.Minute + jitter)
					out = append(out, store.Query{
						DeviceMAC: device.mac,
						Domain:    q.domain,
						QueryType: queryTypes[rng.Intn(len(queryTypes))],
						Category:  q.category,
						Timestamp: ts,
					})
					minuteOffset += 15 + rng.Intn(30)
					if minuteOffset > 1380 { // don't exceed 23 hours
						minuteOffset = rng.Intn(120)
					}
				}
			}
		}
	}
	return out
}

var queryTypes = []string{"A", "AAAA", "A", "A"} // bias toward A records

// spikeQueries generates a tracker spike for the Roku (Living Room TV) in the
// current 24h. Its baseline is ~26% tracker rate; this spike pushes it well
// above the 5pp anomaly threshold. Also generates a volume spike on the Samsung TV.
func spikeQueries(now time.Time, rng *rand.Rand) []store.Query {
	var out []store.Query

	// Roku tracker spike — burst of ad/tracking domains
	rokuSpikeDomains := []struct {
		domain   string
		category string
	}{
		{"doubleclick.net", "advertising"},
		{"ads.example-cdn.net", "advertising"},
		{"fls-na.amazon.com", "tracking"},
		{"tracker.example.com", "tracking"},
		{"pagead2.googlesyndication.com", "advertising"},
		{"adsserver.example.net", "advertising"},
	}
	base := now.Add(-8 * time.Hour)
	for i := 0; i < 18; i++ {
		sd := rokuSpikeDomains[i%len(rokuSpikeDomains)]
		out = append(out, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:01",
			Domain:    sd.domain,
			QueryType: queryTypes[rng.Intn(len(queryTypes))],
			Category:  sd.category,
			Timestamp: base.Add(time.Duration(i)*8*time.Minute + time.Duration(rng.Intn(5))*time.Minute),
		})
	}

	// Samsung TV volume spike — sudden burst of normal + tracking queries
	samsungSpikeDomains := []struct {
		domain   string
		category string
	}{
		{"cdn.samsungcloudsolution.com", ""},
		{"samsung-ads.com", "advertising"},
		{"config.samsungads.com", "advertising"},
		{"samsungcloudsolution.com", ""},
		{"log-config.samsungacr.com", "tracking"},
		{"gpm.samsungqbe.com", "telemetry"},
	}
	samsungBase := now.Add(-6 * time.Hour)
	for i := 0; i < 60; i++ {
		sd := samsungSpikeDomains[i%len(samsungSpikeDomains)]
		out = append(out, store.Query{
			DeviceMAC: "aa:bb:cc:dd:ee:06",
			Domain:    sd.domain,
			QueryType: queryTypes[rng.Intn(len(queryTypes))],
			Category:  sd.category,
			Timestamp: samsungBase.Add(time.Duration(i)*3*time.Minute + time.Duration(rng.Intn(3))*time.Minute),
		})
	}

	return out
}

// recentQueries adds current-day activity for all devices so the 24h view
// and overview stats have data. Generates multiple queries per device spread
// across the day for a realistic distribution.
func recentQueries(now time.Time, rng *rand.Rand) []store.Query {
	patterns := trafficPatterns()
	var out []store.Query

	dayStart := now.Add(-20 * time.Hour)
	for _, device := range patterns {
		minuteOffset := 0
		for _, q := range device.queries {
			// Generate weight * 2 queries for recent day to get good volume
			reps := q.weight * 2
			for rep := 0; rep < reps; rep++ {
				jitter := time.Duration(rng.Intn(30)) * time.Minute
				ts := dayStart.Add(time.Duration(minuteOffset)*time.Minute + jitter)
				if ts.After(now) {
					ts = now.Add(-time.Duration(rng.Intn(60)+1) * time.Minute)
				}
				out = append(out, store.Query{
					DeviceMAC: device.mac,
					Domain:    q.domain,
					QueryType: queryTypes[rng.Intn(len(queryTypes))],
					Category:  q.category,
					Timestamp: ts,
				})
				minuteOffset += 8 + rng.Intn(20)
				if minuteOffset > 1200 {
					minuteOffset = rng.Intn(60)
				}
			}
		}
	}

	return out
}

// sourceQueries generates queries from source-only IPs (no associated device)
// to demonstrate unattributed traffic in the devices list.
func sourceQueries(now time.Time, rng *rand.Rand) []store.Query {
	type sourcePattern struct {
		ip      string
		queries []queryPattern
	}

	sources := []sourcePattern{
		{ip: "10.0.0.50", queries: []queryPattern{
			{"vpn-gateway.example.net", "", 3},
			{"api.protonvpn.ch", "", 2},
			{"dns.quad9.net", "", 4},
		}},
		{ip: "10.0.0.75", queries: []queryPattern{
			{"printer.local", "", 2},
			{"hp-updates.hpcloud.com", "", 1},
		}},
	}

	var out []store.Query
	for day := 8; day >= 0; day-- {
		dayStart := now.Add(-time.Duration(day) * 24 * time.Hour)
		for _, src := range sources {
			minuteOffset := 60 + rng.Intn(120)
			for _, q := range src.queries {
				for rep := 0; rep < q.weight; rep++ {
					jitter := time.Duration(rng.Intn(40)) * time.Minute
					ts := dayStart.Add(time.Duration(minuteOffset)*time.Minute + jitter)
					if ts.After(now) {
						continue
					}
					out = append(out, store.Query{
						SourceIP:  src.ip,
						Domain:    q.domain,
						QueryType: queryTypes[rng.Intn(len(queryTypes))],
						Category:  q.category,
						Timestamp: ts,
					})
					minuteOffset += 30 + rng.Intn(60)
				}
			}
		}
	}

	return out
}

// bypassCandidateQueries generates historical queries for the guest laptop
// that trigger the bypass detection heuristic: the device was recently seen
// on the LAN but has zero DNS queries in the last 45 minutes, with prior
// encrypted DNS bootstrap domain queries.
func bypassCandidateQueries(now time.Time) []store.Query {
	mac := "aa:bb:cc:dd:ee:0d"
	var out []store.Query

	// Historical queries (more than 45 min ago but within 30 days)
	for day := 5; day >= 1; day-- {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		out = append(out, store.Query{
			DeviceMAC: mac,
			Domain:    "dns.google",
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(2 * time.Hour),
		})
		out = append(out, store.Query{
			DeviceMAC: mac,
			Domain:    "example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(4 * time.Hour),
		})
	}

	// Recent queries but older than 45 min — establishes prior activity
	out = append(out, store.Query{
		DeviceMAC: mac,
		Domain:    "dns.google",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-3 * time.Hour),
	})
	out = append(out, store.Query{
		DeviceMAC: mac,
		Domain:    "clients.l.google.com",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-2 * time.Hour),
	})

	// No queries in the last 45 minutes — this is the silence that triggers bypass detection
	return out
}

func seedLists(db *store.DB) error {
	trackingID, err := db.AddList("https://demo.umberrelay.local/lists/tracking.txt", "Tracker Blocklist", "tracking")
	if err != nil {
		return err
	}
	if err := db.WriteListDomains(trackingID, map[string]string{
		"scribe.logs.roku.com":         "tracking",
		"cooper.logs.roku.com":         "tracking",
		"ads.roku.com":                 "advertising",
		"doubleclick.net":              "advertising",
		"ads.example-cdn.net":          "advertising",
		"tracker.example.com":          "tracking",
		"pagead2.googlesyndication.com": "advertising",
		"adsserver.example.net":        "advertising",
		"app.adjust.com":               "tracking",
		"app-measurement.com":          "analytics",
		"samsung-ads.com":              "advertising",
		"config.samsungads.com":        "advertising",
		"log-config.samsungacr.com":    "tracking",
		"tracking.epicgames.com":       "tracking",
		"adservice.google.com":         "advertising",
		"mads-adz.amazon.com":          "advertising",
	}); err != nil {
		return err
	}
	if err := db.UpdateListFetchTime(trackingID); err != nil {
		return err
	}

	telemetryID, err := db.AddList("https://demo.umberrelay.local/lists/telemetry.txt", "Telemetry List", "telemetry")
	if err != nil {
		return err
	}
	if err := db.WriteListDomains(telemetryID, map[string]string{
		"device-metrics-us.amazon.com":         "analytics",
		"fls-na.amazon.com":                    "tracking",
		"unagi-na.amazon.com":                  "telemetry",
		"sentry.io":                            "analytics",
		"segment.io":                           "analytics",
		"firebaselogging-pa.googleapis.com":    "analytics",
		"app-analytics.ring.com":               "analytics",
		"data.iot.irobotapi.com":               "telemetry",
		"gpm.samsungqbe.com":                   "telemetry",
		"telemetry.api.playstation.com":         "telemetry",
	}); err != nil {
		return err
	}
	if err := db.UpdateListFetchTime(telemetryID); err != nil {
		return err
	}

	return nil
}
