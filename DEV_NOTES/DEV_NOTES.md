# Umberrelay — Development Notes

Lightweight network behavior and privacy analyzer for Raspberry Pi homelabs.
Companion project to [Vigil](../vigil) (system monitor).

**Positioning:** "See what your devices are really doing — and who they're talking to."

## Naming Shortlist

Current project name:
- `Umberrelay`

Shortlisted alternatives considered during naming review:
- `Gloamrelay`
- `Veilrelay`
- `Cinderrelay`
- `Murkrelay`
- `Dreadrelay`
- `Hollowsignal`
- `Hollowrelay`
- `Veilsignal`
- `Gloamgrid`
- `Hallowire`
- `Cindersigil`
- `Ashenmesh`
- `Umbermesh`

---

## Competitive Landscape

### Established Tools

#### Pi-hole / AdGuard Home
- DNS sinkhole, blocks ads/trackers at DNS level
- Runs trivially on Pi (Pi-hole runs on a Pi Zero)
- Excellent query logging and dashboard
- **Gaps:** DNS-only visibility means hardcoded IPs and DoH/DoT bypass still disappear from view. Client attribution is mostly limited to IP, MAC, and hostname context. Strong for filtering and query logs, lighter on per-device privacy summaries or behavioral analysis.

#### ntopng
- Real-time flow monitoring, deep packet inspection via nDPI
- Protocol/application classification, JA3/JA4 TLS fingerprinting
- **Gaps:** Much broader than Scrye, but also heavier operationally. On Pi-class hardware it can become memory- and CPU-hungry under sustained traffic, and some advanced features live in paid editions. Better fit for users who want full traffic analysis than a focused DNS privacy lens.

#### Zeek (formerly Bro)
- Network security monitor producing structured logs (conn, dns, http, ssl, etc.)
- Extensible via scripting language, JA3/JA4 via packages
- **Gaps:** Extremely capable, but log-centric and expertise-heavy. Pi deployments are possible for lighter workloads, though sustained monitoring is still a much bigger ask than Scrye's DNS-forwarder model. No built-in UI, so the user still has to build the reporting layer.

#### Suricata
- Signature-based IDS/IPS with EVE JSON logging
- Mature rule ecosystem (ET Open, Snort rules), TLS fingerprinting
- **Gaps:** Strong IDS tool, but optimized for threat detection rather than explaining routine device behavior. On Pi hardware it is usually a constrained fit, especially with richer rule sets. It answers "is something suspicious or known-bad?" better than "what privacy patterns does this device show over time?"

#### Netdata
- Real-time system/infrastructure monitoring with beautiful dashboards
- Lightweight, runs well on Pi, ML-based anomaly detection for system metrics
- **Gaps:** Excellent system monitor, but network visibility is mostly interface-level. It does not try to attribute DNS or traffic behavior to individual devices, so it complements Scrye more than it replaces it.

#### Wireshark / tshark
- Gold standard packet capture and protocol dissection
- tshark runs fine on Pi
- **Gaps:** Best-in-class for packet inspection and debugging, but closer to a microscope than a long-running household report. Continuous capture is storage-heavy, and turning packets into actionable per-device summaries is still left to the operator.

#### Darkstat
- Lightweight per-host bandwidth stats with simple web UI
- Trivially runs on Pi
- **Gaps:** Lightweight and approachable, but intentionally narrow: basic bandwidth visibility rather than DNS attribution, classification, or privacy reporting. Useful as a simple traffic meter, not a privacy analyzer.

#### Arpwatch
- Monitors ARP traffic for MAC/IP pairing changes, new device detection
- Negligible resource usage
- **Gaps:** Very good at one job: spotting address changes and new devices. Beyond that, it does not attempt traffic analysis, privacy reporting, or a modern dashboard.

#### NMAP
- Best-in-class active host discovery and OS/service fingerprinting
- **Gaps:** Excellent inventory and point-in-time reconnaissance tool, but it is active rather than passive and does not observe ongoing network behavior. Helpful alongside Scrye, not a substitute for continuous DNS-based monitoring.

