#!/usr/bin/env bash
set -euo pipefail

BIND_ADDRESS="${BIND_ADDRESS:-127.0.0.1}"

check_url() {
  local name="$1"
  local url="$2"
  if curl --fail --silent --show-error --max-time 5 "$url" >/dev/null; then
    printf '[OK]   %s\n' "$name"
  else
    printf '[FAIL] %s (%s)\n' "$name" "$url" >&2
    return 1
  fi
}

failures=0
check_url "Grafana" "http://${BIND_ADDRESS}:3000/api/health" || failures=$((failures + 1))
check_url "Prometheus" "http://${BIND_ADDRESS}:9090/-/ready" || failures=$((failures + 1))
check_url "Alertmanager" "http://${BIND_ADDRESS}:9093/-/ready" || failures=$((failures + 1))
check_url "VPN exporter" "http://${BIND_ADDRESS}:9586/healthz" || failures=$((failures + 1))
check_url "Blackbox exporter" "http://${BIND_ADDRESS}:9115/" || failures=$((failures + 1))
check_url "Incident webhook" "http://${BIND_ADDRESS}:8081/healthz" || failures=$((failures + 1))

if ((failures > 0)); then
  printf '\n%d health check(s) failed.\n' "$failures" >&2
  exit 1
fi

printf '\nAll published services are responding.\n'
