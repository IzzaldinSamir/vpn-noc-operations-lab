#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

echo "Checking Go formatting, tests, and vet findings"
unformatted="$(gofmt -l ./cmd)"
if [[ -n "$unformatted" ]]; then
  printf 'gofmt required for:\n%s\n' "$unformatted" >&2
  exit 1
fi
go test ./...
go vet ./...

echo "Checking shell syntax"
for script in scripts/*.sh lab/node/entrypoint.sh; do
  bash -n "$script"
done

echo "Checking JSON"
jq empty monitoring/grafana/dashboards/vpn-noc-overview.json

if ! command -v docker >/dev/null 2>&1; then
  if [[ "${REQUIRE_DOCKER:-0}" == "1" ]]; then
    echo "Docker is required but was not found" >&2
    exit 1
  fi
  echo "Docker not found; container configuration checks skipped."
  exit 0
fi

test -f runtime/wireguard/site-a/wg0.conf || go run ./cmd/keygen
docker compose config --quiet

echo "Checking Prometheus configuration and alert rules"
docker run --rm --entrypoint /bin/promtool \
  -v "${root}/monitoring/prometheus:/etc/prometheus:ro" \
  prom/prometheus:v3.13.2 check config /etc/prometheus/prometheus.yml
docker run --rm --entrypoint /bin/promtool \
  -v "${root}/monitoring/prometheus:/etc/prometheus:ro" \
  prom/prometheus:v3.13.2 test rules /etc/prometheus/rule_tests.yml

echo "Checking Alertmanager configuration"
docker run --rm --entrypoint /bin/amtool \
  -v "${root}/monitoring/alertmanager:/etc/alertmanager:ro" \
  prom/alertmanager:v0.33.1 check-config /etc/alertmanager/alertmanager.yml

echo "Validation passed."