### Open Source / Research Projects

#### Princeton IoT Inspector (retired)
- Python-based local traffic analysis of IoT devices via ARP spoofing
- Revealed pervasive tracking in smart TVs, speakers, etc.
- **Gaps:** Retired/unmaintained. ARP spoofing is fragile and disruptive. No continuous monitoring. No alerting or anomaly detection. Research-quality code. Closest conceptual predecessor to Scrye.

#### MUD / MUDGEE (UNSW Sydney)
- IETF RFC 8520: manufacturers declare what network access devices need
- MUDGEE reverse-engineers MUD profiles from observed traffic
- **Gaps:** Near-zero manufacturer adoption. Offline profile generation only. No real-time monitoring or anomaly detection. Academic proof-of-concept quality.

#### Netify Agent (OpenWrt)
- DPI engine using nDPI for application/protocol identification on routers
- Open-source agent (GPLv3)
- **Gaps:** Good dashboards and alerting locked behind commercial cloud service (Netify Informatics). Running locally requires building your own visualization. Router-bound, not Pi-friendly.

#### Home Assistant Network Integrations
- nmap tracker (presence detection), UniFi integration (bandwidth stats)
- **Gaps:** HA is a home automation orchestrator, not a network analyzer. No traffic analysis, no DNS logging, no behavioral detection. "Is device online?" not "what is device doing?"

#### JA3/JA4 Fingerprinting Libraries
- Open-source TLS client hello fingerprinting
- Useful as a component, not a standalone tool

#### Academic IoT Fingerprinting (various)
- UNSW, Princeton, Northeastern research showed IoT devices are classifiable from traffic metadata (DNS patterns, packet sizes, inter-arrival times) using lightweight ML (random forests)
- Confirmed feasible on constrained hardware
- All unmaintained proof-of-concept code

### Commercial Products

#### Firewalla ($319-699)
- Inline firewall/IDS, per-device flow visibility ("Flows"), ad blocking, VPN, geo-IP blocking
- No subscription (one-time purchase) — key to its success
- Active community (~30k+ subreddit)
- **Gaps:** Strongest conceptual competitor because it already sells per-device visibility in a consumer-friendly package. Its tradeoff is that the product is closed, appliance-bound, and app-centric. Visibility is stronger at the flow/firewall layer than Scrye, but less inspectable and less focused on DNS-driven privacy reporting.
- **Demand signal:** Users pay $300-700 for per-device network visibility. Strongest direct competitor conceptually.

#### Fingbox ($99 + subscription)
- Excellent device fingerprinting via crowdsourced recognition database
- Network scanning, bandwidth monitoring, new device alerts
- **Gaps:** Device identification is the standout strength. The product leans more toward discovery and alerts than ongoing behavioral analysis, and the cloud/subscription model is a meaningful tradeoff for the privacy-conscious audience Scrye targets.
- **Demand signal:** Device identification alone is valuable enough to monetize.

#### GlassWire ($29-99)
- Per-application network visualization on Windows/Android (endpoint agent)
- Beautiful timeline, "first connection" alerts
- **Gaps:** Great example of how much users value attribution and history, but it is endpoint software rather than network infrastructure. It cannot see unmanaged IoT devices, which is exactly where Scrye is meant to help.
- **Demand signal:** Users love visualization of behavior over time and per-entity attribution. Millions of downloads.

#### Bitdefender Box ($149 + subscription)
- Inline IDS/vulnerability scanning. Required subscription. Mediocre detection.
- **Lesson:** Subscription + poor detection = failure.

#### Cujo AI ($99 + subscription)
- ML-powered threat detection. Pivoted to B2B/ISP.
- **Lesson:** Tech works, consumer hardware business is hard. Don't be a security company — be a tool.

#### Norton Core ($279 + subscription)
- Router with built-in Norton security.
- **Lesson:** Vendor lock-in + subscription for a router = DOA.

