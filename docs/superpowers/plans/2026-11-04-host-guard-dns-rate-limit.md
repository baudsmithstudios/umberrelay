# Host Guard and DNS Rate Limit Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reject web requests with unrecognized `Host` headers, and cap DNS queries per source IP with a "rate limited" flag in Needs attention.

**Architecture:** A `withHostGuard` middleware joins the existing middleware chain in `internal/web/server.go`. A `RateLimiter` in `internal/dns` is shared by the listener (which asks it before forwarding) and the web server (which reads its refusals to build flags). Flags are re-keyed by actor key so source-IP actors can carry them.

**Tech Stack:** Go 1.26, miekg/dns, html/template, SQLite (untouched).

**Spec:** `docs/superpowers/specs/2026-11-04-host-guard-dns-rate-limit-design.md`

## Global Constraints

- TDD for each behavior: failing test, watch it fail, minimal code, watch it pass.
- No new dependencies.
- Match existing style; run `gofmt -l .` (must print nothing) before each commit.
- Full suite must pass with pristine output before each commit: `go test ./...`.
- No `Co-Authored-By` trailer on commits.
- Work stays on local branch `wip/host-guard-dns-rate-limit`; do not push.
- Comments carry the why only. The rate-limit `shortcut:` comment states the limit and nothing else (no "when to upgrade" instruction).

## Review Focus

- A `Host` of `[::1]` (IPv6 literal, no port) must be allowed. Test in Task 1.
- A listed name sent in different case with a port (`Umberrelay.Local:443`) must be allowed. Test in Task 1.
- A refusal more than an hour old must not be reported, and the next refusal must start a fresh count and first time. Test in Task 2.
- A refused IP that belongs to a known device on the Pi's VLAN must flag the device, not a source. Test in Task 5.
- In a flood's first minute, durations must read `1m`, never `0m`. Test in Task 5.

---

### Task 1: Reject unrecognized Host headers

**Files:**
- Modify: `internal/config/config.go` (add field)
- Modify: `internal/web/server.go:41-60` (`NewServer`, middleware chain), new `withHostGuard`
- Modify: `cmd/umberrelay/main.go:125`
- Modify: `config.toml`, `README.md`, `CHANGELOG.md`
- Test: `internal/config/config_test.go`, `internal/web/server_test.go`

**Interfaces:**
- Produces: `config.Config.HTTPHosts []string` (toml `http_hosts`, default nil, no validation).
- Produces: `web.NewServer(db *store.DB, mgr *classify.Manager, version string, httpHosts []string) *Server`.
- Produces: `withHostGuard(httpHosts []string, next http.Handler) http.Handler` in `server.go`.

- [ ] **Step 1: Write the failing tests**

In `TestLoadValid`, add `http_hosts = ["umberrelay.home.arpa"]` to the TOML and assert `cfg.HTTPHosts` equals `[]string{"umberrelay.home.arpa"}`.

