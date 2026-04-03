# Umberrelay Roadmap

Umberrelay's MVP is feature-complete, with a short follow-up list below for the highest-value trust and visibility gaps before broader post-MVP expansion. After that, the roadmap should focus on strengthening the privacy story and expanding scope only where the added complexity is justified.

This roadmap is intentionally product-driven rather than implementation-driven. Features are ordered by expected user impact, not by technical novelty.

Priorities may change as Umberrelay matures. This roadmap reflects current product direction, not guaranteed release commitments.

## MVP Follow-Up

1. **DoH/DoT detection**
   Identify devices bypassing local DNS. If a device uses encrypted DNS, Umberrelay is blind to it, and users need to know that immediately.

2. **New behavior alerting**
   Alert when a device contacts a never-before-seen domain. This is one of the highest-signal changes Umberrelay can surface with the data it already has.

3. **Live query stream**
   Add a real-time DNS query feed in the web UI with filtering by device, domain, and classification. This has immediate debugging value and strong demo value.

4. **Notifications**
   Add Discord, webhook, and `ntfy.sh` delivery with quiet hours. Alerts are much less useful if they only exist inside the UI.

5. **Cross-subnet attribution fallback**
   Surface source IP clearly when MAC attribution is unavailable so routed multi-VLAN deployments still remain understandable without router-specific integrations.

## Post-MVP

### Priority 1: Strengthen The Core Privacy Story

1. **Per-device privacy score**
   Add a numeric score based on tracker frequency and diversity. This should only ship if the scoring remains simple, explainable, and defensible.

2. **Destination country/org enrichment**
   Add GeoIP and organization context so users can understand who devices are talking to and where those services are hosted.

3. **CLI reports and export**
   Add `umberrelay report` and `umberrelay export` commands for summaries, automation, and scripting.

### Priority 2: Make Longer-Term Use Practical

4. **Rollup tables**
   Add hourly and daily summary tables so long-term trend views do not require keeping all raw queries.

5. **Configurable retention per data type**
   Allow different retention windows for raw queries, summaries, and device records.

6. **Downsampled storage**
    Keep 90+ day history at reduced granularity for lighter-weight long-term reporting.

### Priority 3: Expand Visibility Beyond DNS

7. **Passive packet capture**
    Support mirrored-port or bridge-based capture for flow metadata, TLS SNI extraction, and richer per-device visibility. High upside, but also a major deployment and scope change.

8. **Router flow export**
    Accept NetFlow or IPFIX from capable routers as an alternative to local packet capture.

9. **DHCP fingerprinting**
    Improve device identification using DHCP option ordering and vendor class information.

### Nice To Have

10. **Structured logging**
    Move to `log/slog` for leveled, structured logs. Good operational hygiene, but not a major user-facing feature.

11. **TLS fingerprinting**
    Add JA3/JA4-style enrichment once capture infrastructure exists. Useful, but not necessary to prove Umberrelay's core value.

12. **Behavioral clustering**
    Infer device type from traffic patterns. Interesting, but high complexity and easy to overbuild too early.

13. **Home Assistant integration**
    Expose device states and privacy scores via MQTT or API for home automation use.

14. **Interactive setup wizard**
    Add `umberrelay init` for guided first-run setup.

15. **Pi-hole / AdGuard Home query import**
    Import historical query logs from existing DNS tools.

16. **OpenAPI schema and API versioning plan**
    Add a machine-readable API contract and a clearer path for long-term client compatibility.
