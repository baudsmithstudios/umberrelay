package demo

import (
	"fmt"
	"math/rand"
	"time"

	"umberrelay/internal/store"
)

const (
	macRokuTV      = "aa:bb:cc:dd:ee:01"
	macPhone       = "aa:bb:cc:dd:ee:02"
	macEchoDot     = "aa:bb:cc:dd:ee:03"
	macWorkLaptop  = "aa:bb:cc:dd:ee:04"
	macNestHub     = "aa:bb:cc:dd:ee:05"
	macSamsungTV   = "aa:bb:cc:dd:ee:06"
	macIPad        = "aa:bb:cc:dd:ee:07"
	macPlayStation = "aa:bb:cc:dd:ee:08"
	macThermostat  = "aa:bb:cc:dd:ee:09"
	macDoorbell    = "aa:bb:cc:dd:ee:0a"
	macRoomba      = "aa:bb:cc:dd:ee:0b"
	macDesktop     = "aa:bb:cc:dd:ee:0c"
	macGuestLaptop = "aa:bb:cc:dd:ee:0d"
	macHueBridge   = "aa:bb:cc:dd:ee:0e"
	macNAS         = "aa:bb:cc:dd:ee:0f"
	macPrinter     = "aa:bb:cc:dd:ee:10"
	macFireStick   = "aa:bb:cc:dd:ee:11"
	macSwitch      = "aa:bb:cc:dd:ee:12"
	macWyzeCam     = "aa:bb:cc:dd:ee:13"
)

// Seed fills an empty database with demo data; it does nothing once queries exist.
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

	for ip, label := range demoSourceLabels() {
		if err := db.SetSourceLabel(ip, label); err != nil {
			return err
		}
	}

	rng := rand.New(rand.NewSource(42))
	queries := baselineQueries(now, rng)
	queries = append(queries, spikeQueries(now, rng)...)
	queries = append(queries, recentQueries(now, rng)...)
	queries = append(queries, paginationShowcaseQueries(now)...)
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

	if err := db.SetDomainOverride("ntp.ubuntu.com", "uncategorized"); err != nil {
		return err
	}

	return nil
}

