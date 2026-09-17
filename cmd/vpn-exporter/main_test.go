package main

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseDump(t *testing.T) {
	raw := "wg0\tprivate\tpublic\t51820\toff\n" +
		"wg0\tabcdefghijklmnop\t(none)\t172.30.0.3:51820\t10.77.0.2/32\t1700000000\t1234\t5678\t5\n"

	snap, err := parseDump([]byte(raw))
	if err != nil {
		t.Fatalf("parseDump returned an error: %v", err)
	}
	if len(snap.interfaces) != 1 || snap.interfaces[0] != "wg0" {
		t.Fatalf("unexpected interfaces: %#v", snap.interfaces)
	}
	if len(snap.peers) != 1 {
		t.Fatalf("expected one peer, got %d", len(snap.peers))
	}
	got := snap.peers[0]
	if got.latestHandshake != 1700000000 || got.receivedBytes != 1234 || got.transmittedBytes != 5678 {
		t.Fatalf("unexpected peer: %#v", got)
	}
}

func TestParseDumpRejectsMalformedInput(t *testing.T) {
	_, err := parseDump([]byte("wg0\ttoo\tshort\n"))
	if err == nil {
		t.Fatal("expected malformed input to fail")
	}
}

func TestMetricsHealthyPeer(t *testing.T) {
	now := time.Unix(1700000030, 0)
	snap := snapshot{
		interfaces: []string{"wg0"},
		peers: []peer{{
			interfaceName:    "wg0",
			publicKey:        "abcdefghijklmnop",
			endpoint:         "172.30.0.3:51820",
			allowedIPs:       "10.77.0.2/32",
			latestHandshake:  1700000000,
			receivedBytes:    1234,
			transmittedBytes: 5678,
		}},
	}
	got := metrics(now, snap, nil)
	for _, want := range []string{
		"vpn_exporter_scrape_error 0",
		"vpn_tunnel_up{interface=\"wg0\"} 1",
		"vpn_peer_handshake_established",
		"vpn_peer_last_handshake_age_seconds",
		" 30\n",
		"vpn_peer_received_bytes_total",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("metrics output missing %q:\n%s", want, got)
		}
	}
}

func TestMetricsScrapeFailure(t *testing.T) {
	got := metrics(time.Unix(0, 0), snapshot{}, errors.New("wg unavailable"))
	if !strings.Contains(got, "vpn_exporter_scrape_error 1") {
		t.Fatalf("expected scrape error metric:\n%s", got)
	}
	if !strings.Contains(got, "vpn_tunnel_up{interface=\"wg0\"} 0") {
		t.Fatalf("expected tunnel down metric:\n%s", got)
	}
}