#### Unifi Dream Machine ($189-499)
- DPI traffic classification, IDS/IPS (Suricata-based), per-device bandwidth
- **Gaps:** Widely deployed and good enough for broad traffic categories, but the visibility stays fairly high-level. Better for "which device used bandwidth?" than "which exact domains and trackers did this device contact?"

### Router/Firewall Platforms

#### pfSense / OPNsense
- ntopng package is the most capable open-source option but requires x86 hardware
- Suricata/Snort for IDS, pfBlockerNG for DNS blocking
- Complex to configure, not accessible to non-experts

#### OpenWrt
- collectd stats, nlbwmon bandwidth accounting, Netify agent for DPI
- Resource-constrained (128MB RAM routers). Fragmented ecosystem. No coherent privacy analyzer.

#### MikroTik
- Granular raw data (Torch, connection tracking, NetFlow export) but zero user-friendly analysis

---

## Validated Demand (What Users Pay For)

Ranked by strength of signal:

1. **Per-device network visibility** — "What is each device talking to?" ($300+ validated by Firewalla)
2. **Device discovery and identification** — "What IS on my network?" (Fing built a business on this)
3. **Blocking/filtering** — DNS-level ad/tracker blocking (Pi-hole proved demand)
4. **Alerting on anomalies** — new devices, unusual destinations, abnormal uploads
5. **Historical data and trends** — ability to look back in time
6. **No subscription** — every product that required subscriptions either died or lost goodwill

---

## Product Gaps (Where Scrye Fits)

### Gap 1: DNS as a Privacy Lens
Pi-hole and AdGuard Home have rich DNS query logs but do not analyze them. Nobody produces: "Your Roku made 847 requests to tracking domains in 24 hours, here's a breakdown by tracker category."

### Gap 2: Per-Device Behavioral Baselines
No lightweight open-source tool learns what "normal" looks like for each device and alerts on deviations. Firewalla has a crude "abnormal upload" threshold. Academic research (UNSW, Princeton) proved the approach works with lightweight ML on flow features. Nobody has productized it.

### Gap 3: Privacy-Framed Reporting
Every existing tool frames itself around "security" or "monitoring." Nobody says "here's your smart home privacy report card." No tool produces: "Your Samsung TV contacted 23 tracking domains today. Your robot vacuum uploaded 340MB to servers in 3 countries." The privacy framing is an open lane.

### Gap 4: Cross-Layer Correlation
DNS queries + flow data + device fingerprints = a complete per-device behavioral picture. These data sources exist separately (Pi-hole for DNS, Netify for DPI, JA3 for fingerprinting). Nobody integrates them in a lightweight package on Pi.

### Gap 5: Fully Local, No Cloud, Runs on a Pi
Firewalla needs its mobile app. Fing needs cloud. Netify's best features need cloud. The privacy-conscious users who want this tool are exactly the users who refuse to send network metadata to someone else's cloud.

---

## Technical Feasibility on Pi 4/5

### Comfortable
- DNS query logging and analysis
- TLS SNI extraction (see destinations even for encrypted traffic)
- Device fingerprinting (MAC OUI + DHCP + mDNS/SSDP + behavioral)
- Flow-level monitoring (connection metadata, not packet contents)
- Statistical anomaly detection (isolation forests, simple models on flow features)
- Historical storage in SQLite (weeks/months of flow-level data)
- Lightweight web dashboard

### Not Feasible
- Full packet capture at sustained high throughput (1Gbps+)
- Deep packet inspection at line rate
- Inline filtering/firewall (Pi 4 Ethernet is USB-attached)
- ntopng with full features (too memory-hungry)

---

## Key Technical Challenges

1. **Encrypted traffic:** Most traffic is TLS. No deep inspection without MITM (off the table). Limited to metadata: DNS queries, TLS SNI/JA3/JA4, certificate info, flow patterns, timing. This is still powerful for privacy analysis.

2. **Passive fingerprinting accuracy:** Without active scanning, device ID is fuzzier. Combining multiple passive signals (DHCP options, DNS patterns, MAC OUI, mDNS, SSDP/UPnP) can be surprisingly good but won't match NMAP's accuracy.