func demoDevices(now time.Time) []store.Device {
	return []store.Device{
		{
			MAC:       macRokuTV,
			IP:        "192.168.1.10",
			Hostname:  "roku-tv",
			Vendor:    "Roku",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-10 * time.Minute),
		},
		{
			MAC:       macPhone,
			IP:        "192.168.1.20",
			Hostname:  "wireless-phone",
			Vendor:    "Apple",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			MAC:       macEchoDot,
			IP:        "192.168.1.30",
			Hostname:  "echo-dot",
			Vendor:    "Amazon",
			FirstSeen: now.Add(-28 * 24 * time.Hour),
			LastSeen:  now.Add(-1 * time.Minute),
		},
		{
			MAC:       macWorkLaptop,
			IP:        "192.168.1.40",
			Hostname:  "work-laptop",
			Vendor:    "Apple",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-1 * time.Minute),
		},
		{
			MAC:       macNestHub,
			IP:        "192.168.1.50",
			Hostname:  "nest-hub",
			Vendor:    "Google",
			FirstSeen: now.Add(-25 * 24 * time.Hour),
			LastSeen:  now.Add(-3 * time.Minute),
		},
		{
			MAC:       macSamsungTV,
			IP:        "192.168.1.60",
			Hostname:  "samsung-tv",
			Vendor:    "Samsung",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-5 * time.Minute),
		},
		{
			MAC:       macIPad,
			IP:        "192.168.1.70",
			Hostname:  "ipad",
			Vendor:    "Apple",
			FirstSeen: now.Add(-20 * 24 * time.Hour),
			LastSeen:  now.Add(-12 * time.Minute),
		},
		{
			MAC:       macPlayStation,
			IP:        "192.168.1.80",
			Hostname:  "playstation-5",
			Vendor:    "Sony",
			FirstSeen: now.Add(-22 * 24 * time.Hour),
			LastSeen:  now.Add(-20 * time.Minute),
		},
		{
			MAC:       macThermostat,
			IP:        "192.168.1.90",
			Hostname:  "ecobee-thermostat",
			Vendor:    "Ecobee",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-4 * time.Minute),
		},
		{
			MAC:       macDoorbell,
			IP:        "192.168.1.100",
			Hostname:  "ring-doorbell",
			Vendor:    "Ring",
			FirstSeen: now.Add(-26 * 24 * time.Hour),
			LastSeen:  now.Add(-6 * time.Minute),
		},
		{
			MAC:       macRoomba,
			IP:        "192.168.1.110",
			Hostname:  "roomba",
			Vendor:    "iRobot",
			FirstSeen: now.Add(-18 * 24 * time.Hour),
			LastSeen:  now.Add(-15 * time.Minute),
		},
		{
			MAC:       macDesktop,
			IP:        "192.168.1.120",
			Hostname:  "desktop-tower",
			Vendor:    "Dell",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-3 * time.Minute),
		},
		{
			MAC:       macGuestLaptop,
			IP:        "192.168.1.130",
			Hostname:  "guest-laptop",
			Vendor:    "Lenovo",
			FirstSeen: now.Add(-5 * 24 * time.Hour),
			LastSeen:  now.Add(-2 * time.Minute),
		},
		{
			MAC:       macHueBridge,
			IP:        "192.168.1.140",
			Hostname:  "hue-bridge",
			Vendor:    "Signify",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-8 * time.Minute),
		},
		{
			MAC:       macNAS,
			IP:        "192.168.1.150",
			Hostname:  "synology-nas",
			Vendor:    "Synology",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-1 * time.Minute),
		},
		{
			MAC:       macPrinter,
			IP:        "192.168.1.160",
			Hostname:  "hp-printer",
			Vendor:    "HP",
			FirstSeen: now.Add(-30 * 24 * time.Hour),
			LastSeen:  now.Add(-25 * time.Minute),
		},
		{
			MAC:       macFireStick,
			IP:        "192.168.1.170",
			Hostname:  "fire-stick",
			Vendor:    "Amazon",
			FirstSeen: now.Add(-14 * 24 * time.Hour),
			LastSeen:  now.Add(-45 * time.Minute),
		},
		{
			MAC:       macSwitch,
			IP:        "192.168.1.180",
			Hostname:  "nintendo-switch",
			Vendor:    "Nintendo",
			FirstSeen: now.Add(-10 * 24 * time.Hour),
			LastSeen:  now.Add(-4 * time.Hour),
		},
		{
			MAC:       macWyzeCam,
			IP:        "192.168.1.190",
			Hostname:  "wyze-cam",
			Vendor:    "Wyze",
			FirstSeen: now.Add(-24 * 24 * time.Hour),
			LastSeen:  now.Add(-7 * time.Minute),
		},
	}
}

func demoLabels() map[string]string {
	return map[string]string{
		macRokuTV:      "Living Room TV",
		macPhone:       "Main Phone",
		macEchoDot:     "Kitchen Echo",
		macWorkLaptop:  "Work Laptop",
		macNestHub:     "Living Room Hub",
		macSamsungTV:   "Bedroom TV",
		macIPad:        "Tablet",
		macPlayStation: "Gaming Console",
		macThermostat:  "Thermostat",
		macDoorbell:    "Front Door Cam",
		macRoomba:      "Robot Vacuum",
		macDesktop:     "Desktop PC",
		macGuestLaptop: "Guest Laptop",
		macHueBridge:   "Smart Lights",
		macNAS:         "NAS",
		macPrinter:     "Office Printer",
		macFireStick:   "Fire Stick",
		macSwitch:      "Switch",
		macWyzeCam:     "Garage Cam",
	}
}

func demoSourceLabels() map[string]string {
	return map[string]string{
		"10.0.0.50":  "Wireguard Tunnel",
		"10.0.0.100": "Docker Host",
	}
}

type deviceTraffic struct {
	mac     string
	queries []queryPattern
}

type queryPattern struct {
	domain   string
	category string
	weight   int // relative frequency per day
}

