# Umberrelay Roadmap

Umberrelay's MVP is complete. The next phase should focus on closing the biggest trust and visibility gaps first, then strengthening the privacy story, then expanding scope only where the added complexity is justified.

This roadmap is intentionally product-driven rather than implementation-driven. Features are ordered by expected user impact, not by technical novelty.

Priorities may change as Umberrelay matures. This roadmap reflects current product direction, not guaranteed release commitments.

## Priority 1: Close The Biggest Product Gaps

1. **DoH/DoT detection**
   Identify devices bypassing local DNS. If a device uses encrypted DNS, Umberrelay is blind to it, and users need to know that immediately.

2. **New behavior alerting**
   Alert when a device contacts a never-before-seen domain. This is one of the highest-signal changes Umberrelay can surface with the data it already has.

3. **Notifications**
   Add Discord, webhook, and `ntfy.sh` delivery with quiet hours. Alerts are much less useful if they only exist inside the UI.

4. **Live query stream**
   Add a real-time DNS query feed in the web UI with filtering by device, domain, and classification. This has immediate debugging value and strong demo value.

## Priority 2: Strengthen The Core Privacy Story

5. **Per-device privacy score**
   Add a numeric score based on tracker frequency and diversity. This should only ship if the scoring remains simple, explainable, and defensible.

6. **Destination country/org enrichment**
   Add GeoIP and organization context so users can understand who devices are talking to and where those services are hosted.

7. **CLI reports and export**
   Add `umberrelay report` and `umberrelay export` commands for summaries, automation, and scripting.

## Priority 3: Make Longer-Term Use Practical

8. **Rollup tables**
   Add hourly and daily summary tables so long-term trend views do not require keeping all raw queries.

9. **Configurable retention per data type**
   Allow different retention windows for raw queries, summaries, and device records.

10. **Downsampled storage**
    Keep 90+ day history at reduced granularity for lighter-weight long-term reporting.

## Priority 4: Expand Visibility Beyond DNS

11. **Passive packet capture**
    Support mirrored-port or bridge-based capture for flow metadata, TLS SNI extraction, and richer per-device visibility. High upside, but also a major deployment and scope change.

12. **Router flow export**
    Accept NetFlow or IPFIX from capable routers as an alternative to local packet capture.

13. **DHCP fingerprinting**
    Improve device identification using DHCP option ordering and vendor class information.

## Nice To Have

14. **Structured logging**
    Move to `log/slog` for leveled, structured logs. Good operational hygiene, but not a major user-facing feature.

15. **TLS fingerprinting**
    Add JA3/JA4-style enrichment once capture infrastructure exists. Useful, but not necessary to prove Umberrelay's core value.

16. **Behavioral clustering**
    Infer device type from traffic patterns. Interesting, but high complexity and easy to overbuild too early.

17. **Home Assistant integration**
    Expose device states and privacy scores via MQTT or API for home automation use.

18. **Interactive setup wizard**
    Add `umberrelay init` for guided first-run setup.

19. **Pi-hole / AdGuard Home query import**
    Import historical query logs from existing DNS tools.

20. **OpenAPI schema and API versioning plan**
    Add a machine-readable API contract and a clearer path for long-term client compatibility.
