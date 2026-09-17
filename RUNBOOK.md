# VPN NOC Incident Runbook

This runbook defines the first-line checks, recovery actions, evidence requirements, and escalation boundaries for the lab. Commands assume the repository root as the working directory.

## Incident workflow

1. **Acknowledge:** record the alert name, start time, severity, affected probe, and current owner.
2. **Validate:** confirm the symptom from a second source before changing state.
3. **Scope:** determine whether the fault affects the tunnel, one protocol, one service, or monitoring itself.
4. **Diagnose:** collect interface, route, handshake, probe, log, and packet evidence.
5. **Recover:** apply the smallest reversible action that restores service.
6. **Verify:** confirm alerts resolve and every synthetic probe returns to `1`.
7. **Document:** complete the incident report and record follow-up actions.

Do not rotate keys, rebuild containers, or delete volumes during initial triage. Preserve evidence first.

## Fast triage

```bash
docker compose ps
./scripts/health-check.sh

docker compose exec -T site-a ip -brief address
docker compose exec -T site-a ip route
docker compose exec -T site-a wg show
docker compose exec -T site-a ping -c 4 10.77.0.2
docker compose exec -T site-a dig @10.77.0.2 service.internal
docker compose exec -T site-a curl -fsS http://10.77.0.2:8080/health
```

- Prometheus targets: <http://localhost:9090/targets>
- Prometheus alerts: <http://localhost:9090/alerts>
- Alertmanager: <http://localhost:9093>
- Grafana: <http://localhost:3000>

## VPNExporterUnavailable

**Meaning:** Prometheus cannot scrape the process translating `wg show all dump` into metrics.

**L1 checks**

```bash
curl -fsS http://localhost:9586/healthz
curl -fsS http://localhost:9586/metrics | head
docker compose ps vpn-exporter site-a
docker compose logs --tail=100 vpn-exporter
```

If `site-a` is healthy but the exporter is not, restart only the exporter:

```bash
docker compose restart vpn-exporter
```

**Escalate** when the process repeatedly exits, `wg` returns permission errors, or restarting would destroy evidence of an unexplained crash. Attach logs and container state.

## VPNTunnelInterfaceDown

**Meaning:** the expected `wg0` interface is missing from the site-a network namespace.

```bash
docker compose exec -T site-a ip link show wg0
docker compose exec -T site-a wg show all
docker compose logs --tail=100 site-a
```

Validate that `runtime/wireguard/site-a/wg0.conf` exists and is mounted read-only. Restart `site-a` only after collecting logs:

```bash
docker compose restart site-a vpn-exporter blackbox-exporter
```

**Escalate** for repeated `Operation not permitted`, kernel/module errors, or configuration rejection.

## VPNHandshakeMissing or Stale

**Meaning:** the peer is configured but has never completed a handshake, or the latest handshake is older than 180 seconds.

```bash
docker compose exec -T site-a wg show
docker compose exec -T site-b wg show
docker compose exec -T site-a ping -c 3 site-b
docker compose exec -T site-a tcpdump -ni eth0 udp port 51820 -c 10
```

Compare public keys, endpoint addresses, `AllowedIPs`, listen ports, and container clocks. A control-network ping with no UDP packets points to local configuration; outbound packets with no response points to the remote peer or return path.

Use `make recover` for a known drill. Otherwise escalate before rotating keys or replacing both peers.

## VPNPeerUnreachable

**Meaning:** the ICMP probe to `10.77.0.2` is failing.

```bash
docker compose exec -T site-a ping -c 4 10.77.0.2
docker compose exec -T site-a ip route get 10.77.0.2
docker compose exec -T site-a wg show
docker compose exec -T site-a tcpdump -ni wg0 icmp -c 10
```

If the handshake is healthy but packets do not return, inspect `AllowedIPs`, the remote interface state, and firewall rules. Correlate with DNS, TCP, and HTTP probes to define the blast radius.

## VPNServiceUnavailable

**Meaning:** the tunnel may be healthy, but TCP port `8080` or the HTTP health response is unavailable.

```bash
docker compose exec -T site-a nc -vz -w 3 10.77.0.2 8080
docker compose exec -T site-a curl -v --max-time 5 http://10.77.0.2:8080/health
docker compose exec -T site-b ps
docker compose logs --tail=100 site-b
```

If ICMP and DNS remain healthy, treat this as a service-layer incident. Recover a known drill with `make recover`; otherwise restart only `site-b` after capturing process and log evidence.

## VPNDNSResolutionFailed

**Meaning:** `service.internal` did not resolve to `10.77.0.2` through the tunnel.

```bash
docker compose exec -T site-a dig @10.77.0.2 service.internal A +time=2 +tries=1
docker compose exec -T site-a nc -zvu -w 2 10.77.0.2 53
docker compose exec -T site-b ps | grep dnsmasq
docker compose logs --tail=100 site-b
```

If IP and HTTP probes pass, keep the incident scoped to DNS. Verify listener binding, the `wg0` address, and the expected record before restarting the service.

## VPNProbeLatencyHigh

**Meaning:** ICMP probe duration has remained above 200 ms for two minutes.

```bash
docker compose exec -T site-a ping -c 20 10.77.0.2
docker compose exec -T site-a tc qdisc show dev wg0
docker compose exec -T site-a wg show
```

Record minimum, average, maximum, and packet loss. Correlate latency with traffic rate and service response time. Recover the lab delay drill with `make recover`.

**Escalate** when latency persists without local queueing or packet loss evidence, or when it affects multiple paths.

## Packet capture

Capture only the headers and duration needed to diagnose the incident. WireGuard payloads remain encrypted on the control network.

```bash
# Outer encrypted UDP traffic
docker compose exec -T site-a tcpdump -ni eth0 udp port 51820 -c 50

# Inner tunnel traffic
docker compose exec -T site-a tcpdump -ni wg0 -c 50
```

## Recovery verification

```bash
make recover
make health
make smoke
```

Confirm that:

- both peers report a current handshake;
- ICMP, TCP, HTTP, and DNS probes equal `1`;
- the Grafana dashboard is populated;
- firing alerts transition to resolved;
- the resolution is present in the webhook incident log.

## Escalation evidence

Attach the following to an L2 escalation:

- alert name, severity, start time, and affected component;
- `docker compose ps` output;
- `ip -brief address`, `ip route`, and `wg show` from both peers;
- relevant probe results and logs;
- a bounded packet capture when required;
- actions already attempted and their results;
- customer or service impact, if known.
