# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

## [0.3.0] - 2026-10-27

### Changed

- The web UI is rebuilt on ElumKit as one Activity view: a device and source roster beside a detail pane with Overview, Domains and Live tabs.
- Tracker rate change against the prior 7 days is shown in points ("+3 pts vs prior 7d").
- Volume spike flags show their multiple of the daily average ("4.2x daily avg"); bypass flags show silent time ("2h 3m silent").
- Blocklist matches are listed per category with share and change; telemetry is no longer counted as unclassified.
- The tracker-rate chart plots one series with an average line and tooltips.
- The footer shows the release version, or `dev` for other builds.

### Fixed

- Unclassified domains no longer preselect "tracking" in the override menu.

### Removed

- The stacked category bar.
- The `/devices` and `/domains` pages; actor pages stay at `/devices/{actorKey}`.
- Pico CSS and the unused uPlot assets.

### Security

- Updated `x/net` to 0.62.0 for advisories in `x/net/html`, which Umberrelay does not use.

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

[Unreleased]: https://github.com/baudsmithstudios/umberrelay/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/baudsmithstudios/umberrelay/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/baudsmithstudios/umberrelay/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/baudsmithstudios/umberrelay/releases/tag/v0.1.0