func trafficPatterns() []deviceTraffic {
	return []deviceTraffic{
		{mac: macRokuTV, queries: []queryPattern{
			{"captive.roku.com", "", 12},
			{"scribe.logs.roku.com", "tracking", 10},
			{"ads.roku.com", "advertising", 8},
			{"cooper.logs.roku.com", "tracking", 6},
			{"cloudservices.roku.com", "", 8},
			{"content-delivery.roku.com", "", 15},
			{"image.roku.com", "", 10},
			{"netflix.com", "", 20},
			{"api.netflix.com", "", 8},
			{"ichnaea.netflix.com", "analytics", 4},
			{"assets.nflxext.com", "", 6},
			{"nrdp.prod.cloud.netflix.com", "", 5},
			{"api.pluto.tv", "", 4},
			{"stitcher-ipv4.pluto.tv", "", 6},
			{"siloh.pluto.tv", "analytics", 3},
		}},
		{mac: macPhone, queries: []queryPattern{
			{"api.apple.com", "", 15},
			{"icloud-content.com", "", 12},
			{"gsp-ssl.ls.apple.com", "", 6},
			{"weather.apple.com", "", 8},
			{"news-events.apple.com", "", 4},
			{"gateway.icloud.com", "", 10},
			{"mask.icloud.com", "", 6},
			{"mask-h2.icloud.com", "", 4},
			{"app-measurement.com", "analytics", 8},
			{"app.adjust.com", "tracking", 5},
			{"cdn.syndication.twimg.com", "", 10},
			{"api.twitter.com", "", 6},
			{"graph.instagram.com", "", 8},
			{"i.instagram.com", "", 12},
			{"edge-chat.instagram.com", "", 5},
			{"mqtt-mini.facebook.com", "", 4},
			{"api.spotify.com", "", 6},
			{"spclient.wg.spotify.com", "", 10},
			{"audio-ak-spotify-com.akamaized.net", "", 8},
			{"crashlyticsreports-pa.googleapis.com", "analytics", 3},
			{"firebaseinstallations.googleapis.com", "analytics", 4},
			{"api.branch.io", "tracking", 3},
			{"cdn.branch.io", "tracking", 2},
			{"api2.amplitude.com", "analytics", 3},
		}},
		{mac: macEchoDot, queries: []queryPattern{
			{"alexa.amazon.com", "", 15},
			{"avs-alexa-14-na.amazon.com", "", 10},
			{"device-metrics-us.amazon.com", "analytics", 8},
			{"unagi-na.amazon.com", "telemetry", 6},
			{"music.amazon.com", "", 12},
			{"mads-adz.amazon.com", "advertising", 4},
			{"fls-na.amazon.com", "tracking", 5},
			{"ntp-g7g.amazon.com", "", 3},
			{"api.amazonalexa.com", "", 8},
			{"dp-gw-na.amazon.com", "", 6},
			{"todo.amazon.com", "", 3},
			{"dcape-na.amazon.com", "telemetry", 4},
		}},
		{mac: macWorkLaptop, queries: []queryPattern{
			{"github.com", "", 25},
			{"api.github.com", "", 15},
			{"copilot-proxy.githubusercontent.com", "", 10},
			{"raw.githubusercontent.com", "", 5},
			{"objects.githubusercontent.com", "", 8},
			{"alive.github.com", "", 4},
			{"collector.github.com", "analytics", 6},
			{"sentry.io", "analytics", 5},
			{"o12345.ingest.sentry.io", "analytics", 3},
			{"segment.io", "analytics", 4},
			{"cdn.segment.com", "analytics", 3},
			{"registry.npmjs.org", "", 8},
			{"fonts.googleapis.com", "", 6},
			{"fonts.gstatic.com", "", 6},
			{"www.google.com", "", 10},
			{"www.googleapis.com", "", 5},
			{"accounts.google.com", "", 4},
			{"lh3.googleusercontent.com", "", 3},
			{"translate.googleapis.com", "", 2},
			{"ntp.ubuntu.com", "", 3},
			{"security.ubuntu.com", "", 2},
			{"dl.google.com", "", 2},
			{"updates.signal.org", "", 2},
			{"chat.signal.org", "", 4},
			{"storage.signal.org", "", 2},
			{"cdn2.unrealengine.com", "", 2},
			{"stackoverflow.com", "", 8},
			{"cdn.sstatic.net", "", 5},
		}},
		{mac: macNestHub, queries: []queryPattern{
			{"clients4.google.com", "", 10},
			{"play.googleapis.com", "", 6},
			{"connectivitycheck.gstatic.com", "", 12},
			{"firebaselogging-pa.googleapis.com", "analytics", 6},
			{"adservice.google.com", "advertising", 4},
			{"pagead2.googlesyndication.com", "advertising", 3},
			{"www.googleadservices.com", "advertising", 3},
			{"time.google.com", "", 4},
			{"clients2.google.com", "", 5},
			{"lh3.googleusercontent.com", "", 4},
			{"photos.googleapis.com", "", 3},
			{"home.nest.com", "", 8},
			{"grpc-web.production.nest.com", "", 5},
		}},
		{mac: macSamsungTV, queries: []queryPattern{
			{"samsungcloudsolution.com", "", 8},
			{"cdn.samsungcloudsolution.com", "", 6},
			{"samsung-ads.com", "advertising", 10},
			{"config.samsungads.com", "advertising", 8},
			{"gpm.samsungqbe.com", "telemetry", 6},
			{"log-config.samsungacr.com", "tracking", 8},
			{"samsungacr.com", "tracking", 4},
			{"us-api.samsungacr.com", "tracking", 5},
			{"auth.samsungosp.com", "", 3},
			{"multiscreen.samsung.com", "", 4},
			{"api.samsungapps.com", "", 3},
			{"osb-apps.samsungqbe.com", "", 5},
			{"d1oxlq5h9komer.cloudfront.net", "", 8},
			{"api.hulu.com", "", 4},
			{"play.hulu.com", "", 6},
			{"ads-e-darwin.hulustream.com", "advertising", 3},
		}},
		{mac: macIPad, queries: []queryPattern{
			{"api.apple.com", "", 8},
			{"identity.apple.com", "", 4},
			{"icloud.com", "", 6},
			{"app-measurement.com", "analytics", 4},
			{"www.youtube.com", "", 10},
			{"i.ytimg.com", "", 8},
			{"yt3.ggpht.com", "", 5},
			{"www.google.com", "", 6},
			{"fonts.googleapis.com", "", 3},
			{"static.xx.fbcdn.net", "", 5},
			{"web.facebook.com", "", 3},
			{"pixel.facebook.com", "tracking", 2},
			{"connect.facebook.net", "tracking", 3},
		}},
		{mac: macPlayStation, queries: []queryPattern{
			{"ps5.np.playstation.net", "", 12},
			{"telemetry.api.playstation.com", "telemetry", 6},
			{"store.playstation.com", "", 8},
			{"gs-sec.ww.np.dl.playstation.net", "", 10},
			{"web.np.playstation.com", "", 5},
			{"livearea.np.dl.playstation.net", "", 4},
			{"image.api.playstation.com", "", 6},
			{"activity.api.np.km.playstation.net", "", 3},
			{"accounts.api.playstation.com", "", 4},
			{"trophy.api.np.km.playstation.net", "", 3},
		}},
		{mac: macThermostat, queries: []queryPattern{
			{"api.ecobee.com", "", 15},
			{"mqtt.ecobee.com", "", 20},
			{"weather.ecobee.com", "", 10},
			{"time.google.com", "", 4},
		}},
		{mac: macDoorbell, queries: []queryPattern{
			{"fw.ring.com", "", 8},
			{"app-analytics.ring.com", "analytics", 6},
			{"ntp-g7g.amazon.com", "", 3},
			{"prod.api.ring.com", "", 12},
			{"ring-account.ring.com", "", 4},
			{"snaps.ring.com", "", 10},
			{"oauth.ring.com", "", 3},
			{"prd-storage-mms.ring.com", "", 8},
		}},
		{mac: macRoomba, queries: []queryPattern{
			{"disc-prod.iot.irobotapi.com", "", 6},
			{"data.iot.irobotapi.com", "telemetry", 4},
			{"api.irobot.com", "", 5},
			{"global.iot.irobotapi.com", "", 3},
			{"ntp.ubuntu.com", "", 2},
		}},
		{mac: macDesktop, queries: []queryPattern{
			{"github.com", "", 12},
			{"api.github.com", "", 8},
			{"cdn.jsdelivr.net", "", 6},
			{"unpkg.com", "", 4},
			{"steamcommunity.com", "", 5},
			{"store.steampowered.com", "", 6},
			{"cdn.cloudflare.steamstatic.com", "", 8},
			{"steamcdn-a.akamaihd.net", "", 5},
			{"tracking.epicgames.com", "tracking", 3},
			{"launcher-public-service-prod06.ol.epicgames.com", "", 4},
			{"sentry.io", "analytics", 3},
			{"www.google.com", "", 8},
			{"www.reddit.com", "", 6},
			{"i.redd.it", "", 4},
			{"oauth.reddit.com", "", 3},
			{"ntp.ubuntu.com", "", 2},
			{"fonts.googleapis.com", "", 4},
			{"dns.nextdns.io", "", 2},
		}},
		{mac: macHueBridge, queries: []queryPattern{
			{"diagnostics.meethue.com", "telemetry", 4},
			{"discovery.meethue.com", "", 3},
			{"firmware.meethue.com", "", 2},
			{"ws.meethue.com", "", 6},
			{"time.google.com", "", 3},
		}},
		{mac: macNAS, queries: []queryPattern{
			{"update.synology.com", "", 4},
			{"checkip.synology.com", "", 6},
			{"ddns.synology.com", "", 3},
			{"global.quickconnect.to", "", 4},
			{"pkgautoinstall.synology.com", "", 2},
			{"ntp.ubuntu.com", "", 3},
			{"plex.tv", "", 5},
			{"metadata.provider.plex.tv", "", 4},
			{"analytics.plex.tv", "analytics", 3},
			{"my.plexapp.com", "", 3},
			{"pubsub.plex.tv", "", 4},
		}},
		{mac: macPrinter, queries: []queryPattern{
			{"hp-updates.hpcloud.com", "", 3},
			{"devmon.hpconnected.com", "telemetry", 2},
			{"print-analytics.hpconnected.com", "analytics", 2},
			{"ntp.ubuntu.com", "", 1},
		}},
		{mac: macFireStick, queries: []queryPattern{
			{"device-metrics-us.amazon.com", "analytics", 6},
			{"fls-na.amazon.com", "tracking", 5},
			{"mads-adz.amazon.com", "advertising", 4},
			{"unagi-na.amazon.com", "telemetry", 4},
			{"api.amazon.com", "", 8},
			{"atv-ps.amazon.com", "", 6},
			{"aiv-cdn.net", "", 10},
			{"images-na.ssl-images-amazon.com", "", 5},
			{"netflix.com", "", 8},
			{"api.netflix.com", "", 4},
			{"assets.nflxext.com", "", 3},
			{"disney.api.edge.bamgrid.com", "", 5},
			{"prod.bamgrid.com", "", 3},
		}},
		{mac: macSwitch, queries: []queryPattern{
			{"conntest.nintendowifi.net", "", 3},
			{"ctest.cdn.nintendo.net", "", 2},
			{"dauth-lp1.ndas.srv.nintendo.net", "", 3},
			{"sun.hac.lp1.d4c.nintendo.net", "", 2},
			{"atum-eda.hac.lp1.d4c.nintendo.net", "", 4},
			{"receive-lp1.er.srv.nintendo.net", "telemetry", 2},
			{"bcat-topics-lp1.cdn.nintendo.net", "", 3},
		}},
		{mac: macWyzeCam, queries: []queryPattern{
			{"api.wyzecam.com", "", 8},
			{"wyze-mars-service.wyzecam.com", "", 6},
			{"wyze-general-api.wyzecam.com", "", 4},
			{"log.wyzecam.com", "telemetry", 4},
			{"app-measurement.com", "analytics", 3},
			{"firebaseinstallations.googleapis.com", "analytics", 2},
			{"ntp.ubuntu.com", "", 2},
		}},
	}
}

