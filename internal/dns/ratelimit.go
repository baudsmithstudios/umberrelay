package dns

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// shortcut: one limit per source IP, so a NAT or forwarding resolver shares it.
const QueriesPerSecondPerSource = 100

const refusalsKeptFor = time.Hour

type Refusal struct {
	SourceIP string
	Count    int
	First    time.Time
	Last     time.Time
}

type RateLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	counts      map[string]int
	refusals    map[string]Refusal
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{counts: map[string]int{}, refusals: map[string]Refusal{}}
}

// Allow records a refusal when it returns false.
func (l *RateLimiter) Allow(sourceIP string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if window := now.Truncate(time.Second); !window.Equal(l.windowStart) {
		l.windowStart = window
		l.counts = map[string]int{}
		for ip, r := range l.refusals {
			if now.Sub(r.Last) >= refusalsKeptFor {
				delete(l.refusals, ip)
			}
		}
	}
	if l.counts[sourceIP] < QueriesPerSecondPerSource {
		l.counts[sourceIP]++
		return true
	}
	r, ok := l.refusals[sourceIP]
	if !ok {
		r = Refusal{SourceIP: sourceIP, First: now}
	}
	r.Count++
	r.Last = now
	l.refusals[sourceIP] = r
	return false
}

func (l *RateLimiter) Refusals(now time.Time) []Refusal {
	l.mu.Lock()
	defer l.mu.Unlock()
	var recent []Refusal
	for _, r := range l.refusals {
		if now.Sub(r.Last) < refusalsKeptFor {
			recent = append(recent, r)
		}
	}
	slices.SortFunc(recent, func(a, b Refusal) int { return cmp.Compare(b.Count, a.Count) })
	return recent
}
