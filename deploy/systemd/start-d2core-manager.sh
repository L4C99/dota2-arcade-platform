#!/usr/bin/env bash
set -euo pipefail

: "${NODE_CONFIG:?NODE_CONFIG must name the Controller JSON config}"
: "${D2CORE_BIN:?D2CORE_BIN must name fixed d2core v0.1.1}"
test -f "$NODE_CONFIG"
test -x "$D2CORE_BIN"

# The Manager and Controller read the same authoritative local port range.
data_dir="$(jq -er '.d2coreDataDir | select(type=="string" and length>0)' "$NODE_CONFIG")"
port_min="$(jq -er '.network.localPortMin | select(type=="number" and .>=1 and .<=65535)' "$NODE_CONFIG")"
port_max="$(jq -er '.network.localPortMax | select(type=="number" and .>=1 and .<=65535)' "$NODE_CONFIG")"
if (( port_min > port_max )); then
  echo 'invalid Controller local game port range' >&2
  exit 1
fi

exec "$D2CORE_BIN" serve --data-dir "$data_dir" --port-min "$port_min" --port-max "$port_max"
