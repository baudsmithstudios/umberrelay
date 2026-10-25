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
	want := listMatchRow{Category: "tracking", Count: 404, Share: "10.4%", Delta: "▲ 0.4"}
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

func TestFlagFor(t *testing.T) {
	base := store.Anomaly{DeviceName: "TV", TopDomain: "ads.example", TopDomainSourceList: "Blocklist", Delta: 12, CurrentValue: 30, AverageValue: 10}
	cases := []struct {
		anomalyType string
		want        actorFlag
	}{
		{"tracker_spike", actorFlag{Word: "tracker spike", Tone: "error", Badge: "+12 pts", Explanation: "TV jumped above its usual tracker rate, led by ads.example from Blocklist."}},
		{"volume_spike", actorFlag{Word: "volume spike", Tone: "warn", Badge: "3.0x daily avg", Explanation: "TV is making far more requests than usual, with ads.example contributing the largest spike from Blocklist."}},
		{"dns_bypass_likely", actorFlag{Word: "bypass likely", Tone: "warn", Badge: "12m silent", Explanation: "TV is present on the LAN but has stopped using local DNS. Earlier queries included ads.example, which suggests encrypted DNS usage."}},
		{"dns_bypass_suspected", actorFlag{Word: "bypass suspected", Tone: "warn", Badge: "12m silent", Explanation: "TV is present on the LAN but has stopped using local DNS despite prior DNS activity, so visibility may be bypassed."}},
	}
	for _, c := range cases {
		a := base
		a.Type = c.anomalyType
		got := flagFor(a)
		if got.Word != c.want.Word || got.Tone != c.want.Tone || got.Badge != c.want.Badge || got.Explanation != c.want.Explanation {
			t.Errorf("flagFor(%s) = %+v, want %+v", c.anomalyType, got, c.want)
		}
	}
}

func TestFlagForBypassBadgeCountsHours(t *testing.T) {
	got := flagFor(store.Anomaly{Type: "dns_bypass_suspected", Delta: 123})
	if got.Badge != "2h 3m silent" {
		t.Fatalf("badge = %q, want 2h 3m silent", got.Badge)
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
