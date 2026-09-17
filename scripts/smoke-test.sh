#!/usr/bin/env bash
set -euo pipefail

BIND_ADDRESS="${BIND_ADDRESS:-127.0.0.1}"
PROMETHEUS="http://${BIND_ADDRESS}:9090"
GRAFANA="http://${BIND_ADDRESS}:3000"

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "required command not found: $1" >&2
    exit 1
  }
}

query() {
  curl --fail --silent --show-error --get \
    --data-urlencode "query=$1" \
    "${PROMETHEUS}/api/v1/query"
}

require_command curl
require_command jq

echo "[1/6] Checking service health"
./scripts/health-check.sh

echo "[2/6] Checking Prometheus targets"
curl --fail --silent --show-error "${PROMETHEUS}/api/v1/targets" |
  jq -e '
    [.data.activeTargets[]
      | select(.labels.job == "vpn-exporter" or (.labels.job | startswith("blackbox-")))] as $targets
    | ($targets | length) == 5
      and all($targets[]; .health == "up")
  ' >/dev/null

echo "[3/6] Checking the WireGuard interface and handshake"
query 'vpn_tunnel_up{interface="wg0"}' |
  jq -e '.data.result | length == 1 and .[0].value[1] == "1"' >/dev/null
query 'vpn_peer_handshake_established' |
  jq -e '.data.result | length == 1 and .[0].value[1] == "1"' >/dev/null

echo "[4/6] Checking all synthetic probes"
query 'probe_success{job=~"blackbox-.*"}' |
  jq -e '.data.result | length == 4 and all(.[]; .value[1] == "1")' >/dev/null

echo "[5/6] Checking alert-rule and dashboard provisioning"
curl --fail --silent --show-error "${PROMETHEUS}/api/v1/rules" |
  jq -e '[.data.groups[].rules[] | select(.type == "alerting")] | length == 9' >/dev/null
curl --fail --silent --show-error "${GRAFANA}/api/search?query=VPN%20NOC%20Operations" |
  jq -e 'any(.[]; .uid == "vpn-noc-overview")' >/dev/null

echo "[6/6] Generating tunnel traffic and checking counters"
docker compose exec -T site-a ping -c 3 -W 2 10.77.0.2 >/dev/null
sleep 2
query 'vpn_peer_transmitted_bytes_total' |
  jq -e '.data.result | length == 1 and (.[0].value[1] | tonumber) > 0' >/dev/null

echo "Smoke test passed: tunnel, probes, rules, dashboard, and traffic counters are operational."
