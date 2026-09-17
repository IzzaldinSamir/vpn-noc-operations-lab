#!/usr/bin/env bash
set -euo pipefail

cleanup() {
  wg-quick down wg0 >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

test -f /etc/wireguard/wg0.conf || {
  echo "missing /etc/wireguard/wg0.conf; run 'make init' first" >&2
  exit 1
}

wg-quick up wg0

if [[ "${ENABLE_HTTP:-false}" == "true" ]]; then
  mkdir -p /srv/www
  printf '{"status":"ok","site":"%s"}\n' "${SITE_NAME:-unknown}" > /srv/www/health
  busybox httpd -f -p 8080 -h /srv/www &
fi

if [[ "${ENABLE_DNS:-false}" == "true" ]]; then
  dnsmasq \
    --keep-in-foreground \
    --log-facility=- \
    --log-queries \
    --no-resolv \
    --server=1.1.1.1 \
    --interface=wg0 \
    --bind-interfaces \
    --address=/service.internal/10.77.0.2 &
fi

echo "${SITE_NAME:-vpn-site} ready: $(wg show wg0 public-key)"
while sleep 30; do
  wg show wg0 >/dev/null
done