3. **Behavioral baselines with limited resources:** Statistical methods (isolation forests, moving averages, standard deviations) over ML. IoT behavioral patterns are regular enough that simple approaches work well.

4. **Traffic visibility:** Need to be on a network segment that sees traffic (mirror port, DNS proxy, or ARP-based interception). Deployment constraint to address early in design.

---

## Optimization

### Raspberry Pi (Resource-Limited) Priorities

1. Remove idle DB polling for live stream
   - Replace periodic SSE polling with writer-pushed stream events.
   - Keep DB reads only for initial catch-up via cursor (`after` / `Last-Event-ID`).
   - Expected impact: lower idle CPU wakeups and fewer SQLite reads.

2. Add SQLite connection and pragma tuning for Pi
   - Use `SetMaxOpenConns(1)` and `SetMaxIdleConns(1)` for SQLite.
   - Add/validate pragmas: `busy_timeout`, `temp_store=MEMORY`, `wal_autocheckpoint`, `journal_size_limit`, and bounded `cache_size`.
   - Expected impact: reduced lock contention and smoother SD-card I/O.

3. Make purge incremental instead of large delete bursts
   - Replace large daily deletes with chunked retention deletes (for example, 2k-10k rows per pass).
   - Expected impact: fewer periodic CPU and I/O spikes on small Pi hardware.

4. Add rollup tables for activity and anomaly reads
   - Precompute hourly and daily aggregates for charts and summary endpoints.
   - Expected impact: major CPU reduction as historical query volume grows.

5. Reduce memory footprint of OUI/vendor lookup
   - Load vendor prefix data lazily or move to a compact on-disk lookup structure.
   - Expected impact: lower baseline RSS, especially important on 1 GB Pis.
   - Additional notes:
     - Trigger to prioritize: sustained RSS pressure or OOM risk on Pi 3/Zero-class deployments.
     - Preferred approach: keep a compact prefix index on disk and cache only hot prefixes in memory.
     - Guardrail: avoid slower per-query attribution lookups on the DNS hot path.

6. Make passive discovery loops adaptive
   - Back off ARP/discovery cadence when traffic is idle and ramp up on activity.
   - Expected impact: steady-state CPU savings during low-activity periods.
   - Additional notes:
     - Trigger to prioritize: profiling shows discovery goroutines as a top idle CPU consumer.
     - Preferred approach: bounded backoff windows with immediate wakeups on observed DNS/client activity.
     - Guardrail: do not regress attribution freshness after DHCP churn or device reconnects.

---

## Design Principles (Inherited from Vigil)

- Lightweight, single binary
- SQLite persistence (WAL mode)
- Runs on Raspberry Pi
- Opinionated defaults, minimal configuration
- Passive deployment (not inline)
- Fully local, no cloud dependency
- Data export (JSON, CSV, API)

---

## Licensing Notes

### Decision

Scrye should use **Apache License 2.0**.

Reasoning:
- More protective and explicit than MIT while still staying permissive
- Better fit for a community-oriented Raspberry Pi / homelab project than AGPL
- Keeps adoption friction low for forks, self-hosting, and downstream packaging
- Supports a `NOTICE` file so attribution survives redistribution in a standard way
- Includes an explicit patent grant, which is cleaner for a product-shaped codebase than MIT

### Tradeoff Summary

- **MIT** is the lowest-friction option, but too barebones if the goal is broad reuse plus durable attribution
- **Apache-2.0** keeps the permissive model while improving legal hygiene and attribution handling
- **MPL-2.0** is an awkward middle ground here: more friction than Apache, but not enough protection to really solve the competitor concern
- **GPL-3.0** is stronger for distributed derivatives, but still does not address the network-service gap
- **AGPL-3.0** is the strongest defensive option for a server product, but likely imposes more friction than desired for a fun community project

### Practical Recommendation

- Use `Apache-2.0`
- Include a root `LICENSE`
- Include a root `NOTICE`
- Keep README license metadata aligned with the actual repo license

### References

