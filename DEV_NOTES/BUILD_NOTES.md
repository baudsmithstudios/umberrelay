# Umberrelay v1 Build Notes

Umberrelay is a lightweight DNS privacy analyzer designed for Raspberry Pi homelabs. It operates as a DNS forwarder that logs queries per-device, classifies domains against community blocklists, and serves privacy insights through a web dashboard.

## Architecture

Single Go binary with an async pipeline architecture:

```
DNS Listener → buffered channel (4096) → Pipeline Writer → SQLite (WAL mode)
                                              ↓
                                     Device Tracker (IP→MAC)
                                     Classify Manager (domain→category)
```

All components run as goroutines coordinated via `context.Context`. Graceful shutdown on SIGTERM/SIGINT drains the pipeline before exiting.

### Key packages

| Package | Purpose |
|---------|---------|
| `cmd/umberrelay` | Entrypoint — wires all components, signal handling, purge goroutine |
| `internal/dns` | UDP+TCP DNS forwarder using `miekg/dns`, emits `QueryRecord` to channel |
| `internal/pipeline` | Batched async writer — enriches records (device, classification) and bulk-inserts |
| `internal/store` | SQLite via `modernc.org/sqlite` (pure Go, no CGO), WAL mode, all CRUD |
| `internal/device` | Passive device discovery: ARP polling, DHCP option 12, mDNS, SSDP |
| `internal/classify` | Domain classification against community blocklists with local overrides |
| `internal/config` | TOML bootstrap config with sensible defaults |
| `internal/web` | HTTP server with HTML pages (Go templates) and JSON API endpoints |

## Technology Choices

### Go (1.26+)

Single static binary, excellent concurrency primitives for the pipeline architecture, cross-compiles to ARM for Raspberry Pi. Go 1.22+ method routing (`GET /api/...`) eliminates the need for a third-party router.

### modernc.org/sqlite (pure Go)

CGO-free SQLite driver. Critical for easy cross-compilation to ARM without a C toolchain. WAL mode enables concurrent reads during writes.

### miekg/dns

The standard Go DNS library. Handles UDP and TCP serving/forwarding with full message parsing. Also used for mDNS packet parsing in device discovery.

### BurntSushi/toml

Minimal config parsing for the bootstrap config file. Chosen for simplicity — the app stores runtime settings in SQLite, so the config file only handles startup parameters.

### HTMX + Pico CSS

Server-rendered HTML with HTMX for dynamic interactions (list management, settings). Pico CSS provides classless styling. No JavaScript build toolchain required. uPlot is included for future charting.

### IEEE OUI Database

Embedded as a Go map literal (~39k entries) generated from the IEEE CSV. Provides vendor identification from MAC address prefixes without runtime network calls or external files.

## Device Discovery

Passive observation only — no active scanning:

- **ARP table**: Polls `/proc/net/arp` every 30 seconds for IP↔MAC mappings
- **DHCP**: Listens on UDP:67 for client requests, extracts hostname from option 12
- **mDNS**: Listens on 224.0.0.251:5353 for PTR/SRV records with hostnames
- **SSDP**: Listens on 239.255.255.250:1900 for UPnP SERVER announcements

## Domain Classification

Two-tier lookup: local overrides (user-defined) take precedence, then community blocklists. Lists are fetched periodically (default 24h) and cached in SQLite. Supports both hosts-file format and plain domain-list format.

Default lists: Steven Black Unified, EasyPrivacy, Disconnect.me Tracking.

## Deployment

Docker multi-stage build: `golang:1.26-alpine` for build, `alpine:3.19` for runtime. Host network mode is required for DNS port binding and multicast listeners.

There are now two compose paths:

- `docker-compose.yml` for local development and simple local Docker runs
- `docker-compose.pi.yml` for Raspberry Pi deployment with a prebuilt `umberrelay:latest` image transferred from a dev machine

For contributor-facing Pi deployment steps, see `docs/DEPLOYMENT.md`.

## Binary Size

~22MB, dominated by the embedded OUI database (~39k entries as a Go map literal).
