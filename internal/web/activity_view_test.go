package web

import (
	"testing"

	"umberrelay/internal/store"
)

func TestMakeListMatches(t *testing.T) {
	got := makeListMatches([]store.CategoryWindowCount{
		{Category: "", Current: 3208, Prior: 2000},
		{Category: "analytics", Current: 267, Prior: 200},
		{Category: "tracking", Current: 404, Prior: 250},
		{Category: "uncategorized", Current: 0, Prior: 50},
		{Category: "telemetry", Current: 0, Prior: 0},
	})

	if len(got.Rows) != 2 {
		t.Fatalf("rows = %+v, want tracking and analytics", got.Rows)
	}
	tracking := got.Rows[0]
	want := listMatchRow{Category: "tracking", Tone: "error", Count: 404, Share: "10.4%", Delta: "▲ 0.4"}
	if tracking != want {
		t.Fatalf("tracking row = %+v, want %+v", tracking, want)
	}
	if got.Rows[1].Category != "analytics" {
		t.Fatalf("second row = %q, want analytics", got.Rows[1].Category)
	}
	if got.Unmatched != 3208 {
		t.Fatalf("unmatched = %d, want 3208", got.Unmatched)
	}
}

func TestMakeListMatchesNoPrior(t *testing.T) {
	got := makeListMatches([]store.CategoryWindowCount{
		{Category: "tracking", Current: 5},
		{Category: "analytics", Current: 3},
	})
	for _, row := range got.Rows {
		if row.Delta != "─" {
			t.Fatalf("%s delta = %q, want ─", row.Category, row.Delta)
		}
	}
}

func TestMakeListMatchesOrder(t *testing.T) {
	got := makeListMatches([]store.CategoryWindowCount{
		{Category: "advertising", Current: 1},
		{Category: "analytics", Current: 1},
		{Category: "malware", Current: 1},
		{Category: "telemetry", Current: 1},
		{Category: "tracking", Current: 1},
	})
	var order []string
	for _, row := range got.Rows {
		order = append(order, row.Category)
	}
	want := []string{"tracking", "advertising", "malware", "analytics", "telemetry"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v", order, want)
		}
	}
}

func TestTrackerBadge(t *testing.T) {
	cases := []struct {
		trend store.Trend
		want  string
	}{
		{store.Trend{HasPrior: false}, ""},
		{store.Trend{HasPrior: true, Change: 19}, "+19 vs prior 7d"},
		{store.Trend{HasPrior: true, Change: -3.46}, "-3.5 vs prior 7d"},
	}
	for _, c := range cases {
		if got := trackerBadge(c.trend); got != c.want {
			t.Errorf("trackerBadge(%+v) = %q, want %q", c.trend, got, c.want)
		}
	}
}

func TestFlagFor(t *testing.T) {
	cases := []struct {
		anomalyType string
		word        string
		tone        string
	}{
		{"tracker_spike", "tracker spike", "error"},
		{"volume_spike", "volume spike", "warn"},
		{"dns_bypass_likely", "bypass likely", "warn"},
		{"dns_bypass_suspected", "bypass suspected", "warn"},
	}
	for _, c := range cases {
		got := flagFor(store.Anomaly{Type: c.anomalyType, DeviceMAC: "aa", DeviceName: "TV"})
		if got.Word != c.word || got.Tone != c.tone {
			t.Errorf("flagFor(%s) = %q/%q, want %q/%q", c.anomalyType, got.Word, got.Tone, c.word, c.tone)
		}
	}
}

func TestFlagsByMACPriority(t *testing.T) {
	flags := flagsByMAC([]store.Anomaly{
		{Type: "volume_spike", DeviceMAC: "aa"},
		{Type: "tracker_spike", DeviceMAC: "aa"},
	})
	if got := flags["aa"].Word; got != "tracker spike" {
		t.Fatalf("flag = %q, want tracker spike", got)
	}
}
