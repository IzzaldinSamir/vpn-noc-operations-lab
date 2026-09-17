#!/usr/bin/env bash
set -euo pipefail

scenario="${1:-}"

usage() {
  cat <<'EOF'
Usage: ./scripts/drill.sh SCENARIO

Scenarios:
  peer-down       Stop site-b and trigger connectivity, service, DNS, and handshake alerts.
  service-down    Stop the HTTP endpoint while keeping the VPN tunnel available.
  dns-failure     Stop dnsmasq while keeping the tunnel and HTTP endpoint available.
  high-latency    Add 300 ms delay to wg0 on site-a and trigger the latency warning.
  recover         Remove fault injection and restore both sites.
EOF
}

case "$scenario" in
  peer-down)
    docker compose stop site-b
    echo "site-b stopped. Watch Prometheus and Alertmanager for the correlated incident."
    ;;
  service-down)
    docker compose exec -T site-b pkill -f 'httpd -f' || true
    echo "site-b HTTP service stopped. The TCP and HTTP probes should fail."
    ;;
  dns-failure)
    docker compose exec -T site-b pkill dnsmasq || true
    echo "site-b DNS stopped. The DNS probe should fail while the tunnel remains healthy."
    ;;
  high-latency)
    docker compose exec -T site-a tc qdisc replace dev wg0 root netem delay 300ms
    echo "300 ms delay added to wg0. The round-trip probe should cross the alert threshold."
    ;;
  recover)
    docker compose start site-a site-b >/dev/null
    docker compose exec -T site-a tc qdisc del dev wg0 root 2>/dev/null || true
    docker compose restart site-b >/dev/null
    echo "Fault injection removed. Waiting for service recovery..."
    sleep 8
    docker compose ps
    ;;
  *)
    usage >&2
    exit 2
    ;;
esac
