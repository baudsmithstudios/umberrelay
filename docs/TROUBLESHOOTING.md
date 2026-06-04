# Umberrelay Troubleshooting

Use this guide when client DNS queries aren't reaching Umberrelay, attribution looks wrong, or the UI doesn't reflect API state. The steps assume a Linux host (typically a Raspberry Pi) running Umberrelay and clients on routed subnets that should be using it for DNS.

Assumptions:
- container commands use Docker Compose v2 (`docker compose`)
- package install examples use Debian/Raspberry Pi OS (`apt`)

Before sharing logs or screenshots publicly, redact sensitive details like internal IPs, MAC addresses, hostnames, and domain history.

## Service Health

Before walking the DNS path, sanity-check the service itself.

### Read Umberrelay's logs

```sh
docker compose logs -f umberrelay
```

Startup errors (port 53 already bound, missing config, schema mismatches) surface here first. Keep this open in another terminal while reproducing a problem.

### Restart after config changes

`config.toml` is read at startup, so edits don't take effect until the container restarts:

```sh
docker compose restart umberrelay
```

### Where data lives

The SQLite database lives at `/data/umberrelay.db` inside the container, which maps to whatever host path your compose file bind-mounts (or names a volume for) at `/data`. When debugging persistence (did my queries survive a restart? is the DB the size I expect?), check that host path directly, not the path inside the container.

## DNS Path Validation

### 1. Confirm the client is querying Umberrelay

From a client on the network under test:

```sh
dig @<PI_IP> test-$(date +%s).example.com
```

Expected result:
- `status: NOERROR`
- `SERVER: <PI_IP>#53(<PI_IP>)`

If `dig` or `nslookup` is missing on Debian/Raspberry Pi OS:

```sh
sudo apt update
sudo apt install -y dnsutils
```

### 2. Confirm Umberrelay is listening on port 53

Run on the Pi host:

```sh
sudo ss -lntup | grep ':53'
```

Expected result:
- `umberrelay` is bound on `0.0.0.0:53` for UDP and TCP

Red flags:
- no process is listening on `:53`
- only `127.0.0.1:53` is listening
- another DNS service owns port 53

### 3. Confirm DNS packets are reaching the Pi host

Run on the Pi host:

```sh
ip -br addr
sudo tcpdump -ni any -vvv 'udp port 53 or tcp port 53'
```

Then run a fresh `dig @<PI_IP> ...` from the client.

If `tcpdump` is missing on Debian/Raspberry Pi OS:

```sh
sudo apt update
sudo apt install -y tcpdump
```

If `-i any` is too noisy, scope to the interface from `ip -br addr` above (e.g. `sudo tcpdump -ni eth0 -vvv 'udp port 53 or tcp port 53'`).

### 4. Confirm Umberrelay is storing queries

Run from any machine that can reach the web UI:

```sh
curl -s http://<PI_IP>:8080/api/health
curl -s http://<PI_IP>:8080/api/summary
curl -s "http://<PI_IP>:8080/api/queries?limit=20"
```

Expected result:
- `/api/health` returns `{"status":"ok"}`
- `TotalQueries` increases after test lookups
- `/api/queries` shows fresh domains and timestamps

### 5. Confirm the UI reflects the API

Open:

```text
http://<PI_IP>:8080/
```

Expected result:
- the Home page loads
- top-level query counters increase
- the domain list updates after fresh DNS activity

## Common Gotchas

### DoH/DoT bypass signals are best-effort

The bypass signal in the attention feed and `/api/bypass` is intentionally heuristic. It detects devices that look active on LAN but have gone quiet on local DNS, with higher confidence when historical encrypted-DNS bootstrap domains were observed.

Use it as a trust warning, not packet-level proof. If you need to verify a specific client:
- run direct DNS tests from that client (`dig @<PI_IP> ...`)
- confirm DHCP/router DNS policy is enforced
- capture host traffic on the Pi (`tcpdump`) before drawing conclusions

### Gateway-level features can silently intercept DNS

If a client `dig @<PI_IP> ...` succeeds but the Pi host sees nothing in `tcpdump`, the gateway between the client and the Pi may be intercepting DNS. Common culprits:
- built-in ad blocking or content filtering on the router
- DNS redirect or NAT rules that force traffic to the gateway's own resolver
- "secure DNS" features that proxy queries to the vendor's resolver

Disable those features (or carve out an exception for `<PI_IP>`) and re-test.

### Client DNS changes do not apply until the DHCP lease renews

If clients still query the old resolver after changing DHCP DNS, force lease renewal by reconnecting the client or rebooting it.

### Local Pi queries can hide network-path bugs

`dig @127.0.0.1 ...` on the Pi only proves Umberrelay works locally. Always test from at least one client on each routed subnet that should use the Pi.

### Run `tcpdump` on the Pi host, not in a container shell

If packet capture behaves strangely, confirm the host context and interface list:

```sh
hostname
ip -br addr
sudo tcpdump -D
```

### Cross-subnet attribution uses source-IP fallback

Across routed subnets the Pi rarely learns remote client MACs, so queries record `SourceIP` with `DeviceMAC` blank. Those appear as source-IP fallback actors in the Devices page and via `/api/actors`.

## Result Triage

| Symptom | Likely Cause |
|---|---|
| `dig @<PI_IP>` fails | firewall rule, wrong DHCP DNS, or client-side DNS override |
| `dig @<PI_IP>` succeeds, `tcpdump` on Pi sees nothing | gateway-level DNS interception, wrong capture context, or wrong host/interface assumption |
| `tcpdump` shows DNS packets, `/api/queries` is empty | query ingestion/storage issue |
| `/api/queries` has rows, UI is stale/empty | UI rendering/filter issue |
| `/api/queries` has `SourceIP` but blank `DeviceMAC` | expected routed-subnet behavior; verify source-IP fallback actor is present in UI or `/api/actors` |