func baselineQueries(now time.Time, rng *rand.Rand) []store.Query {
	patterns := trafficPatterns()
	var out []store.Query

	for day := 30; day >= 2; day-- {
		dayStart := now.Add(-time.Duration(day) * 24 * time.Hour)
		for _, device := range patterns {
			minuteOffset := 0
			for _, q := range device.queries {
				reps := q.weight
				if isWeekend(dayStart) && isEntertainmentDevice(device.mac) {
					reps = reps * 3 / 2
				}
				for rep := 0; rep < reps; rep++ {
					hour := (minuteOffset / 60) % 24
					if !isActiveHour(device.mac, hour) && rng.Intn(3) < 2 {
						minuteOffset += 30 + rng.Intn(60)
						continue
					}
					jitter := time.Duration(rng.Intn(40)) * time.Minute
					ts := dayStart.Add(time.Duration(minuteOffset)*time.Minute + jitter)
					out = append(out, store.Query{
						DeviceMAC: device.mac,
						Domain:    q.domain,
						QueryType: queryTypes[rng.Intn(len(queryTypes))],
						Category:  q.category,
						Timestamp: ts,
					})
					minuteOffset += 8 + rng.Intn(20)
					if minuteOffset > 1380 {
						minuteOffset = rng.Intn(120)
					}
				}
			}
		}
	}
	return out
}