- MIT summary: <https://choosealicense.com/licenses/mit/>
- Apache-2.0 summary: <https://choosealicense.com/licenses/apache-2.0/>
- MPL-2.0 summary: <https://choosealicense.com/licenses/mpl-2.0/>
- GPL-3.0 summary: <https://choosealicense.com/licenses/gpl-3.0/>
- AGPL-3.0 summary: <https://choosealicense.com/licenses/agpl-3.0/>
- Apache License 2.0 text: <https://www.apache.org/licenses/LICENSE-2.0.txt>
- Apache licensing FAQ: <https://www.apache.org/foundation/license-faq>
- GNU AGPL v3 text: <https://www.gnu.org/licenses/agpl-3.0.en.html>
- GNU GPL v3 text: <https://www.gnu.org/licenses/gpl-3.0.en.html>

---

## Scoping Decisions

### v1 (MVP): DNS Privacy Analyzer

**Status: Core implemented.** All foundational components are working. Remaining v1 work is UI polish and privacy-specific reporting.

**Data source:** Standalone DNS forwarder. Scrye is the network's DNS server, forwarding to upstream resolvers. Logs every query attributed to the originating device. Same deployment model as Pi-hole — users understand it.

**Output:** Web UI only. Embedded HTTP server. Privacy reports, per-device breakdowns, and historical trends are better suited to a browser than a terminal. No TUI — the data doesn't fit that format.

**Key distinction:** Scrye is NOT a DNS blocker / Pi-hole clone. The DNS forwarder is the collection mechanism, not the product. The product is the privacy insights layer on top.

**Device identification:** Passive only. IP + MAC + OUI vendor lookup + DHCP hostname + mDNS/SSDP broadcast discovery. No active scanning. Users can label devices via the web UI.

**Privacy insights (v1 scope):**
- Per-device query attribution — which device queried which domains
- Tracker/ad domain classification — categorize domains using community blocklists as a knowledge base (not for blocking, for labeling)
- Top talkers / summary stats — noisiest devices, tracker query percentages, daily breakdowns

**Domain classification:** Fetch and cache community blocklists (Steven Black, EasyList, etc.) as a classification database. Periodic refresh. User-defined overrides via web UI. Lists are used for labeling, not filtering.

**Configuration:** Web-first. Minimal bootstrap TOML (listen port, data directory, upstream DNS). Everything else configured through web UI and persisted to SQLite. No init wizard in v1.

**Storage:** Raw DNS query retention with time-based purge. Default 30 days. DNS logs are small (~50-100MB/month for active networks).

**Deployment:** Docker-first, config volume-mounted. Consistent with vigil.

#### v1 Implementation Status

| Component | Status | Notes |
|---|---|---|
| DNS forwarder (UDP + TCP) | **Done** | `internal/dns` — miekg/dns, sequential upstream fallback |
| Pipeline writer (async, batched) | **Done** | `internal/pipeline` — 100/batch, 1s flush, enriches with device + category |
| SQLite store (WAL mode) | **Done** | `internal/store` — pure-Go driver, schema auto-applied |
| ARP table polling | **Done** | `internal/device` — `/proc/net/arp`, 30s interval |
| DHCP hostname discovery | **Done** | `internal/device` — option 12 extraction from client requests |
| mDNS hostname discovery | **Done** | `internal/device` — PTR/SRV on 224.0.0.251:5353 |
| SSDP device discovery | **Done** | `internal/device` — SERVER header on 239.255.255.250:1900 |
| OUI vendor lookup | **Done** | `internal/device` — embedded ~39k entry IEEE database |
| Domain classification | **Done** | `internal/classify` — atomic pointer swap, lock-free reads |
| Classification list management | **Done** | Add/remove/refresh lists via API and settings page |
| Domain overrides | **Done** | User-defined overrides via API, persisted to SQLite |
| List caching in SQLite | **Done** | Lists cached locally, loaded from cache on startup |
| Bootstrap TOML config | **Done** | `internal/config` — listen, upstream, data_dir, http_port |
| Runtime settings in SQLite | **Done** | retention_days, list_refresh_hours via API/UI |
| Query retention + purge | **Done** | 30-day default, daily purge loop |
| Web dashboard | **Done** | Query volume, tracker %, top devices (last 24h) |
| Devices page | **Done** | All devices with query count and tracker % |
| Device detail page | **Done** | Per-device top domains, privacy summary, category breakdown |
| Domains page | **Done** | Top queried domains with category and device count |
| Settings page | **Done** | Retention, refresh interval, list management |
| REST API | **Done** | Read/write API for devices, lists, settings, and overrides; read APIs for queries and domains |
| Device labeling | **Done** | `PUT /api/devices/{mac}` updates label |
| Docker deployment | **Done** | Multi-stage build, host network, named volume |
| GitHub CI/CD | **Done** | CI, CodeQL, release workflows |
| Per-device privacy report | **Done** | Privacy summary stats + category breakdown on device detail page |
| Daily breakdown charts | **Done** | uPlot line charts on dashboard and device detail pages |
| Historical trend views | **Done** | Trend indicators on dashboard, devices, device detail |