In `server_test.go`, change `testServerVersion` to call `NewServer(db, classify.NewManager(db), version, []string{"example.com", "umberrelay.local"})` (`example.com` is `httptest.NewRequest`'s default host; `umberrelay.local` is used by the origin-guard tests). Then add:

```go
func TestHostGuard(t *testing.T) {
	s := testServer(t)
	cases := []struct {
		host string
		want int
	}{
		{"attacker.example:8080", http.StatusForbidden},
		{"192.168.1.5:8080", http.StatusOK},
		{"[fd00::5]:8080", http.StatusOK},
		{"[::1]", http.StatusOK},
		{"localhost:8080", http.StatusOK},
		{"Umberrelay.Local:443", http.StatusOK},
	}
	for _, c := range cases {
		req := httptest.NewRequest("GET", "/api/health", nil)
		req.Host = c.host
		w := httptest.NewRecorder()
		s.Handler().ServeHTTP(w, req)
		if w.Code != c.want {
			t.Errorf("Host %q: status = %d, want %d", c.host, w.Code, c.want)
		}
	}
}
```

Also assert the rejected body is `forbidden\n` (what `http.Error` writes).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config ./internal/web -run 'TestLoadValid|TestHostGuard' -v`
Expected: build failure (`NewServer` arity, `HTTPHosts` undefined).

- [ ] **Step 3: Implement**

- Add `HTTPHosts []string \`toml:"http_hosts"\`` to `Config`.
- `withHostGuard`: take the hostname with the existing `splitHostPort`, strip `[` `]`, and allow it when `netip.ParseAddr` succeeds, when it equals `localhost` (`strings.EqualFold`), or when it matches an entry of `httpHosts` (`strings.EqualFold`). Otherwise `http.Error(w, "forbidden", http.StatusForbidden)`.
- Chain, built in `NewServer` from its parameter (no `Server` field needed): `withSecurityHeaders(withHostGuard(httpHosts, withMutationOriginGuard(withMutationBodyLimit(s.mux))))`.
- `main.go`: `web.NewServer(db, mgr, version, cfg.HTTPHosts)`.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS, no stray output.

- [ ] **Step 5: Docs**

- `config.toml`: commented example `# http_hosts = ["umberrelay.home.arpa"]` with a one-line comment that names beyond IPs and localhost must be listed.
- README: add `http_hosts` to the config example and the field table (default `[]`; description: hostnames the web UI answers to besides IP addresses and `localhost`). In the API notes, next to the cross-origin `403` line, add that requests with an unrecognized `Host` get a plain-text `403 Forbidden`. In Access And Hardening, add: Caddy passes the incoming `Host` through by default, so list the proxy's hostname in `http_hosts`, or set `header_up Host {upstream_hostport}` in the `reverse_proxy` block.
- CHANGELOG `[Unreleased]` → `### Security`: "The web UI and API reject requests whose `Host` is not an IP address, `localhost`, or a name in the new `http_hosts` setting, closing a DNS rebinding hole. Hostname access needs the name listed."

- [ ] **Step 6: Commit**

```bash
gofmt -l . && go test ./...
git add internal/config internal/web/server.go internal/web/server_test.go cmd/umberrelay/main.go config.toml README.md CHANGELOG.md
git commit -m "Reject web requests with unrecognized Host headers"
```

---

### Task 2: Per-source rate limiter

**Files:**
- Create: `internal/dns/ratelimit.go`
- Test: `internal/dns/ratelimit_test.go`

**Interfaces:**
- Produces:
  ```go
  const QueriesPerSecondPerSource = 100

  type Refusal struct {
  	SourceIP string
  	Count    int
  	First    time.Time
  	Last     time.Time
  }

  func NewRateLimiter() *RateLimiter
  // Allow reports whether sourceIP may send another query in now's one-second window, recording a refusal when it may not.
  func (l *RateLimiter) Allow(sourceIP string, now time.Time) bool
  // Refusals returns sources whose most recent refusal is under an hour old, most refused first.
  func (l *RateLimiter) Refusals(now time.Time) []Refusal
  ```

- [ ] **Step 1: Write the failing tests**

All tests pass explicit times; none read the clock. Use `t0 := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)`.

- `TestAllowRefusesOverLimitWithinWindow`: 100 calls at `t0` return true; the 101st at `t0.Add(500*time.Millisecond)` returns false; `Refusals(t0)` is one entry `{SourceIP: "10.0.30.42", Count: 1, First: t0+500ms, Last: t0+500ms}`.
- `TestAllowResetsInNextWindow`: fill to 101 at `t0`; `Allow` at `t0.Add(time.Second)` returns true.
- `TestAllowCountsSourcesSeparately`: 100 calls for `a` at `t0`; the first call for `b` at `t0` returns true.
- `TestRefusalsAccumulateAcrossWindows`: refusals at `t0` and at `t0+10m` (each after 100 allowed calls in that window) give `Count: 2, First: t0, Last: t0+10m`.
- `TestRefusalsForgetAfterAnHour` (Review Focus): a refusal at `t0`; `Refusals(t0.Add(61*time.Minute))` is empty; a new refusal at `t0+61m` reports `Count: 1, First: t0+61m`.
- `TestRefusalsSortMostRefusedFirst`: `a` refused twice and `b` once → order `a, b`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/dns -run 'TestAllow|TestRefusals' -v`
Expected: build failure (`NewRateLimiter` undefined).

- [ ] **Step 3: Implement `RateLimiter` in `ratelimit.go`**

A mutex guards three fields: the current window start (`now.Truncate(time.Second)`), `map[string]int` counts for that window, and `map[string]*Refusal` per source. When `Allow` sees a new window, replace the counts map and delete refusals whose `Last` is an hour or more before `now`. That keeps both maps bounded by recent traffic. A refusal whose previous `Last` is an hour or more old restarts at `Count: 1, First: now`. `Refusals` copies the qualifying entries and sorts them by `Count` descending. Put the `shortcut:` comment on `QueriesPerSecondPerSource`: one limit per source IP, so a NAT or resolver forwarding for many devices shares it.

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/dns -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go test ./...
git add internal/dns/ratelimit.go internal/dns/ratelimit_test.go
git commit -m "Add a per-source DNS rate limiter"
```

---

### Task 3: Listener refuses queries over the limit

**Files:**
- Modify: `internal/dns/listener.go` (`Listener`, `NewListener`, `handleQuery`)
- Modify: `cmd/umberrelay/main.go:120-135`
- Modify: `README.md` (Architecture, DNS Listener bullet)
- Test: `internal/dns/listener_test.go`

**Interfaces:**
- Consumes: `NewRateLimiter`, `(*RateLimiter).Allow` from Task 2.
- Produces: `NewListener(addr string, upstream []string, limiter *RateLimiter, out chan<- QueryRecord) *Listener`.

- [ ] **Step 1: Write the failing test**

Update the two existing `NewListener` calls to pass `NewRateLimiter()`. Add `TestListenerRefusesSourceOverLimit`, table-driven over `udp` and `tcp` like `TestListenerForwardsQuery`, with a fresh listener and limiter per subtest:
- Records channel capacity `2*QueriesPerSecondPerSource + 1`.
- Send up to `2*QueriesPerSecondPerSource + 1` queries for `example.com.` sequentially from one client, stopping at the first response with `Rcode == mdns.RcodeRefused`. 201 sequential local queries cannot avoid putting 101 in one window unless they span more than a second.
- Assert a REFUSED response arrived.
- Assert `len(records)` equals the number of successful responses (the refused query emitted no record).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/dns -run TestListenerRefusesSourceOverLimit -v`
Expected: build failure on the `NewListener` arity, then (once compiled) FAIL with no REFUSED response.

- [ ] **Step 3: Implement**

Store the limiter on `Listener`. At the start of `handleQuery`, after `sourceIP` is known and before any upstream exchange: if `!l.limiter.Allow(sourceIP, time.Now())`, reply with `RcodeRefused` and return without logging or emitting a record. Share the empty-reply code with the existing SERVFAIL branch instead of duplicating it. In `main.go`, create `limiter := dns.NewRateLimiter()` before `web.NewServer` and pass it to `dns.NewListener(cfg.Listen, cfg.Upstream, limiter, records)`.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Docs and commit**

README Architecture, DNS Listener bullet: add that each source IP is limited to 100 queries a second, and queries over the limit get `REFUSED` without being forwarded or logged.

```bash
gofmt -l . && go test ./...
git add internal/dns/listener.go internal/dns/listener_test.go cmd/umberrelay/main.go README.md
git commit -m "Refuse DNS queries over the per-source rate limit"
```

---

### Task 4: Key flags by actor key

Behavior-preserving for devices; it makes source actors able to carry flags.

**Files:**
- Modify: `internal/web/activity_view.go:99-150, 200-235` (`actorFlag`, `flagFor`, `flagsByMAC`, `makeRoster`)
- Modify: `internal/web/pages.go:257-270, 366-435, 435-455` (`loadAttentionAnomalies`, `handleActivity`, `loadNetworkPane`)
- Modify: `internal/web/static/templates/activity.html:97`
- Test: `internal/web/activity_view_test.go`, `internal/web/pages_test.go`

**Interfaces:**
- Produces: `actorFlag` with `ActorKey string` and `Name string` replacing `DeviceMAC` and `DeviceName` (other fields unchanged). `flagFor` sets `ActorKey: actorKeyForDevice(a.DeviceMAC)`, `Name: a.DeviceName`.
- Produces: `flagsByActor(flags []actorFlag) map[string]actorFlag`, replacing `flagsByMAC`, keeping the highest `priority` per `ActorKey`.
- Produces: `(*Server).loadAttentionFlags(now time.Time) ([]actorFlag, error)`, replacing `loadAttentionAnomalies`, returning `flagFor` of the anomalies and bypass signals. Task 5 adds parameters for rate-limit flags.
- Produces: `loadNetworkPane(now, stats, flags []actorFlag, totalActors)`, which sets `pane.Flags = flags`.

- [ ] **Step 1: Update the tests to the new names**

Rename `TestFlagsByMACPriority` to `TestFlagsByActorPriority` and build its input with `flagFor` so it uses keys from `actorKeyForDevice("aa")`. Add to `TestFlagFor` an assertion that `got.ActorKey == "device:"+base.DeviceMAC` (give `base` a `DeviceMAC`).

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web -run 'TestFlagFor|TestFlagsByActor' -v`
Expected: build failure (`flagsByActor`, `ActorKey` undefined).

- [ ] **Step 3: Implement**

Rename as above. In `handleActivity`, look up the detail flag with `flags[actorKeyForDevice(selected.MAC)]` for devices and `flags[actorKeyForSource(selected.SourceIP)]` for sources (sources gain `detail.Flag`). In `makeRoster`, look flags up by each row's key for devices and sources alike, applying the same `note` and `Hot` logic to source rows. Template link: `<a href="/devices/{{.ActorKey}}">{{.Name}}</a>`.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS. Existing page tests (`TestFlaggedDeviceSubheaderShowsOnlyTheFlag`, `TestTrackerSpikeMarksTheRosterValueHot`) prove device behavior is unchanged.

- [ ] **Step 5: Commit**

```bash
gofmt -l . && go test ./...
git add internal/web
git commit -m "Key flags by actor so sources can carry them"
```

---

### Task 5: Rate-limited flag

**Files:**
- Modify: `internal/web/activity_view.go` (new `rateLimitFlag`, shared minutes formatting with `silentFor`)
- Modify: `internal/web/pages.go` (`loadAttentionFlags`)
- Modify: `internal/web/server.go` (`NewServer` gains the limiter)
- Modify: `cmd/umberrelay/main.go`
- Modify: `README.md` (Features), `CHANGELOG.md`
- Test: `internal/web/activity_view_test.go`, `internal/web/pages_test.go`, `internal/web/server_test.go` (helper)

**Interfaces:**
- Consumes: `dns.Refusal`, `dns.QueriesPerSecondPerSource`, `(*dns.RateLimiter).Refusals`, `(*dns.RateLimiter).Allow` (tests use it to create refusals).
- Consumes: `actorFlag`, `flagsByActor`, `loadAttentionFlags` from Task 4.
- Produces: `web.NewServer(db *store.DB, mgr *classify.Manager, version string, httpHosts []string, limiter *dns.RateLimiter) *Server`.
- Produces: `rateLimitFlag(name, actorKey string, r dns.Refusal, now time.Time) actorFlag`.
- Produces: `(*Server).loadAttentionFlags(now time.Time, devices []store.DeviceWithTrends, sources []store.SourceWithTrends) ([]actorFlag, error)`. `handleActivity` already loads both lists before it needs the flags.

- [ ] **Step 1: Write the failing tests**

`activity_view_test.go`, with `first := time.Date(2026, 11, 4, 14, 2, 0, 0, time.UTC)`:

- `TestRateLimitFlagStillHappening`: `rateLimitFlag("Living Room TV", "source:10.0.30.42", dns.Refusal{Count: 264000, First: first, Last: first.Add(88*time.Minute)}, first.Add(88*time.Minute + 20*time.Second))` →
  - Word `rate limited`, Tone `error`, Badge `264000 refused`, ActorKey `source:10.0.30.42`
  - Explanation `Living Room TV is sending more than 100 DNS queries a second; 264000 refused over the last 1h 28m.`
- `TestRateLimitFlagStopped`: `Refusal{Count: 354000, First: first, Last: first.Add(118*time.Minute)}`, now `first.Add(148*time.Minute)` → Explanation `Living Room TV sent more than 100 DNS queries a second; 354000 refused over 1h 58m, ending 30m ago.`
- `TestRateLimitFlagFirstMinute` (Review Focus): `Refusal{Count: 50, First: first, Last: first.Add(10*time.Second)}`, now `first.Add(20*time.Second)` → Explanation ends `refused over the last 1m.`
- `TestFlagsByActorPriority`: add a case where a rate-limited flag beats a tracker spike for the same key.

`pages_test.go`:

- `TestRateLimitedSourceIsFlagged`: seed a source query for `10.44.0.7` and label it `Kitchen Display` (as in `TestDeviceDetailPageSourceRoute`), then call `s.limiter.Allow("10.44.0.7", now)` 101 times. Assert:
  - `GET /` contains `<a href="/devices/source:10.44.0.7">Kitchen Display · 10.44.0.7</a> · rate limited.`
  - `GET /devices/source:10.44.0.7` contains `data-tone="error">rate limited</span>`
  - the `/devices/source:10.44.0.7` roster row from `rowFor` contains `rate limited`.
- `TestRateLimitedKnownDeviceFlagsTheDevice` (Review Focus): after `seedPrivacyPageData`, refuse `192.168.1.10` (device `aa:bb:cc:dd:ee:ff`). `GET /` contains `<a href="/devices/device:aa:bb:cc:dd:ee:ff">Living Room TV</a> · rate limited.`

Update `testServerVersion` to pass `dns.NewRateLimiter()`.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web -run 'TestRateLimit|TestFlagsByActor' -v`
Expected: build failure (`rateLimitFlag`, `s.limiter`, `NewServer` arity).

- [ ] **Step 3: Implement**

- `NewServer` stores the limiter as `s.limiter`. `main.go` passes the `limiter` created in Task 3, so demo mode gets a limiter that never refuses and needs no nil checks.
- Pull the minutes formatting out of `silentFor` into `formatMinutes(m int) string` (`12m`, `2h 3m`). `silentFor` keeps its output. `rateLimitFlag` uses whole minutes, rounded down with a floor of 1.
- `rateLimitFlag`: Word `rate limited`, Tone `error`, priority 4, Badge `fmt.Sprintf("%d refused", r.Count)`. It is still happening when `now.Sub(r.Last) < time.Minute`; the explanation copy is exactly as in the tests, with the limit from `dns.QueriesPerSecondPerSource`.
- `loadAttentionFlags`: for each `s.limiter.Refusals(now)` entry, a device whose `IP` equals the refused IP gives `actorKeyForDevice(mac)` and `deviceDisplayName(device)`. Otherwise use `actorKeyForSource(ip)` and `sourceActorDisplayName(ip, label)`, with the label from `findSource(sources, ip)` when found. Rate-limit flags come first, then the anomaly flags.

- [ ] **Step 4: Run the full suite**

Run: `go test ./...`
Expected: PASS.

- [ ] **Step 5: Docs and commit**

- README Features, Anomaly flags bullet: add that sources over the DNS rate limit are flagged "rate limited" with the refused count and whether it is still happening.
- CHANGELOG `[Unreleased]` → `### Security`: "DNS queries are limited to 100 a second per source IP; queries over the limit get `REFUSED` and are not forwarded or logged. Rate-limited devices and sources are flagged in Needs attention."

```bash
gofmt -l . && go test ./...
git add internal/web cmd/umberrelay/main.go README.md CHANGELOG.md
git commit -m "Flag rate-limited sources in Needs attention"
```