func isWeekend(t time.Time) bool {
	wd := t.Weekday()
	return wd == time.Saturday || wd == time.Sunday
}

func isEntertainmentDevice(mac string) bool {
	switch mac {
	case macRokuTV,
		macSamsungTV,
		macPlayStation,
		macFireStick,
		macSwitch:
		return true
	}
	return false
}

func isActiveHour(mac string, hour int) bool {
	switch mac {
	case macWorkLaptop:
		return hour >= 8 && hour <= 19
	case macRokuTV, macSamsungTV, macFireStick:
		return hour >= 17 || hour <= 1
	case macPlayStation, macSwitch:
		return hour >= 14 || hour <= 2
	case macThermostat, macHueBridge, macNAS, macPrinter:
		return true
	default:
		return true
	}
}

var queryTypes = []string{"A", "AAAA", "A", "A"} // bias toward A records

// spikeQueries trips the anomaly detectors in the current 24h.
func spikeQueries(now time.Time, rng *rand.Rand) []store.Query {
	var out []store.Query

	// Roku tracker spike
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
		{"pixel.adsafeprotected.com", "tracking"},
		{"ib.adnxs.com", "advertising"},
	}
	base := now.Add(-8 * time.Hour)
	for i := 0; i < 35; i++ {
		sd := rokuSpikeDomains[i%len(rokuSpikeDomains)]
		out = append(out, store.Query{
			DeviceMAC: macRokuTV,
			Domain:    sd.domain,
			QueryType: queryTypes[rng.Intn(len(queryTypes))],
			Category:  sd.category,
			Timestamp: base.Add(time.Duration(i)*4*time.Minute + time.Duration(rng.Intn(3))*time.Minute),
		})
	}

	// Samsung TV volume spike: over 3x its daily average
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
		{"us-api.samsungacr.com", "tracking"},
		{"osb-apps.samsungqbe.com", ""},
	}
	samsungBase := now.Add(-6 * time.Hour)
	for i := 0; i < 90; i++ {
		sd := samsungSpikeDomains[i%len(samsungSpikeDomains)]
		out = append(out, store.Query{
			DeviceMAC: macSamsungTV,
			Domain:    sd.domain,
			QueryType: queryTypes[rng.Intn(len(queryTypes))],
			Category:  sd.category,
			Timestamp: samsungBase.Add(time.Duration(i)*2*time.Minute + time.Duration(rng.Intn(2))*time.Minute),
		})
	}

	return out
}

