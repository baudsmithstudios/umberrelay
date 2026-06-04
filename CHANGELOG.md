# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Changed

- **Breaking:** API responses now use snake_case JSON keys (e.g. `device_mac`); update consumers relying on the old PascalCase keys.

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

[Unreleased]: https://github.com/baudsmithstudios/umberrelay/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/baudsmithstudios/umberrelay/releases/tag/v0.1.0
