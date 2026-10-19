package web

import (
	"fmt"
	"math"
	"strconv"

	"umberrelay/internal/category"
	"umberrelay/internal/store"
)

// listMatchOrder puts the categories that count toward the tracker rate first.
var listMatchOrder = []string{category.Tracking, category.Advertising, category.Malware, category.Analytics, category.Telemetry}

type listMatchRow struct {
	Category string
	Tone     string
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
			Tone:     categoryTone(name),
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

func formatPoints(v float64) string {
	return strconv.FormatFloat(math.Round(v*10)/10, 'f', -1, 64)
}

func trackerBadge(t store.Trend) string {
	if !t.HasPrior {
		return ""
	}
	sign := ""
	if math.Round(t.Change*10) > 0 {
		sign = "+"
	}
	return sign + formatPoints(t.Change) + " vs prior 7d"
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
	flag := actorFlag{
		Tone:        "warn",
		Badge:       anomalyBadge(a),
		Explanation: anomalyExplanation(a),
		DeviceMAC:   a.DeviceMAC,
		DeviceName:  a.DeviceName,
	}
	switch a.Type {
	case "tracker_spike":
		flag.Word, flag.Tone, flag.priority = "tracker spike", "error", 3
	case "dns_bypass_likely":
		flag.Word, flag.priority = "bypass likely", 2
	case "dns_bypass_suspected":
		flag.Word, flag.priority = "bypass suspected", 2
	default:
		flag.Word, flag.priority = "volume spike", 1
	}
	return flag
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
