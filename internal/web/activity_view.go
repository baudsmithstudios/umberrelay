package web

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"umberrelay/internal/category"
	"umberrelay/internal/store"
)

// listMatchOrder puts the categories that count toward the tracker rate first.
var listMatchOrder = []string{category.Tracking, category.Advertising, category.Malware, category.Analytics, category.Telemetry}

type listMatchRow struct {
	Category string
	Count    int
	Share    string
	Delta    string
}

type listMatches struct {
	Rows      []listMatchRow
	Unmatched int
}

func makeListMatches(counts []store.CategoryWindowCount) listMatches {
	byCategory := make(map[string]store.CategoryWindowCount, len(counts))
	var currentTotal, priorTotal int
	for _, c := range counts {
		byCategory[c.Category] = c
		currentTotal += c.Current
		priorTotal += c.Prior
	}

	matches := listMatches{Unmatched: byCategory[""].Current + byCategory[category.Uncategorized].Current}
	for _, name := range listMatchOrder {
		c := byCategory[name]
		if c.Current == 0 {
			continue
		}
		share := float64(c.Current) / float64(currentTotal) * 100
		matches.Rows = append(matches.Rows, listMatchRow{
			Category: name,
			Count:    c.Current,
			Share:    fmt.Sprintf("%.1f%%", share),
			Delta:    shareDelta(share, c.Prior, priorTotal),
		})
	}
	return matches
}

func shareDelta(currentShare float64, prior, priorTotal int) string {
	if priorTotal == 0 {
		return "─"
	}
	delta := currentShare - float64(prior)/float64(priorTotal)*100
	switch {
	case math.Abs(delta) < 0.05:
		return "─"
	case delta > 0:
		return "▲ " + formatPoints(delta)
	default:
		return "▼ " + formatPoints(-delta)
	}
}

func categoryTone(name string) string {
	switch name {
	case category.Tracking, category.Malware:
		return "error"
	case category.Advertising:
		return "warn"
	case category.Analytics, category.Telemetry:
		return "success"
	default:
		return ""
	}
}

// categoryTonesJSON gives the live feed, which builds its rows in the browser, the same tones as the server-rendered pages.
func categoryTonesJSON() string {
	tones := make(map[string]string)
	for _, option := range category.Options() {
		tones[option.Value] = categoryTone(option.Value)
	}
	// Marshalling a map of strings cannot fail.
	b, _ := json.Marshal(tones)
	return string(b)
}