// recentQueries adds current-day activity so the 24h view has data.
func recentQueries(now time.Time, rng *rand.Rand) []store.Query {
	patterns := trafficPatterns()
	var out []store.Query

	dayStart := now.Add(-20 * time.Hour)
	for _, device := range patterns {
		minuteOffset := 0
		for _, q := range device.queries {
			reps := q.weight * 3
			for rep := 0; rep < reps; rep++ {
				jitter := time.Duration(rng.Intn(25)) * time.Minute
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
				minuteOffset += 5 + rng.Intn(15)
				if minuteOffset > 1200 {
					minuteOffset = rng.Intn(60)
				}
			}
		}
	}

	return out
}

func paginationShowcaseQueries(now time.Time) []store.Query {

	out := make([]store.Query, 0, 24)
	base := now.Add(-90 * time.Minute)
	for i := 1; i <= 24; i++ {
		out = append(out, store.Query{
			DeviceMAC: macWorkLaptop,
			Domain:    fmt.Sprintf("workbench-%02d.demo.umberrelay.local", i),
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(time.Duration(i) * time.Minute),
		})
	}
	return out
}

func sourceQueries(now time.Time, rng *rand.Rand) []store.Query {
	type sourcePattern struct {
		ip      string
		queries []queryPattern
	}

	sources := []sourcePattern{
		{ip: "10.0.0.50", queries: []queryPattern{
			{"vpn-gateway.example.net", "", 4},
			{"api.protonvpn.ch", "", 3},
			{"dns.quad9.net", "", 5},
			{"api.mullvad.net", "", 2},
		}},
		{ip: "10.0.0.75", queries: []queryPattern{
			{"printer.local", "", 3},
			{"hp-updates.hpcloud.com", "", 2},
		}},
		{ip: "10.0.0.100", queries: []queryPattern{
			{"registry.docker.io", "", 5},
			{"auth.docker.io", "", 3},
			{"production.cloudflare.docker.com", "", 4},
			{"ghcr.io", "", 3},
			{"ntp.ubuntu.com", "", 2},
		}},
		{ip: "172.16.0.5", queries: []queryPattern{
			{"releases.ubuntu.com", "", 2},
			{"archive.ubuntu.com", "", 3},
			{"security.ubuntu.com", "", 2},
		}},
	}

	var out []store.Query
	for day := 30; day >= 0; day-- {
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
					minuteOffset += 20 + rng.Intn(40)
				}
			}
		}
	}

	return out
}

