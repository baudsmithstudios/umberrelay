<p align="center">
  <img src="assets/umberrelay-title.svg" alt="Umberrelay" width="600">
</p>

<p align="center">
  <img src="https://img.shields.io/badge/Raspberry%20Pi-A22846?style=flat&logo=raspberrypi&logoColor=white" alt="Raspberry Pi">
  <a href="https://github.com/baudsmithstudios/umberrelay/releases/latest"><img src="https://img.shields.io/github/v/release/baudsmithstudios/umberrelay?label=version" alt="Latest release"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache%202.0-5faf87.svg" alt="Apache 2.0 License"></a>
  <img src="https://img.shields.io/badge/go-1.26-00ADD8.svg" alt="Go 1.26">
  <img src="https://img.shields.io/badge/platform-Linux%20%2F%20ARM64-333333.svg" alt="Linux / ARM64">
</p>

## What It Does

Umberrelay is a forwarding DNS server that logs every query, identifies which network actor made it, and classifies domains against community-maintained tracking lists. It gives you an attribution-focused picture of where your network traffic is going — and how much of it is talking to trackers.

DNS is an intentionally narrow lens, but it is still a useful one: it is cheap to collect, passive to deploy, and often enough to reveal which devices are the noisiest, which services they depend on, and how often they contact known trackers. Umberrelay focuses on turning that signal into something readable instead of trying to be a blocker, IDS, or full packet analyzer.

## Features

- **Forwarding DNS server** — drop-in replacement for your router's DNS, forwards to upstream resolvers (Cloudflare, Google, etc.)
- **Attribution with source-IP fallback** — maps queries to devices via ARP table polling, DHCP snooping, mDNS, and SSDP discovery, and surfaces source IP when MAC is unavailable
- **Domain classification** — matches queries against configurable blocklists (Steven Black, EasyPrivacy, Disconnect.me) with automatic refresh
- **OUI vendor lookup** — identifies device manufacturers from MAC address prefixes
- **Privacy UI** — Privacy and Settings pages for query volume, tracker percentage, actor breakdown (device or source fallback), domain rankings, and runtime configuration
- **Best-effort DoH/DoT bypass signal** — flags devices that appear active on LAN but stop using local DNS, with higher confidence when encrypted-DNS bootstrap domains were seen
- **REST API** — JSON API for actors, devices, queries, activity, domains, lists, settings, and overrides
- **Domain overrides** — manually classify any domain when the lists get it wrong
- **Persistent storage** — SQLite (WAL mode), configurable retention, batched writes
- **Configurable via UI** — retention, list refresh interval, blocklist management all from the settings page

## What Umberrelay Is Not

- **Not a DNS blocker** — Umberrelay labels domains but does not block or rewrite responses
- **Not an IDS or firewall** — it does not inspect packets deeply, enforce policy, or sit inline as a security appliance
- **Not a packet capture tool** — it works from DNS traffic plus passive discovery signals, not full payload capture
- **Not complete network visibility** — devices using DoH, DoT, hardcoded resolvers, or direct IP connections can bypass the DNS lens entirely

## Where It Fits Best

Umberrelay is strongest when you want a **fully local, low-overhead privacy view by device** without turning your network into a full security stack.

- If your main question is **"which device is talking to trackers, and how much?"**, Umberrelay is a good fit.
- If your main question is **"what protocol flow and payload details are on my network?"**, use DPI/flow tools (or run them alongside Umberrelay).
- If your main question is **"block ads/trackers aggressively at DNS"**, a blocker-first tool (Pi-hole or AdGuard Home) is a better primary fit.

## Quick Start

> For Raspberry Pi deployment, ARM64 image builds on a dev machine, and live-Pi testing, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

```sh
git clone https://github.com/baudsmithstudios/umberrelay.git && cd umberrelay

# Build and run with Docker
docker compose up -d
```

Then open `http://localhost:8080` in a browser and point your router's DNS to the host running Umberrelay.

## Deployment Model

Umberrelay works best when it is the DNS server your network actually uses. In the common setup, that means pointing your router's LAN DNS setting at the host running Umberrelay so client devices send their queries through it.