func formatPoints(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

type actorFlag struct {
	Word        string
	Tone        string
	Badge       string
	Explanation string
	DeviceMAC   string
	DeviceName  string
	priority    int
}

func flagFor(a store.Anomaly) actorFlag {
	flag := actorFlag{Tone: "warn", DeviceMAC: a.DeviceMAC, DeviceName: a.DeviceName}
	switch a.Type {
	case "tracker_spike":
		flag.Word, flag.Tone, flag.priority = "tracker spike", "error", 3
		flag.Badge = fmt.Sprintf("+%.0f pts", a.Delta)
		flag.Explanation = fmt.Sprintf("%s jumped above its usual tracker rate, led by %s from %s.", a.DeviceName, a.TopDomain, a.TopDomainSourceList)
	case "dns_bypass_likely":
		flag.Word, flag.priority = "bypass likely", 2
		flag.Badge = silentFor(a.Delta)
		flag.Explanation = fmt.Sprintf("%s is present on the LAN but has stopped using local DNS. Earlier queries included %s, which suggests encrypted DNS usage.", a.DeviceName, a.TopDomain)
	case "dns_bypass_suspected":
		flag.Word, flag.priority = "bypass suspected", 2
		flag.Badge = silentFor(a.Delta)
		flag.Explanation = fmt.Sprintf("%s is present on the LAN but has stopped using local DNS despite prior DNS activity, so visibility may be bypassed.", a.DeviceName)
	default:
		flag.Word, flag.priority = "volume spike", 1
		flag.Badge = fmt.Sprintf("%.1fx daily avg", a.CurrentValue/a.AverageValue)
		flag.Explanation = fmt.Sprintf("%s is making far more requests than usual, with %s contributing the largest spike from %s.", a.DeviceName, a.TopDomain, a.TopDomainSourceList)
	}
	return flag
}

// silentFor names how long a device has gone without local DNS, from its silent minutes.
func silentFor(minutes float64) string {
	m := int(minutes)
	if m < 60 {
		return fmt.Sprintf("%dm silent", m)
	}
	return fmt.Sprintf("%dh %dm silent", m/60, m%60)
}

func flagsByMAC(anomalies []store.Anomaly) map[string]actorFlag {
	out := make(map[string]actorFlag, len(anomalies))
	for _, a := range anomalies {
		flag := flagFor(a)
		if existing, ok := out[a.DeviceMAC]; ok && existing.priority >= flag.priority {
			continue
		}
		out[a.DeviceMAC] = flag
	}
	return out
}

type networkPane struct {
	Stats         store.DashboardStats
	ListMatches   listMatches
	TrackerChange string
	Flags         []actorFlag
	TopDomains    []privacyDomainRow
}

type activityView struct {
	pageData
	Roster  roster
	Tab     string
	Network *networkPane
	Detail  *privacyDetail
}

type rosterRow struct {
	ActorKey       string
	Href           string
	Name           string
	Meta           string
	TrackerPercent float64
	QueryCount     int
	Hot            bool
	Selected       bool
}

type roster struct {
	Network rosterRow
	Devices []rosterRow
	Sources []rosterRow
}

func makeRoster(devices []store.DeviceWithTrends, sources []store.SourceWithTrends, stats store.DashboardStats, flags map[string]actorFlag, selectedKey string) roster {
	r := roster{Network: rosterRow{
		Href:           "/",
		Name:           "Whole network",
		Meta:           fmt.Sprintf("%d devices · %d queries", stats.DeviceCount, stats.TotalQueries),
		TrackerPercent: stats.TrackerPercent,
		QueryCount:     stats.TotalQueries,
		Selected:       selectedKey == "",
	}}
	for _, d := range devices {
		key := actorKeyForDevice(d.MAC)
		flag, flagged := flags[d.MAC]
		note := trackerTrendText(d.TrackerTrend)
		if flagged {
			note = flag.Word
		}
		r.Devices = append(r.Devices, rosterRow{
			ActorKey:       key,
			Href:           "/devices/" + key,
			Name:           deviceDisplayName(d.Device),
			Meta:           joinMeta(d.Vendor, fmt.Sprintf("%d q", d.QueryCount), note),
			TrackerPercent: d.TrackerPercent,
			QueryCount:     d.QueryCount,
			Hot:            flagged && flag.Tone == "error",
			Selected:       key == selectedKey,
		})
	}
	for _, src := range sources {
		key := actorKeyForSource(src.SourceIP)
		r.Sources = append(r.Sources, rosterRow{
			ActorKey:       key,
			Href:           "/devices/" + key,
			Name:           sourceActorDisplayName(src.SourceIP, src.Label),
			Meta:           joinMeta(src.SourceIP, fmt.Sprintf("%d q", src.QueryCount), trackerTrendText(src.TrackerTrend)),
			TrackerPercent: src.TrackerPercent,
			QueryCount:     src.QueryCount,
			Selected:       key == selectedKey,
		})
	}
	sortRosterRows(r.Devices)
	sortRosterRows(r.Sources)
	return r
}

func sortRosterRows(rows []rosterRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TrackerPercent == rows[j].TrackerPercent {
			return rows[i].Name < rows[j].Name
		}
		return rows[i].TrackerPercent > rows[j].TrackerPercent
	})
}

func joinMeta(parts ...string) string {
	kept := parts[:0]
	for _, part := range parts {
		if part != "" {
			kept = append(kept, part)
		}
	}
	return strings.Join(kept, " · ")
}

func parseTab(raw string) string {
	switch raw {
	case "domains", "live":
		return raw
	default:
		return "overview"
	}
}

// identificationNote explains which discovery signals named a device.
func identificationNote(d store.Device) string {
	parts := []string{"MAC from the ARP table."}
	if d.Hostname != "" {
		parts = append(parts, "Hostname "+d.Hostname+" from DHCP.")
	}
	if d.Vendor != "" {
		parts = append(parts, "Vendor "+d.Vendor+" from the MAC prefix.")
	}
	return strings.Join(parts, " ")
}