func bypassCandidateQueries(now time.Time) []store.Query {
	var out []store.Query

	for day := 5; day >= 1; day-- {
		base := now.Add(-time.Duration(day) * 24 * time.Hour)
		out = append(out, store.Query{
			DeviceMAC: macGuestLaptop,
			Domain:    "dns.google",
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(2 * time.Hour),
		})
		out = append(out, store.Query{
			DeviceMAC: macGuestLaptop,
			Domain:    "example.com",
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(4 * time.Hour),
		})
		out = append(out, store.Query{
			DeviceMAC: macGuestLaptop,
			Domain:    "www.google.com",
			QueryType: "A",
			Category:  "",
			Timestamp: base.Add(6 * time.Hour),
		})
	}

	out = append(out, store.Query{
		DeviceMAC: macGuestLaptop,
		Domain:    "dns.google",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-3 * time.Hour),
	})
	out = append(out, store.Query{
		DeviceMAC: macGuestLaptop,
		Domain:    "clients.l.google.com",
		QueryType: "A",
		Category:  "",
		Timestamp: now.Add(-2 * time.Hour),
	})

	return out
}

func seedLists(db *store.DB) error {
	trackingID, err := db.AddList("https://demo.umberrelay.local/lists/tracking.txt", "Tracker Blocklist", "tracking")
	if err != nil {
		return err
	}
	if err := db.WriteListDomains(trackingID, map[string]string{
		"scribe.logs.roku.com":          "tracking",
		"cooper.logs.roku.com":          "tracking",
		"ads.roku.com":                  "advertising",
		"doubleclick.net":               "advertising",
		"ads.example-cdn.net":           "advertising",
		"tracker.example.com":           "tracking",
		"pagead2.googlesyndication.com": "advertising",
		"adsserver.example.net":         "advertising",
		"app.adjust.com":                "tracking",
		"app-measurement.com":           "analytics",
		"samsung-ads.com":               "advertising",
		"config.samsungads.com":         "advertising",
		"log-config.samsungacr.com":     "tracking",
		"samsungacr.com":                "tracking",
		"us-api.samsungacr.com":         "tracking",
		"tracking.epicgames.com":        "tracking",
		"adservice.google.com":          "advertising",
		"mads-adz.amazon.com":           "advertising",
		"pixel.adsafeprotected.com":     "tracking",
		"ib.adnxs.com":                  "advertising",
		"www.googleadservices.com":      "advertising",
		"api.branch.io":                 "tracking",
		"cdn.branch.io":                 "tracking",
		"pixel.facebook.com":            "tracking",
		"connect.facebook.net":          "tracking",
		"ads-e-darwin.hulustream.com":   "advertising",
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
		"dcape-na.amazon.com":                  "telemetry",
		"sentry.io":                            "analytics",
		"o12345.ingest.sentry.io":              "analytics",
		"segment.io":                           "analytics",
		"cdn.segment.com":                      "analytics",
		"firebaselogging-pa.googleapis.com":    "analytics",
		"firebaseinstallations.googleapis.com": "analytics",
		"crashlyticsreports-pa.googleapis.com": "analytics",
		"api2.amplitude.com":                   "analytics",
		"app-analytics.ring.com":               "analytics",
		"data.iot.irobotapi.com":               "telemetry",
		"gpm.samsungqbe.com":                   "telemetry",
		"telemetry.api.playstation.com":        "telemetry",
		"diagnostics.meethue.com":              "telemetry",
		"log.wyzecam.com":                      "telemetry",
		"ichnaea.netflix.com":                  "analytics",
		"siloh.pluto.tv":                       "analytics",
		"collector.github.com":                 "analytics",
		"devmon.hpconnected.com":               "telemetry",
		"print-analytics.hpconnected.com":      "analytics",
		"analytics.plex.tv":                    "analytics",
		"receive-lp1.er.srv.nintendo.net":      "telemetry",
	}); err != nil {
		return err
	}
	if err := db.UpdateListFetchTime(telemetryID); err != nil {
		return err
	}

	return nil
}