That deployment model is the main tradeoff. Umberrelay sees only the DNS traffic that reaches it. If a device uses encrypted DNS (`DoH` / `DoT`), a hardcoded external resolver, or talks directly to IP addresses without DNS lookups, Umberrelay's visibility for that device becomes partial.

### Works With Pi-hole / AdGuard Home

Umberrelay is a passive DNS pass-through observer and classifier, so you can run it alongside blocker-first tools instead of choosing one or the other.

Recommended chain:

```
Clients / Router
      |
      v
  Umberrelay
      |
      v
Pi-hole or AdGuard Home
      |
      v
Upstream DNS
```

This lets Pi-hole or AdGuard do blocking while Umberrelay provides attribution-aware privacy reporting (including source-IP fallback when MAC is unavailable).

Caveats:

- Avoid DNS forwarding loops.
- Do not bind both services to the same `:53` socket on the same host/interface without explicit port/interface separation.

## Configuration

Umberrelay needs minimal bootstrap config — everything else is managed through the web UI.

```toml
# config.toml
listen    = "0.0.0.0:53"
upstream  = ["1.1.1.1:53", "8.8.8.8:53"]
data_dir  = "/data"
http_port = 8080
```

| Field | Default | Description |
|---|---|---|
| `listen` | `0.0.0.0:53` | DNS listener address |
| `upstream` | `["1.1.1.1:53", "8.8.8.8:53"]` | Upstream DNS resolvers (sequential fallback) |
| `data_dir` | `/data` | SQLite database and data directory |
| `http_port` | `8080` | Web UI and API port |

All fields are optional — Umberrelay runs with sane defaults if no config file exists.

### Runtime Settings

These are managed through the web UI or API:

| Setting | Default | Description |
|---|---|---|
| `retention_days` | `30` | Days of query history to keep before purging |
| `list_refresh_hours` | `24` | Hours between blocklist refresh cycles |

## Device Discovery

Umberrelay uses four passive methods to build and maintain a device inventory:

| Method | What It Discovers |
|---|---|
| **ARP table** | IP-to-MAC mapping from `/proc/net/arp` (polled every 30s) |
| **DHCP snooping** | Hostnames from DHCP option 12 in client requests |
| **mDNS** | Hostnames from PTR/SRV records on `224.0.0.251:5353` |
| **SSDP** | Device presence from announcements on `239.255.255.250:1900` |

All discovery is passive — Umberrelay never sends probes or scans your network.

## Classification

Umberrelay ships with three default blocklists:

| List | Category |
|---|---|
| [Steven Black Unified](https://raw.githubusercontent.com/StevenBlack/hosts/master/hosts) | tracking |
| [EasyPrivacy](https://v.firebog.net/hosts/Easyprivacy.txt) | analytics |
| [Disconnect.me Tracking](https://s3.amazonaws.com/lists.disconnect.me/simple_tracking.txt) | tracking |

Lists are fetched on first run, cached to SQLite, and refreshed on a configurable interval. Add, remove, or disable lists from the settings page. Custom list URLs must be public `http` or `https` endpoints. Override individual domain classifications when you disagree with a list.

## Privacy And Storage

Umberrelay stores DNS query history, device identifiers discovered on the local network, domain classifications, and the runtime settings needed by the UI and API. By default, query history is retained for 30 days and then purged automatically.

All of that data stays local unless you choose upstream DNS resolvers or blocklists hosted elsewhere. Umberrelay does not ship analytics, cloud sync, or third-party telemetry.

## API

The API is unauthenticated — bind to localhost or a trusted network.

- Read endpoints return JSON.
- Mutation endpoints accept `application/json`.
- Mutation endpoints return either JSON or an empty success status (`204 No Content` / `202 Accepted`).
- Errors return JSON in the form `{ "error": "message" }`.
- `/ui/...` routes are internal SSR form handlers, not part of the public API contract.

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/health` | Health check |
| `GET` | `/api/summary` | Dashboard stats (last 24h) |
| `GET` | `/api/devices` | All devices with query stats |
| `GET` | `/api/actors` | Attribution actors (known devices + source-IP fallback actors) with query stats |
| `GET` | `/api/devices/{mac}` | Single device |
| `PUT` | `/api/devices/{mac}` | Update device label |
| `GET` | `/api/queries` | Query log (filterable by actor, device, domain, time range) |
| `GET` | `/api/queries/stream` | Live query stream via Server-Sent Events (filterable by actor, device, domain, category) |
| `GET` | `/api/activity` | Activity buckets for `24h`, `7d`, or `30d` (optionally filter by actor, device, or source) |
| `GET` | `/api/anomalies` | Known devices with unusual tracker rate or query volume spikes |
| `GET` | `/api/bypass` | Best-effort signals for devices that may be bypassing local DNS visibility |
| `GET` | `/api/domains` | Top domains with source list attribution and attribution-actor counts (last 24h) |
| `GET` | `/api/settings` | Current settings |
| `GET` | `/api/lists/status` | Last classification-list refresh attempt/success/error status |
| `PUT` | `/api/settings` | Update settings |
| `GET` | `/api/lists` | All classification lists |
| `POST` | `/api/lists` | Add a list |
| `PUT` | `/api/lists/{id}` | Enable or disable a list |
| `DELETE` | `/api/lists/{id}` | Remove a list |
| `POST` | `/api/lists/refresh` | Trigger immediate list refresh |
| `PUT` | `/api/overrides/{domain}` | Set domain classification override |
| `DELETE` | `/api/overrides/{domain}` | Remove domain override |

### Request Bodies

Mutation endpoints expect JSON request bodies:

| Endpoint | JSON Body |
|---|---|
| `PUT /api/devices/{mac}` | `{ "label": "Living Room TV" }` |
| `PUT /api/settings` | `{ "retention_days": 30, "list_refresh_hours": 24 }` |
| `POST /api/lists` | `{ "url": "https://example.com/list.txt", "name": "Example", "category": "tracking" }` |
| `PUT /api/lists/{id}` | `{ "enabled": true }` |
| `PUT /api/overrides/{domain}` | `{ "category": "tracking" }` |

### Response Shapes

Selected read endpoints return these JSON shapes:

| Endpoint | JSON Response |
|---|---|
| `GET /api/health` | `{ "status": "ok" }` |
| `GET /api/actors` | `[{"key":"device:aa:bb:cc:dd:ee:ff","type":"device","name":"Living Room TV","device_mac":"aa:bb:cc:dd:ee:ff","source_ip":"","query_count":120,"tracker_percent":47.5},{"key":"source:10.0.0.7","type":"source","name":"Unattributed · 10.0.0.7","device_mac":"","source_ip":"10.0.0.7","query_count":25,"tracker_percent":12}]` |
| `GET /api/activity` | `[{"timestamp": 1711670400, "total": 42, "tracker": 18}]` |
| `GET /api/anomalies` | `[{"device_mac": "aa:bb:cc:dd:ee:ff", "device_name": "Living Room TV", "type": "tracker_spike", "current_value": 75, "average_value": 20, "delta": 55, "top_domain": "ads.example.com", "top_domain_category": "tracking", "top_domain_source_list": "Tracking List"}]` |
| `GET /api/bypass` | `[{"device_mac":"aa:bb:cc:dd:ee:ff","device_name":"Living Room TV","confidence":"likely","hint_domain":"dns.google","silent_minutes":180,"prior_query_count":42,"last_seen":1711670400,"last_query":1711659600}]` |
| `GET /api/domains` | `{ "total_devices": 12, "domains": [{"domain": "ads.example.com", "category": "tracking", "query_count": 120, "device_count": 4, "source_list": "Tracking List"}] }` |
| `GET /api/settings` | `{ "retention_days": 30, "list_refresh_hours": 24 }` |
| `GET /api/lists/status` | `{ "last_attempt_at": 1711670400, "last_success_at": 1711666800, "last_error": "..." }` |
| `GET /api/queries/stream` | SSE `query` events with JSON `data` like `{"id":42,"actor_key":"device:aa:bb:cc:dd:ee:ff","device_mac":"aa:bb:cc:dd:ee:ff","source_ip":"192.168.1.10","domain":"ads.example.com","query_type":"A","category":"tracking","timestamp":1711670400}` |

Selected error responses use this JSON shape:

| Condition | JSON Response |
|---|---|
| Validation or request error | `{ "error": "message" }` |
| Not found | `{ "error": "message" }` |
| Internal or dependency error | `{ "error": "message" }` |

### Query Parameters

`GET /api/queries` supports:

| Param | Description |
|---|---|
| `actor` | Filter by actor key (`device:{mac}` or `source:{ip}`) |
| `device` | Filter by device MAC |
| `domain` | Filter by domain |
| `from` | Start time (RFC3339) |
| `to` | End time (RFC3339, defaults to now) |
| `limit` | Results per page (default 100) |
| `offset` | Pagination offset |

When `actor` is set, it takes precedence over `device`.

`GET /api/queries/stream` supports:

| Param | Description |
|---|---|
| `actor` | Filter by actor key (`device:{mac}` or `source:{ip}`) |
| `device` | Filter by device MAC |
| `domain` | Filter by domain |
| `category` | Filter by category (`tracking`, `advertising`, `analytics`, `telemetry`, `malware`, `uncategorized`) |
| `after` | Only emit events with query ID greater than this cursor |
| `limit` | Batch size per poll (default 100, max 500) |

When `actor` is set, it takes precedence over `device`.

`GET /api/activity` supports:

| Param | Description |
|---|---|
| `actor` | Filter by actor key (`device:{mac}` or `source:{ip}`) |
| `device` | Filter by device MAC |
| `source` | Filter by unattributed source IP |
| `range` | Time window: `24h` (default, hourly buckets), `7d` (daily buckets), or `30d` (daily buckets) |

Filter precedence is `actor`, then `source`, then `device`.

`GET /api/domains` returns an object with `total_devices` plus a `domains` array. Each domain item includes:

| Field | Description |
|---|---|
| `domain` | Domain name |
| `category` | Stored classification category |
| `query_count` | Number of matching queries in the last 24h |
| `device_count` | Distinct attribution actors that queried the domain in the last 24h (device MACs + source-IP fallback actors) |
| `source_list` | Best-effort attribution for the matching blocklist, or `manual` / `unknown` |

This replaces the previous flat array response for `GET /api/domains`.

## Docker Deployment

The default [`docker-compose.yml`](docker-compose.yml) is aimed at local development and simple local Docker runs. It uses `network_mode: host` so Umberrelay can see DNS traffic and the ARP table, mounts config read-only, and stores `/data` in a named volume.

For Raspberry Pi deployment from a dev machine, use [`docker-compose.pi.yml`](docker-compose.pi.yml) together with the workflow in [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md). That compose file references a prebuilt `umberrelay:latest` image and is designed for `docker load` on the Pi after transferring an ARM64 image tar.

### Runtime Requirements

- **Linux host** — device attribution depends on Linux networking details such as `/proc/net/arp`
- **Port access** — Umberrelay needs to bind DNS on port `53`; passive listeners also use UDP `67`, `5353`, and `1900`
- **Host networking** — the provided Docker deployment uses `network_mode: host` so DNS and multicast traffic are visible to the container
- **Trusted network placement** — the web UI and API are unauthenticated, so the host should live on a network you trust or sit behind a reverse proxy

```sh
docker compose up -d        # start
docker compose logs -f      # logs
docker compose down          # stop
```

For Pi deployment with the separate compose file:

```sh
docker compose -f docker-compose.pi.yml up -d
docker compose -f docker-compose.pi.yml logs -f
docker compose -f docker-compose.pi.yml down
```

The Dockerfile uses a two-stage build: compile in `golang:1.26-alpine`, run in `alpine:3.19` with just the binary and CA certificates.

## Troubleshooting

- **A device is missing** — confirm the device is actually using Umberrelay for DNS; devices with hardcoded resolvers or encrypted DNS may never appear
- **A bypass signal is unexpected** — `/api/bypass` is best-effort, not packet-level proof; validate with direct DNS tests (`dig @<umberrelay-ip> ...`) and your router DNS policy
- **Routed client is unattributed** — across subnets/VLANs, Umberrelay may only have source IP (no MAC); verify the client appears as a source fallback actor in the Privacy page or `/api/actors`
- **Queries are visible but device names are weak** — hostname enrichment depends on passive DHCP, mDNS, and SSDP traffic; some devices simply do not advertise much
- **Tracker labels look wrong** — classifications come from community blocklists; use domain overrides when a list is too broad or out of date
- **Some traffic is invisible** — Umberrelay does not see direct IP traffic or DNS that bypasses it, so partial visibility is an expected limitation in some networks

## Architecture

```
DNS Client
    │
    ▼
DNS Listener (UDP + TCP)
    │
    ├─ Forward to upstream (sequential fallback)
    ├─ Emit QueryRecord to channel
    │
    ▼
Pipeline Writer (async, batched)
    │
    ├─ Resolve IP → MAC via Tracker
    ├─ Classify domain via Manager
    └─ Batch write to SQLite
                                        ↑
Device Tracker (goroutine) ─────────────┘
    ├─ ARP poller (30s)
    ├─ DHCP listener (port 67)
    ├─ mDNS listener (224.0.0.251:5353)
    └─ SSDP listener (239.255.255.250:1900)

Classification Manager (goroutine)
    ├─ Fetch lists on startup
    ├─ Cache to SQLite
    └─ Periodic refresh

Purge Loop (goroutine)
    └─ Delete queries older than retention_days (daily)

Web Server
    ├─ Privacy page
    ├─ Settings page
    ├─ HTMX fragment handlers for privacy investigation and settings actions
    └─ REST API
```

- **DNS Listener** — dual-stack UDP/TCP, forwards to upstream with sequential fallback, emits records non-blocking (drops on channel full rather than blocking DNS)
- **Pipeline Writer** — batches queries (100 per batch or 1s flush), enriches with device MAC and domain category before writing
- **Device Tracker** — passive-only discovery, never probes the network
- **Classification Manager** — atomic pointer swap on refresh, lock-free reads on the hot path
- **SQLite** — WAL mode, `NORMAL` synchronous; schema auto-applied on startup

## Security

- **No blocking** — Umberrelay observes and classifies but does not block or modify DNS responses
- **No authentication** — the web UI and API are unauthenticated; bind to a trusted network or put behind a reverse proxy
- **No outbound data** — the only outbound connections are DNS forwarding and blocklist fetches
- **Passive discovery** — device identification uses only broadcast/multicast traffic and the local ARP table
- **Parameterized queries** — all SQL uses parameterized statements
- **Input validation** — API and UI mutation handlers validate JSON bodies, form inputs, list URLs, and allowed categories

## Comparison

Umberrelay targets a narrower niche than general network monitors or DNS blockers: a fully local DNS forwarder that turns query logs into per-device privacy reporting. The tradeoff is deliberate. Umberrelay does not do blocking, deep packet inspection, or inline firewalling, because that would push it into a heavier product class with different deployment and hardware expectations. That narrower scope matters because it keeps Umberrelay simple, Pi-friendly, and local-first while filling a practical gap: turning DNS activity into concise per-device privacy visibility instead of just raw logs, broad traffic categories, or IDS-style events.

| Feature | Umberrelay | [Pi-hole](https://pi-hole.net/) | [AdGuard Home](https://adguard.com/en/adguard-home/overview.html) | [Firewalla](https://firewalla.com/) | [ntopng](https://www.ntop.org/products/traffic-analysis/ntop/) |
|---|:---:|:---:|:---:|:---:|:---:|
| DNS query logging | Yes | Yes | Yes | Some | Some |
| Per-device attribution | Yes | Yes | Yes | Yes | Yes |
| Tracker / blocklist classification | Yes | Yes | Yes | Some | Some |
| Privacy-focused per-device summaries | Yes | No | No | Some | No |
| DNS blocking | No | Yes | Yes | Yes | No |
| Flow / DPI visibility | No | No | No | Yes | Yes |
| Fully local, self-hosted | Yes | Yes | Yes | No | Yes |
| Pi-friendly deployment | Yes | Yes | Yes | No | Mixed |
| Open source | Yes | Yes | Yes | No | Yes |

## Tech Stack

| Component | Library | Description |
|---|---|---|
| **Language** | [Go](https://github.com/golang/go) | 1.26 |
| **DNS server** | [miekg/dns](https://github.com/miekg/dns) | Full-featured DNS library |
| **Database** | [modernc.org/sqlite](https://gitlab.com/cznic/sqlite) | Pure-Go SQLite driver (CGo-free) |
| **Config parsing** | [BurntSushi/toml](https://github.com/BurntSushi/toml) | TOML configuration file parser |
| **Containerization** | [Docker](https://github.com/moby/moby) | Multi-stage build |
