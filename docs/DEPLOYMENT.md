# Raspberry Pi Deployment Guide

> **This is a developer/contributor reference.** It covers dev-machine builds, image transfer to a Pi, and live deployment testing. If you're setting up Umberrelay for the first time, the [README Quick Start](../README.md#quick-start) is the right place to begin.

## Prerequisites

- Raspberry Pi running a 64-bit Linux OS (Raspberry Pi OS Lite 64-bit recommended)
- Docker Engine with the Compose v2 plugin installed on the Pi
- A network where you can safely test a DNS server before pointing the whole LAN at it
- Docker with `buildx` available on your dev machine if you want to build off-device

## Deployment Model

Umberrelay is not just a dashboard. A live deployment means the Pi is running the DNS server your clients actually query.

That has a few consequences:

- Umberrelay needs to bind DNS on port `53`
- The provided container deployment uses host networking so it can see DNS traffic and the ARP table
- Passive discovery also listens on UDP `67`, `5353`, and `1900`
- Another service already using those ports on the Pi can reduce visibility or prevent startup

The bootstrap config in [`config.toml`](../config.toml) defaults to:

```toml
listen = "0.0.0.0:53"
upstream = ["1.1.1.1:53", "8.8.8.8:53"]
data_dir = "/data"
http_listen = "0.0.0.0"
http_port = 8080
```

## Building

### Option A — Build on the Pi

```sh
git clone https://github.com/baudsmithstudios/umberrelay.git
cd umberrelay
docker compose up -d
```

The checked-in `docker-compose.yml` has `build: .`, so Compose builds the image on first run. This is the simplest path, but it is slower on older Pi models.

### Option B — Build on a dev machine and deploy to the Pi

This is the recommended contributor workflow for building on a dev machine and deploying to a Pi.

One important difference: the checked-in [`docker-compose.yml`](../docker-compose.yml) is build-oriented (`build: .`). For image transfer from a dev machine, the Pi needs its own compose file that references the prebuilt image. Create `~/umberrelay/docker-compose.yml` on the Pi:

```yaml
services:
  umberrelay:
    image: umberrelay:latest
    container_name: umberrelay
    network_mode: host
    volumes:
      - ./config.toml:/etc/umberrelay/config.toml:ro
      # IMPORTANT: point this at persistent storage on the Pi, ideally not the SD card.
      # Update this path to match your external drive or other persistent mount point.
      - /path/to/external/drive/umberrelay:/data
    restart: unless-stopped
```

Build the ARM64 image tar on your dev machine:

```sh
# One-time: create a multi-platform builder
docker buildx create --use --name pibuilder

# Build for ARM64 and export the image as a tar
docker buildx build --platform linux/arm64 -t umberrelay:latest \
  --output type=docker,dest=umberrelay.tar .
```

Copy the image and config to the Pi (the Pi already has its own `docker-compose.yml` from the step above):

```sh
scp umberrelay.tar config.toml user@<pi-ip>:~/umberrelay/
```

On the Pi, load the image and start the container:

```sh
cd ~/umberrelay
docker load < umberrelay.tar
docker compose up -d
```

### Redeploying after changes

```sh
docker buildx build --platform linux/arm64 -t umberrelay:latest \
  --output type=docker,dest=umberrelay.tar .

scp umberrelay.tar user@<pi-ip>:~/umberrelay/ && \
  ssh user@<pi-ip> "cd ~/umberrelay && docker load < umberrelay.tar && docker compose up -d"
```

If you want a clean restart during redeploy:

```sh
ssh user@<pi-ip> "cd ~/umberrelay && docker compose down && docker load < umberrelay.tar && docker compose up -d"
```

## Persistent Storage

For Pi use, prefer a bind mount on external storage over a named Docker volume so the database does not live on the SD card. The Option B compose example above shows where the bind mount goes; this section covers picking the path.

Find the mount point:

```sh
lsblk -o NAME,MOUNTPOINT,FSTYPE,SIZE,LABEL
```

Create the data directory and use it as the host side of the `:/data` bind mount:

```sh
mkdir -p /your/mount/point/umberrelay
```

## Live Testing on the Pi

Do not treat `./scripts/umberrelay-demo.sh` as a live deployment test. Demo mode seeds fake data for UI review and skips the real DNS listener and passive discovery paths.

Use a staged test instead:

1. Deploy Umberrelay to the Pi without changing router DNS yet.
2. Confirm the web UI and API respond:
   ```sh
   curl http://<pi-ip>:8080/api/health
   ```
3. From another machine, send DNS directly to the Pi:
   ```sh
   dig @<pi-ip> example.com
   ```
4. Confirm the query appears in the UI or `/api/queries`.
5. Point a single test device at the Pi for DNS and browse normally.
6. Confirm attribution behavior and ongoing query logging:
   - same-subnet clients should usually resolve to device MAC entries
   - routed VLAN clients without MAC visibility should appear as source-IP fallback actors
7. Only then switch the router's LAN DNS to the Pi for whole-network coverage.

## Troubleshooting

### Container will not start

Check whether another service on the Pi already owns a required port:

```sh
sudo ss -lntup | grep -E ':(53|67|1900|5353|8080)\b'
```

Typical conflicts include:

- another DNS server already bound to port `53`
- a DHCP server already bound to UDP `67`
- another service already using port `8080`

### The UI works, but Umberrelay sees no real traffic

Make sure the client device is actually using the Pi for DNS:

```sh
dig @<pi-ip> example.com
```

If that works but normal browsing does not show up, the client may be using encrypted DNS, a hardcoded resolver, or direct IP traffic.

### Queries are visible, but attribution is weak

Umberrelay depends on passive signals from the host network namespace. Host networking is required for the provided Docker deployment, and some devices simply do not expose much identity information. Across routed VLANs, source-IP fallback actors are expected when MAC attribution is unavailable.

For deeper network-path troubleshooting and VLAN validation workflows, see [`TROUBLESHOOTING.md`](./TROUBLESHOOTING.md).

## Health Checklist

- [ ] Pi is running a 64-bit OS with Docker and Docker Compose installed
- [ ] ARM64 image built successfully on the dev machine or Pi
- [ ] Pi deployment compose file uses `image: umberrelay:latest` (Option B only)
- [ ] Config is mounted to `/etc/umberrelay/config.toml`
- [ ] Data directory is bind-mounted to persistent storage
- [ ] `docker compose up -d` starts cleanly on the Pi
- [ ] `http://<pi-ip>:8080/api/health` returns success
- [ ] `dig @<pi-ip> example.com` succeeds
- [ ] Queries appear in the UI or API
- [ ] Routed-subnet queries without MAC visibility appear as source-IP fallback actors in `/api/actors`
- [ ] A single-device DNS cutover works before changing router-wide DNS
