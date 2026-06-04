# Roadmap

Last updated: April 19, 2026

Umberrelay is actively evolving, and this roadmap shows where the project is headed next. Priorities may shift as we learn from real-world use, and items are intentionally not tied to fixed release dates.

This is more of a directional guide, not a delivery contract.

## Shipped

Core V1 and immediate follow-up work have shipped:

- Cross-subnet attribution fallback (source-IP actors when MAC is unavailable)
- Best-effort encrypted DNS bypass signal (`/api/bypass`)
- Live query stream via Server-Sent Events (`/api/queries/stream`)
- Hourly rollup aggregation infrastructure for activity trends (`query_rollups_hourly`)
- Statistical anomaly detection for tracker-rate and query-volume spikes (`/api/anomalies`)

---

## v2 - Trust Loop Completion

Completes the missing loop: detect meaningful behavior changes, explain them clearly, and notify reliably. The items below are the current focus areas that fit the DNS-forwarder scope.

### New behavior alerting

- Alert when a known device queries a never-before-seen domain
- Include first-seen timing and basic category context so alerts are actionable
- Start conservative to avoid alert fatigue

### Notifications

- Deliver alerts through webhook and `ntfy.sh` first, then Discord once payloads are stable
- Apply quiet hours and mute controls consistently across channels
- Deduplicate repetitive alerts in short windows

### Destination country and organization enrichment

- Add ASN/org and country context for top contacted destinations
- Cache enrichment metadata locally and tolerate missing lookups
- Keep enrichment optional to avoid unnecessary external coupling

---

## v3 - Operational Maturity and Automation

These items are worth implementing after v2 to improve interoperability, long-term operation, and maintainability.

### OpenAPI schema and API versioning plan

- Publish a machine-readable OpenAPI contract for existing endpoints
- Define additive-change and versioning rules before client ecosystem growth
- Keep first release focused on current API, not speculative endpoints

### CLI reports and export

- Add `umberrelay report` for human-readable and JSON/CSV output
- Add `umberrelay export` for bulk extraction workflows
- Reuse existing filter semantics from API/UI to avoid duplicate logic

### Data lifecycle controls

- Separate retention windows for raw queries, rollups, and device metadata
- Add daily summaries from hourly rollups for 90+ day trend visibility
- Keep sane defaults to prevent accidental storage growth
- Keep data model additive instead of introducing complex archival systems
- Maintain stable trend endpoints while evolving backend query paths

### CSRF protection for browser mutations

- Add CSRF token validation for state-changing browser requests
- Keep non-browser API usage behavior unchanged
- Align with existing reverse-proxy hardening guidance

### Structured logging

- Move runtime logging to `log/slog` with stable structured fields
- Preserve readable startup and shutdown messages
- Improve operational debugging in containerized deployments

---

## Backlog - Lower Priority

These are useful but not core to Umberrelay's near-term value delivery.

### Home Assistant integration

- Expose privacy summaries and alert state in automation-friendly form
- Start with pull-based integration before optional push workflows
- Keep integration optional and disabled by default

### Interactive setup (`umberrelay init`)

- Add guided first-run config generation and validation
- Keep generated config minimal and aligned with existing defaults
- Keep this lightweight since most runtime configuration lives in the UI

---

## Not Planned (Current Scope)

These items are not in current scope because they pull Umberrelay away from its DNS-forwarder boundary or introduce disproportionate complexity.

### Passive packet capture

- Significant deployment and runtime complexity increase
- Conflicts with the lightweight passive-DNS model

### Router flow export (NetFlow/IPFIX)

- High router/vendor compatibility burden
- Better suited to a separate flow-analysis product direction

### High-confidence encrypted DNS detection from flow metadata

- Depends on packet/flow collection that is out of current scope
- Existing best-effort bypass signaling remains the intended approach

### TLS fingerprinting (JA3/JA4)

- Requires packet-capture scope expansion and ongoing signature upkeep
- Low incremental value before flow tooling exists

### Behavioral clustering and inferred device typing

- High false-positive risk and difficult explainability
- Weak fit for the current clear, attribution-first privacy story

### DHCP fingerprinting

- Limited user-visible benefit relative to implementation complexity
- Revisit only if attribution accuracy becomes a recurring blocker

### Pi-hole / AdGuard Home historical import

- One-time migration utility with ongoing parser/support burden
- Lower ROI than strengthening native reporting and export paths
