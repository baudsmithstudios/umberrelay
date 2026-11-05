package web

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"umberrelay/internal/category"
	"umberrelay/internal/dns"
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

// categoryTonesJSON gives the browser-built live feed the server's category tones.
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
	ActorKey    string
	Name        string
	priority    int
}

func flagFor(a store.Anomaly) actorFlag {
	flag := actorFlag{Tone: "warn", ActorKey: actorKeyForDevice(a.DeviceMAC), Name: a.DeviceName}
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

// Durations, not clock times: server-rendered times are UTC in the stock container.
func rateLimitFlag(name, actorKey string, r dns.Refusal, now time.Time) actorFlag {
	minutes := func(d time.Duration) string { return formatMinutes(max(1, int(d.Minutes()))) }
	flag := actorFlag{
		Word:     "rate limited",
		Tone:     "error",
		Badge:    fmt.Sprintf("%d refused", r.Count),
		ActorKey: actorKey,
		Name:     name,
		priority: 4,
	}
	if now.Sub(r.Last) < time.Minute {
		flag.Explanation = fmt.Sprintf("%s is sending more than %d DNS queries a second; %d refused over the last %s.", name, dns.QueriesPerSecondPerSource, r.Count, minutes(now.Sub(r.First)))
	} else {
		flag.Explanation = fmt.Sprintf("%s sent more than %d DNS queries a second; %d refused over %s, ending %s ago.", name, dns.QueriesPerSecondPerSource, r.Count, minutes(r.Last.Sub(r.First)), minutes(now.Sub(r.Last)))
	}
	return flag
}

func silentFor(minutes float64) string {
	return formatMinutes(int(minutes)) + " silent"
}

func formatMinutes(m int) string {
	if m < 60 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dh %dm", m/60, m%60)
}

func flagsByActor(flags []actorFlag) map[string]actorFlag {
	out := make(map[string]actorFlag, len(flags))
	for _, flag := range flags {
		if existing, ok := out[flag.ActorKey]; ok && existing.priority >= flag.priority {
			continue
		}
		out[flag.ActorKey] = flag
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

// domainTable shows Reach only for the network's top domains.
type domainTable struct {
	Label     string
	ShowReach bool
	Rows      []privacyDomainRow
}

func (p networkPane) DomainTable() domainTable {
	return domainTable{Label: "Top domains", ShowReach: true, Rows: p.TopDomains}
}

func (d privacyDetail) DomainTable() domainTable {
	return domainTable{Label: "Domains", Rows: d.Domains}
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
		note, hot := rosterNote(flags, key, trackerTrendText(d.TrackerTrend))
		r.Devices = append(r.Devices, rosterRow{
			ActorKey:       key,
			Href:           "/devices/" + key,
			Name:           deviceDisplayName(d.Device),
			Meta:           joinMeta(d.Vendor, fmt.Sprintf("%d q", d.QueryCount), note),
			TrackerPercent: d.TrackerPercent,
			QueryCount:     d.QueryCount,
			Hot:            hot,
			Selected:       key == selectedKey,
		})
	}
	for _, src := range sources {
		key := actorKeyForSource(src.SourceIP)
		note, hot := rosterNote(flags, key, trackerTrendText(src.TrackerTrend))
		r.Sources = append(r.Sources, rosterRow{
			ActorKey:       key,
			Href:           "/devices/" + key,
			Name:           sourceActorDisplayName(src.SourceIP, src.Label),
			Meta:           joinMeta(src.SourceIP, fmt.Sprintf("%d q", src.QueryCount), note),
			TrackerPercent: src.TrackerPercent,
			QueryCount:     src.QueryCount,
			Hot:            hot,
			Selected:       key == selectedKey,
		})
	}
	sortRosterRows(r.Devices)
	sortRosterRows(r.Sources)
	return r
}

func rosterNote(flags map[string]actorFlag, actorKey, fallback string) (note string, hot bool) {
	flag, ok := flags[actorKey]
	if !ok {
		return fallback, false
	}
	return flag.Word, flag.Tone == "error"
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