### Post-MVP — Prioritized

#### Priority 1: Close the biggest product gaps

1. **DoH/DoT detection** — Identify devices bypassing local DNS. Highest-impact trust feature: if a device uses encrypted DNS, Scrye is blind to it and users should know.

2. **"New behavior" alerting** — Device contacted a never-before-seen domain. High-signal change detection with clear user value.

3. **Notifications** — Discord/webhook, ntfy.sh, quiet hours. Alerting is much less useful without delivery.

4. **Live query stream** — Real-time DNS query feed in web UI (SSE or websocket). Immediate visibility, strong demo value, and useful for debugging "what is this device doing right now?"

#### Priority 2: Strengthen the core privacy story

5. **Per-device privacy score** — Numeric score based on tracker frequency/diversity. Potentially a headline feature, but only if the scoring model is simple, explainable, and defensible.

6. **Destination country/org enrichment** — GeoIP and WHOIS org lookup. Adds context users immediately understand: "which company" and "which country" matter more than raw domain lists.

7. **CLI reports** — `scrye report` for stdout summaries, `scrye report --format json|csv` for scripting/cron, `scrye export` for bulk data. Useful for automation and power users, but secondary to the core UI and alerting loop.

#### Priority 3: Make longer-term use practical

8. **Rollup tables** — Hourly/daily summaries for long-term trend data without retaining raw queries.

9. **Configurable retention per data type** — Different retention for queries vs. summaries vs. device records.

10. **Downsampled storage** — 90+ day history at reduced granularity.

#### Priority 4: Expand visibility beyond DNS

11. **Passive packet capture** — Mirrored port or network bridge. Enables flow-level monitoring, TLS SNI extraction, JA3/JA4 fingerprinting, per-device bandwidth attribution. High upside, but also a major deployment and scope jump.

12. **Router flow export (NetFlow/IPFIX)** — Receive flow data from routers. Alternative to packet capture for managed router environments.

13. **DHCP fingerprinting** — Option ordering, vendor class. Better device identification, but less important than answering "what is this device doing?" clearly.

#### Priority 5: Nice to have

14. **Structured logging** — Migrate from `log` to `log/slog` (stdlib since Go 1.21). Good operational hygiene, but mostly internal value rather than user-visible product value.

15. **TLS fingerprinting (JA3/JA4)** — From passive packet capture. Interesting enrichment, but dependent on capture infrastructure and not needed to prove Scrye's main value.

16. **Behavioral clustering** — Traffic pattern-based device type inference. High complexity, hard to explain, and easy to overbuild before the core product is mature.

17. **Home Assistant integration** — MQTT or REST API for exposing device states and privacy scores.

18. **`scrye init` interactive setup wizard** — Guided first-run configuration.

19. **Pi-hole / AdGuard Home query log import** — Migrate existing data into Scrye.

20. **OpenAPI schema + API versioning plan** — Machine-readable API contract, client generation, and a cleaner path for future integrations without guessing at response shapes.
