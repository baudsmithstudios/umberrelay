#!/usr/bin/env bash

set -euo pipefail

script_dir=$(
  CDPATH= cd -- "$(dirname -- "$0")" && pwd
)
repo_root=$(
  CDPATH= cd -- "$script_dir/.." && pwd
)

demo_dir="$repo_root/.demo"
data_dir="$demo_dir/tmpdata"
config_path="$demo_dir/config.local.toml"
gocache_dir="/tmp/umberrelay-gocache"

dns_listen="${UMBERRELAY_DEMO_DNS_LISTEN:-127.0.0.1:1053}"
upstream_dns="${UMBERRELAY_DEMO_UPSTREAM:-1.1.1.1:53}"
http_port="${UMBERRELAY_DEMO_HTTP_PORT:-8080}"

reset_demo=false

for arg in "$@"; do
  case "$arg" in
    --reset)
      reset_demo=true
      ;;
    *)
      printf 'unknown argument: %s\n' "$arg" >&2
      printf 'usage: %s [--reset]\n' "$(basename "$0")" >&2
      exit 1
      ;;
  esac
done

mkdir -p "$demo_dir" "$gocache_dir"

if [ "$reset_demo" = true ]; then
  rm -rf "$data_dir"
fi

mkdir -p "$data_dir"

cat >"$config_path" <<EOF
listen = "$dns_listen"
upstream = ["$upstream_dns"]
data_dir = "$data_dir"
http_port = $http_port
EOF

printf 'starting Umberrelay demo UI on http://localhost:%s\n' "$http_port"
printf 'demo config: %s\n' "$config_path"
printf 'demo data dir: %s\n' "$data_dir"

cd "$repo_root"
GOCACHE="$gocache_dir" go run ./cmd/umberrelay -config "$config_path" -demo-data
