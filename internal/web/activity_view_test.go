package web

import (
	"strings"
	"testing"
	"time"

	"umberrelay/internal/dns"
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
	base := store.Anomaly{DeviceMAC: "aa:bb:cc:dd:ee:ff", DeviceName: "TV", TopDomain: "ads.example", TopDomainSourceList: "Blocklist", Delta: 12, CurrentValue: 30, AverageValue: 10}
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
		if got.ActorKey != "device:aa:bb:cc:dd:ee:ff" || got.Name != "TV" {
			t.Errorf("flagFor(%s) actor = %q %q, want device:aa:bb:cc:dd:ee:ff TV", c.anomalyType, got.ActorKey, got.Name)
		}
	}
}

func TestFlagForBypassBadgeCountsHours(t *testing.T) {
	got := flagFor(store.Anomaly{Type: "dns_bypass_suspected", Delta: 123})
	if got.Badge != "2h 3m silent" {
		t.Fatalf("badge = %q, want 2h 3m silent", got.Badge)
	}
}

func TestFlagsByActorPriority(t *testing.T) {
	flags := flagsByActor([]actorFlag{
		flagFor(store.Anomaly{Type: "volume_spike", DeviceMAC: "aa"}),
		flagFor(store.Anomaly{Type: "tracker_spike", DeviceMAC: "aa"}),
	})
	if got := flags["device:aa"].Word; got != "tracker spike" {
		t.Fatalf("flag = %q, want tracker spike", got)
	}
}

func TestFlagsByActorRateLimitBeatsTrackerSpike(t *testing.T) {
	at := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)
	flags := flagsByActor([]actorFlag{
		flagFor(store.Anomaly{Type: "tracker_spike", DeviceMAC: "aa"}),
		rateLimitFlag("TV", "device:aa", dns.Refusal{Count: 1, First: at, Last: at}, at),
	})
	if got := flags["device:aa"].Word; got != "rate limited" {
		t.Fatalf("flag = %q, want rate limited", got)
	}
}

func TestRateLimitFlagStillHappening(t *testing.T) {
	first := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)
	got := rateLimitFlag("Living Room TV", "source:10.0.30.42", dns.Refusal{Count: 264000, First: first, Last: first.Add(88 * time.Minute)}, first.Add(88*time.Minute+20*time.Second))
	want := actorFlag{
		Word:        "rate limited",
		Tone:        "error",
		Badge:       "264000 refused",
		Explanation: "Living Room TV is sending more than 100 DNS queries a second; 264000 refused over the last 1h 28m.",
		ActorKey:    "source:10.0.30.42",
		Name:        "Living Room TV",
	}
	if got.Word != want.Word || got.Tone != want.Tone || got.Badge != want.Badge || got.Explanation != want.Explanation || got.ActorKey != want.ActorKey || got.Name != want.Name {
		t.Fatalf("rateLimitFlag = %+v, want %+v", got, want)
	}
}

func TestRateLimitFlagStopped(t *testing.T) {
	first := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)
	got := rateLimitFlag("Living Room TV", "source:10.0.30.42", dns.Refusal{Count: 354000, First: first, Last: first.Add(118 * time.Minute)}, first.Add(148*time.Minute))
	want := "Living Room TV sent more than 100 DNS queries a second; 354000 refused over 1h 58m, ending 30m ago."
	if got.Explanation != want {
		t.Fatalf("explanation = %q, want %q", got.Explanation, want)
	}
}

func TestRateLimitFlagFirstMinute(t *testing.T) {
	first := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)
	got := rateLimitFlag("Living Room TV", "source:10.0.30.42", dns.Refusal{Count: 50, First: first, Last: first.Add(10 * time.Second)}, first.Add(20*time.Second))
	if !strings.HasSuffix(got.Explanation, "refused over the last 1m.") {
		t.Fatalf("explanation = %q, want it to end %q", got.Explanation, "refused over the last 1m.")
	}
}
