# Contributing to Scrye

Thanks for your interest in contributing. Scrye is a small Raspberry Pi and homelab project, and focused contributions are welcome.

## Getting Started

1. Fork the repo and clone your fork
2. Make sure you have Go 1.26+ installed
3. Run `go test ./...` to verify everything passes
4. Create a branch for your change

## Local UI Preview

For UI and UX work, start Scrye with representative local demo data from the repo root:

```sh
./scrye-demo.sh
```

To wipe the demo database and reseed it from scratch:

```sh
./scrye-demo.sh --reset
```

The script:
- writes local config under `.demo/`
- writes local demo data under `.demo/tmpdata`
- starts the web UI on `http://localhost:8080`
- seeds demo data only into an empty database, so normal restarts do not duplicate records

## Submitting Changes

- Open a pull request against `main`
- Keep changes focused — one feature or fix per PR
- Include tests for new functionality
- Make sure `go test ./...` passes before submitting

## Reporting Bugs

Open an issue with:
- What you expected to happen
- What actually happened
- Steps to reproduce
- Your environment (OS, Go version, Docker version if relevant)

## Code Style

- Match the style of surrounding code
- Keep it simple — no over-engineering
- Run `gofmt` before committing

## Questions?

Open an issue.
