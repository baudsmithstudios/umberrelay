# Umberrelay Roadmap

Umberrelay's MVP is feature-complete, with a short follow-up list below for the highest-value trust and visibility gaps before broader post-MVP expansion. After that, the roadmap should focus on strengthening the privacy story and expanding scope only where the added complexity is justified.

This roadmap is intentionally product-driven rather than implementation-driven. Features are ordered by expected user impact, not by technical novelty.

Priorities may change as Umberrelay matures. This roadmap reflects current product direction, not guaranteed release commitments.

## MVP Follow-Up

1. **Cross-subnet attribution fallback**
   Status: Completed on April 3, 2026.
   Surface source IP clearly when MAC attribution is unavailable so routed multi-VLAN deployments remain understandable without router-specific integrations. This closes a practical visibility gap exposed by real Pi testing.

2. **DoH/DoT detection**
   Identify devices bypassing local DNS. Start with a best-effort signal based on existing DNS and passive discovery data so users get immediate visibility into likely blind spots without requiring packet capture. This is the highest-impact trust feature: if a device uses encrypted DNS, Umberrelay is blind to it, and users should know that immediately.

3. **Live query stream**
   Add a real-time DNS query feed in the web UI, likely via SSE or WebSockets, with filtering by device, domain, and classification. This has immediate debugging value, strong demo value, and makes "what is this device doing right now?" easy to answer.

4. **New behavior alerting**
   Alert when a device contacts a never-before-seen domain. This is a high-signal change detection feature with clear user value and fits Umberrelay's privacy framing better than broad "security" alerts.

5. **Notifications**
   Add Discord, webhook, and `ntfy.sh` delivery with quiet hours. This should follow alert quality improvements, because delivery is much less useful if the signal is noisy or unclear.

## Post-MVP

### Priority 1: Strengthen The Core Privacy Story

1. **Per-device privacy score**
   Add a numeric privacy score based on tracker frequency and diversity. This could become a headline "privacy report card" feature, but only if the scoring model stays simple, explainable, and defensible.

2. **Destination country/org enrichment**
   Add GeoIP and organization/WHOIS context so users can understand which companies and countries devices are talking to. That framing is usually more useful than raw domain lists alone.

3. **CLI reports and export**
   Add `umberrelay report` for stdout summaries, `umberrelay report --format json|csv` for automation, and `umberrelay export` for bulk data export. This is valuable for scripting and power users, but secondary to the core UI and alerting loop.

### Priority 2: Make Longer-Term Use Practical

4. **Rollup tables**
   Add hourly and daily summary tables so long-term trend views do not require keeping all raw queries.

5. **Configurable retention per data type**
   Allow different retention windows for raw queries, summaries, and device records.

6. **Downsampled storage**
   Keep 90+ day history at reduced granularity for lighter-weight long-term reporting.

### Priority 3: Expand Visibility Beyond DNS

7. **Passive packet capture**
   Support mirrored-port or bridge-based capture for flow metadata, per-device bandwidth attribution, TLS SNI extraction, and later TLS fingerprinting. High upside, but also a major deployment and scope jump relative to the DNS-forwarder model.

8. **Router flow export**
   Accept NetFlow or IPFIX from capable routers as an alternative to local packet capture in managed router environments.

9. **High-confidence encrypted DNS detection**
   Add higher-confidence DoH/DoT attribution using flow metadata once packet capture or router flow export is available. This is the follow-on enhancement to the MVP best-effort bypass signal.

10. **DHCP fingerprinting**
   Improve device identification using DHCP option ordering and vendor class information. Helpful for attribution, but less important than making device behavior and privacy impact clear.

11. **Statistical behavioral baselines**
    Add explainable per-device anomaly baselines using simple statistical methods over historical DNS/flow metadata. This should stay transparent and lightweight; avoid opaque ML until the core signal is mature.

### Nice To Have

12. **Structured logging**
    Move from `log` to `log/slog` for leveled, structured logs. Good operational hygiene, but mostly internal value rather than a major user-facing feature.

13. **TLS fingerprinting**
    Add JA3/JA4-style enrichment once packet capture exists. Useful, but not necessary to prove Umberrelay's core value.

14. **Behavioral clustering**
    Infer device type from traffic patterns. This is interesting, but high-complexity, easy to overbuild, and should wait until simpler attribution and alerting work is strong.

15. **Home Assistant integration**
    Expose device states, privacy summaries, and future privacy scores via MQTT or API for home automation workflows.

16. **Interactive setup wizard**
    Add `umberrelay init` for guided first-run configuration.

17. **Pi-hole / AdGuard Home query import**
    Import historical query logs from existing DNS tools so users can migrate data instead of starting from zero.

18. **OpenAPI schema and API versioning plan**
    Add a machine-readable API contract and a clearer path for long-term client compatibility.
