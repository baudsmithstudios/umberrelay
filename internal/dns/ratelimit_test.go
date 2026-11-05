package dns

import (
	"reflect"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)

// allowLimit sends a full window of queries from ip at the given time, all of which must be allowed.
func allowLimit(t *testing.T, l *RateLimiter, ip string, at time.Time) {
	t.Helper()
	for i := 1; i <= QueriesPerSecondPerSource; i++ {
		if !l.Allow(ip, at) {
			t.Fatalf("query %d from %s at %s refused, want allowed", i, ip, at)
		}
	}
}

// refuseOnce fills ip's window at the given time and sends one more query, which must be refused.
func refuseOnce(t *testing.T, l *RateLimiter, ip string, at time.Time) {
	t.Helper()
	allowLimit(t, l, ip, at)
	if l.Allow(ip, at) {
		t.Fatalf("query over the limit from %s at %s allowed, want refused", ip, at)
	}
}

func TestAllowRefusesOverLimitWithinWindow(t *testing.T) {
	l := NewRateLimiter()
	allowLimit(t, l, "10.0.30.42", t0)
	at := t0.Add(500 * time.Millisecond)
	if l.Allow("10.0.30.42", at) {
		t.Fatal("query over the limit allowed, want refused")
	}
	want := []Refusal{{SourceIP: "10.0.30.42", Count: 1, First: at, Last: at}}
	if got := l.Refusals(at); !reflect.DeepEqual(got, want) {
		t.Fatalf("Refusals = %+v, want %+v", got, want)
	}
}

func TestAllowResetsInNextWindow(t *testing.T) {
	l := NewRateLimiter()
	refuseOnce(t, l, "10.0.30.42", t0)
	if !l.Allow("10.0.30.42", t0.Add(time.Second)) {
		t.Fatal("first query of the next window refused, want allowed")
	}
}

func TestAllowCountsSourcesSeparately(t *testing.T) {
	l := NewRateLimiter()
	allowLimit(t, l, "10.0.30.42", t0)
	if !l.Allow("10.0.30.43", t0) {
		t.Fatal("first query from a second source refused, want allowed")
	}
}

func TestRefusalsAccumulateAcrossWindows(t *testing.T) {
	l := NewRateLimiter()
	later := t0.Add(10 * time.Minute)
	refuseOnce(t, l, "10.0.30.42", t0)
	refuseOnce(t, l, "10.0.30.42", later)
	want := []Refusal{{SourceIP: "10.0.30.42", Count: 2, First: t0, Last: later}}
	if got := l.Refusals(later); !reflect.DeepEqual(got, want) {
		t.Fatalf("Refusals = %+v, want %+v", got, want)
	}
}

func TestRefusalsForgetAfterAnHour(t *testing.T) {
	l := NewRateLimiter()
	later := t0.Add(61 * time.Minute)
	refuseOnce(t, l, "10.0.30.42", t0)
	if got := l.Refusals(later); len(got) != 0 {
		t.Fatalf("Refusals an hour later = %+v, want none", got)
	}
	refuseOnce(t, l, "10.0.30.42", later)
	want := []Refusal{{SourceIP: "10.0.30.42", Count: 1, First: later, Last: later}}
	if got := l.Refusals(later); !reflect.DeepEqual(got, want) {
		t.Fatalf("Refusals after a new flood = %+v, want %+v", got, want)
	}
}

func TestRefusalsSortMostRefusedFirst(t *testing.T) {
	l := NewRateLimiter()
	refuseOnce(t, l, "10.0.30.43", t0)
	refuseOnce(t, l, "10.0.30.42", t0)
	if l.Allow("10.0.30.42", t0) {
		t.Fatal("second query over the limit allowed, want refused")
	}
	got := l.Refusals(t0)
	if len(got) != 2 || got[0].SourceIP != "10.0.30.42" || got[1].SourceIP != "10.0.30.43" {
		t.Fatalf("Refusals = %+v, want 10.0.30.42 (2) then 10.0.30.43 (1)", got)
	}
}
