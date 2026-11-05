# Host guard and DNS rate limit

Date: 2026-11-04
Status: approved by Thom 2026-11-04

## Why

A security audit of Umberrelay as deployed (Raspberry Pi, Docker host networking, an untrusted IoT VLAN allowed to reach DNS on port 53) found two issues that must be fixed before anything else.

1. **DNS rebinding.** The web server never checks the `Host` header. A malicious page opened in a LAN browser can point its own hostname at the Pi and then read `/api/queries` and `/api/devices`, or change settings such as `retention_days`, because the existing origin guard (`internal/web/server.go:166`) compares `Origin` to `Host` and both carry the attacker's name. Chrome's local network access prompt blocks most of this; Firefox and Safari do not.
2. **No DNS rate limit.** Every query is forwarded upstream and stored (`internal/dns/listener.go:77`). One compromised IoT device can fill the Pi's disk, hold goroutines and sockets for up to 4 seconds per slow query, and get the home IP rate-limited by the upstream resolvers.

## Fix 1: reject unrecognized Host headers

Approved by Thom in chat.

**Behavior.** Every HTTP request (pages, API, static files, the SSE stream) has its `Host` header checked before any handler runs. The port is ignored. A request is allowed when the host is:

- an IP literal, IPv4 or IPv6 (Thom reaches the UI at `http://<pi-ip>:8080` today),
- `localhost`, or
- a name listed in the new `http_hosts` config field (case-insensitive).

Anything else gets `403` with the plain-text body `forbidden`, matching the existing cross-origin rejection.

**Config.** New bootstrap field `http_hosts`, a list of hostnames, default empty. Example: `http_hosts = ["umberrelay.home.arpa"]`.

**Caddy.** Thom's standard deployment puts Umberrelay behind Caddy. Caddy's `reverse_proxy` passes the incoming `Host` through by default, so the name Thom browses Caddy at goes in `http_hosts`. The alternative (Caddy rewriting `Host` with `header_up Host {upstream_hostport}`) also works and needs no Umberrelay config; document both in one or two sentences.

**Docs.** README configuration table, the example `config.toml`, the API note that already describes the cross-origin `403`, and Access And Hardening (the Caddy note). CHANGELOG `[Unreleased]` under Security, including that hostname access now needs the name listed.

**Tests.** An unlisted host is rejected with 403; an IP host is allowed; a listed name is allowed. The web test helper (`testServer` in `internal/web/server_test.go`) builds requests with `httptest.NewRequest`, whose default host is `example.com`, so the helper lists `example.com`.

**Not doing.** No filtering of private-IP answers in the DNS path (Umberrelay stays a non-blocking forwarder, as the README promises). No authentication; that stays the reverse proxy's job. No compatibility path for unlisted hostnames.

## Fix 2: per-source DNS rate limit, flagged in Needs attention

**Intent.** No single device can flood Umberrelay hard enough to fill the disk or hammer the upstreams, and when one tries, Needs attention says plainly what is happening.

**The limit.**

- 100 queries per second per source IP, counted in fixed one-second windows. *(Claude's call; page loads and a TV booting stay well under it.)*
- A fixed constant, not a setting. *(Claude's call.)* A `shortcut:` comment records the limit: a NAT or resolver forwarding for many devices shares one limit.
- Checked at the start of query handling, before any upstream exchange, for both UDP and TCP.
- Over the limit, the query gets a `REFUSED` reply and is not forwarded, not emitted as a `QueryRecord`, and not logged. *(Claude's call; `REFUSED` makes a normal stub resolver move to its secondary server instead of retrying.)*

**Counting refusals.** For each source IP, Umberrelay keeps in memory a running count of refused queries, the time of the first refusal, and the time of the most recent one. Thom chose this over a sliding-hour count because the question the flag must answer is whether the flood is still going on. A source's entry is dropped once an hour passes with no refusals, so memory stays bounded by the sources actually being limited, and a later flood starts a fresh count. The per-second window counts must also stay bounded (for example, discarded each window). Counts clear on restart; Thom approved this.

Refusals must not go through the `QueryRecord` channel: a flood would crowd out real queries, which the pipeline drops when the channel is full.

**The flag.** A "rate limited" flag that looks and behaves like the existing flags (`flagFor` in `internal/web/activity_view.go`):

- Shown in Needs attention and on that actor's detail page, like the other flags.
- Word `rate limited`, tone `error`, and the highest priority, so it wins over a tracker spike when an actor has both. *(Claude's call.)*
- Badge with the refused count as a plain integer formatted with `fmt` like the other badges, e.g. `264000 refused`.
- The explanation says whether refusals are still happening. "Still happening" means the most recent refusal is under a minute old.
  - Still happening: *"Living Room TV is sending more than 100 DNS queries a second; 264000 refused over the last 1h 28m."*
  - Stopped: *"Living Room TV sent more than 100 DNS queries a second; 354000 refused over 1h 58m, ending 30m ago."*
- Times are durations, not clock times. *(Claude's call: server-rendered clock times use the container's timezone, which is UTC on the stock image, so "since 14:02" would show the wrong hour on the Pi.)* Share the duration formatting with `silentFor` rather than duplicating it.
- Shown while the most recent refusal is under an hour old.

**Flags for source-IP actors.** Today flags carry only a MAC: `actorFlag.DeviceMAC`, the hard-coded link `/devices/device:{{.DeviceMAC}}` in `activity.html`, and `flagsByMAC`, which the detail page uses only for devices. Flags must instead carry the actor key used everywhere else (`device:<mac>` or `source:<ip>`), so the link lands on the right page and source detail pages show their flag. Existing device flags keep working unchanged.

A refused IP is attributed the way the pipeline attributes queries: a known device when the IP maps to a MAC, otherwise the source IP. The web layer has no tracker, so it matches the refused IP against the IPs of the devices it already loads; no match means a source actor. *(Claude's call.)* The name comes from the existing helpers, so a source labeled in the UI shows its label (`sourceActorDisplayName`).

**Demo mode.** `--demo-data` runs no DNS listener. The web server must still work and simply shows no rate-limited flags; avoid nil checks by giving the web server the same limiter the listener would use.

**Docs.** README Features (anomaly flags) and Architecture (DNS listener bullet). CHANGELOG `[Unreleased]` under Security.

**Tests.** A source over the limit gets `REFUSED` and no `QueryRecord`; the limit resets in the next window; a rate-limited flag renders for a source-IP actor with the right link; the explanation switches from still-happening to stopped wording. Time-dependent tests must not depend on wall-clock timing.

**Not doing.** An API endpoint for rate-limited sources *(Claude's call; Thom asked for the UI flag, and bypass signals have their own endpoint if one is wanted later)*. A configurable limit. Persisting refusal counts. Defense against devices that spoof source IPs.

## Constraints

- TDD for each behavior, per Thom's CLAUDE.md.
- No new dependencies.
- Match existing style; run `gofmt`.
- Work stays on the local branch `wip/host-guard-dns-rate-limit` until both fixes are in, since the repo is public and this document describes unfixed issues. The spec and plan are removed in the same cleanup used for the roster UI spec.

## Left to the builder

- How the limiter reaches the listener and the web server (constructor parameters are expected), and its exact type and method names.
- Where in the middleware chain the host guard sits, as long as every route is covered.
