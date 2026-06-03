# Security Policy

Umberrelay is a small, self-hosted Raspberry Pi and homelab project. It is being developed and maintained on my spare time.

## Reporting a Vulnerability

Please do not open a public issue for security vulnerabilities.

Use GitHub's private vulnerability reporting from the repository [Security tab](https://github.com/baudsmithstudios/umberrelay/security) — click **"Report a vulnerability"** and describe the issue, its impact, and how to reproduce it. This creates a private advisory visible only to you and the maintainers.

## Scope

Reports are most useful when they describe the affected code. For example, the DNS listener, the web UI/API, or the classification-list fetcher.

Umberrelay ships without authentication by design and is meant to run on a trusted network behind a reverse proxy (see [Access And Hardening](README.md#access-and-hardening)). "The unauthenticated API is reachable when exposed directly" is documented behavior, not a vulnerability.

General bugs, configuration questions, and documentation mistakes are best reported as regular issues.

## What to Expect

Response times vary. Confirmed issues are addressed as soon as is practical, and fixes land on the latest release. With your permission, you'll be credited in the advisory.
