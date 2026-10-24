# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- The web UI is rebuilt on ElumKit. Home, Devices and Device Detail merge into one Activity view: a roster of devices and unattributed sources beside a detail pane for the whole network or the selected actor, with Overview, Domains and Live tabs.
- The stacked category bar is removed. The tracker rate's change against the prior 7 days is shown in points (for example "+3 pts vs prior 7d") in the 24h line of the network, device and source views, on roster rows, and on tracker-spike flags.
- Needs-attention flags lead with a measurement: volume spikes show their multiple of the daily average ("4.2x daily avg") and DNS bypass flags show how long the device has been silent ("2h 3m silent").
- Blocklist matches are listed per category (tracking, advertising, malware, analytics, telemetry) with share and change against the prior 7 days; telemetry is no longer counted as unclassified.
- The tracker-rate chart plots one series on one axis, with an average line and hover/keyboard tooltips.
- `/devices` and `/domains` are removed; device and source pages stay at `/devices/{actorKey}`.
- The footer shows the running version from the release tag (for example `v0.2.0`), or `dev` for builds that are not releases, instead of a hardcoded number.

### Fixed

- Unclassified domains no longer preselect "tracking" in the override menu.

### Removed

- Pico CSS and the unused uPlot assets.

## [0.2.0] - 2026-10-18

### Changed

- **Breaking:** API responses now use snake_case JSON keys (e.g. `device_mac`); update consumers relying on the old PascalCase keys.
- The live query stream starts at the newest query unless `after` or `Last-Event-ID` is given.

### Fixed

- One live stream client's `after` cursor no longer hides new queries from other clients.
- The live query stream connects immediately instead of after up to 15 seconds.
- Retention now purges every expired query, not just 900 per day.
- Queries received just before shutdown are no longer lost.

### Security

- Patched stdlib and `x/net` advisories (Go 1.26.9, x/net 0.60.0).
- Docker image now runs on `alpine:3.22`; `3.19` is past end of support.

## [0.1.0] - 2026-04-19

Initial public release.

### Added

- Forwarding DNS server (UDP + TCP) with sequential upstream fallback
- Per-device attribution via passive ARP, DHCP, mDNS, and SSDP; source-IP fallback for cross-subnet clients
- Domain classification against configurable blocklists (Steven Black, EasyPrivacy, Disconnect.me) with manual overrides
- Web UI: Home, Devices, per-device detail, and Settings pages
- Live query stream over Server-Sent Events
- Statistical anomaly detection for tracker-rate and query-volume spikes
- Best-effort signal for devices bypassing local DNS (DoH/DoT)
- REST API covering actors, devices, queries, activity, anomalies, bypass, domains, lists, settings, and overrides
- SQLite persistence (WAL) with hourly rollups and configurable retention
- `--version` flag
- Docker deployment (`network_mode: host`); ARM64 build for Raspberry Pi

[Unreleased]: https://github.com/baudsmithstudios/umberrelay/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/baudsmithstudios/umberrelay/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/baudsmithstudios/umberrelay/releases/tag/v0.1.0
