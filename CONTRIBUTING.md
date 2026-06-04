# Contributing to Umberrelay

Thanks for your interest in contributing. Umberrelay is a small Raspberry Pi and homelab project where focused contributions are welcome.

## Getting Started

1. Fork the repo and clone your fork
2. Make sure you have the Go version required by `go.mod` installed
3. Run `go test ./...` to verify everything passes
4. Create a branch for your change

## Raspberry Pi Deployment

For cross-compilation, dev-machine image builds, and live Pi testing, see [docs/DEPLOYMENT.md](docs/DEPLOYMENT.md).

## Local UI Preview

For UI and UX work, start Umberrelay with representative local demo data from the repo root:

```sh
./scripts/umberrelay-demo.sh
```

To wipe the demo database and reseed it from scratch:

```sh
./scripts/umberrelay-demo.sh --reset
```

The script:
- writes local config under `.demo/`
- writes local demo data under `.demo/tmpdata`
- starts the web UI on `http://localhost:8080`
- seeds demo data only into an empty database, so normal restarts do not duplicate records

## Submitting Changes

- Open a pull request against `main`
- Keep changes focused — one feature or fix per PR
- Include tests for any new functionality
- Make sure `go build ./...`, `go test ./...`, and `go vet ./...` pass before submitting
- CI also runs `govulncheck`; you can check locally with `go run golang.org/x/vuln/cmd/govulncheck@latest ./...`

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

Please open an issue.
