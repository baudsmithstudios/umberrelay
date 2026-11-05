package dns

import (
	"cmp"
	"slices"
	"sync"
	"time"
)

// shortcut: one limit per source IP, so a NAT or resolver forwarding for many devices shares it.
const QueriesPerSecondPerSource = 100

// refusalsKeptFor is how long a source's refusals are remembered after its most recent one.
const refusalsKeptFor = time.Hour

// Refusal summarizes the queries refused from one source since its flood began.
type Refusal struct {
	SourceIP string
	Count    int
	First    time.Time
	Last     time.Time
}

// RateLimiter caps queries per source IP in fixed one-second windows and remembers recent refusals.
type RateLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	counts      map[string]int
	refusals    map[string]Refusal
}

func NewRateLimiter() *RateLimiter {
	return &RateLimiter{counts: map[string]int{}, refusals: map[string]Refusal{}}
}

// Allow reports whether sourceIP may send another query in now's one-second window, recording a refusal when it may not.
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

// Refusals returns sources whose most recent refusal is under an hour old, most refused first.
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
