package main

import (
	"bytes"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type peer struct {
	interfaceName    string
	publicKey        string
	endpoint         string
	allowedIPs       string
	latestHandshake  int64
	receivedBytes    uint64
	transmittedBytes uint64
}

type snapshot struct {
	interfaces []string
	peers      []peer
}

func parseDump(raw []byte) (snapshot, error) {
	var result snapshot
	lines := bytes.Split(bytes.TrimSpace(raw), []byte("\n"))
	for lineNumber, line := range lines {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		fields := strings.Split(string(line), "\t")
		switch {
		case len(fields) == 5:
			result.interfaces = append(result.interfaces, fields[0])
		case len(fields) >= 9:
			handshake, err := strconv.ParseInt(fields[5], 10, 64)
			if err != nil {
				return snapshot{}, fmt.Errorf("line %d: invalid handshake: %w", lineNumber+1, err)
			}
			received, err := strconv.ParseUint(fields[6], 10, 64)
			if err != nil {
				return snapshot{}, fmt.Errorf("line %d: invalid received bytes: %w", lineNumber+1, err)
			}
			transmitted, err := strconv.ParseUint(fields[7], 10, 64)
			if err != nil {
				return snapshot{}, fmt.Errorf("line %d: invalid transmitted bytes: %w", lineNumber+1, err)
			}
			result.peers = append(result.peers, peer{
				interfaceName:    fields[0],
				publicKey:        fields[1],
				endpoint:         fields[3],
				allowedIPs:       fields[4],
				latestHandshake:  handshake,
				receivedBytes:    received,
				transmittedBytes: transmitted,
			})
		default:
			return snapshot{}, fmt.Errorf("line %d: unexpected WireGuard dump shape with %d fields", lineNumber+1, len(fields))
		}
	}
	return result, nil
}

func metricLabel(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `"`, `\"`)
	return strings.ReplaceAll(value, "\n", `\n`)
}

func peerID(publicKey string) string {
	if len(publicKey) <= 12 {
		return publicKey
	}
	return publicKey[:12]
}

func metrics(now time.Time, snap snapshot, scrapeErr error) string {
	var out strings.Builder
	out.WriteString("# HELP vpn_exporter_scrape_error Whether the WireGuard scrape failed.\n")
	out.WriteString("# TYPE vpn_exporter_scrape_error gauge\n")
	if scrapeErr != nil {
		out.WriteString("vpn_exporter_scrape_error 1\n")
	} else {
		out.WriteString("vpn_exporter_scrape_error 0\n")
	}
	out.WriteString("# HELP vpn_tunnel_up Whether the WireGuard interface is present.\n")
	out.WriteString("# TYPE vpn_tunnel_up gauge\n")
	if len(snap.interfaces) == 0 {
		out.WriteString("vpn_tunnel_up{interface=\"wg0\"} 0\n")
	} else {
		for _, name := range snap.interfaces {
			fmt.Fprintf(&out, "vpn_tunnel_up{interface=\"%s\"} 1\n", metricLabel(name))
		}
	}

	out.WriteString("# HELP vpn_peer_configured Whether a WireGuard peer is configured.\n")
	out.WriteString("# TYPE vpn_peer_configured gauge\n")
	out.WriteString("# HELP vpn_peer_handshake_established Whether the peer has completed a handshake.\n")
	out.WriteString("# TYPE vpn_peer_handshake_established gauge\n")
	out.WriteString("# HELP vpn_peer_last_handshake_age_seconds Seconds since the latest peer handshake.\n")
	out.WriteString("# TYPE vpn_peer_last_handshake_age_seconds gauge\n")
	out.WriteString("# HELP vpn_peer_received_bytes_total Bytes received from the peer.\n")
	out.WriteString("# TYPE vpn_peer_received_bytes_total counter\n")
	out.WriteString("# HELP vpn_peer_transmitted_bytes_total Bytes transmitted to the peer.\n")
	out.WriteString("# TYPE vpn_peer_transmitted_bytes_total counter\n")

	for _, item := range snap.peers {
		labels := fmt.Sprintf("interface=\"%s\",peer=\"%s\",endpoint=\"%s\",allowed_ips=\"%s\"",
			metricLabel(item.interfaceName), metricLabel(peerID(item.publicKey)), metricLabel(item.endpoint), metricLabel(item.allowedIPs))
		fmt.Fprintf(&out, "vpn_peer_configured{%s} 1\n", labels)
		if item.latestHandshake == 0 {
			fmt.Fprintf(&out, "vpn_peer_handshake_established{%s} 0\n", labels)
			fmt.Fprintf(&out, "vpn_peer_last_handshake_age_seconds{%s} 0\n", labels)
		} else {
			age := now.Unix() - item.latestHandshake
			if age < 0 {
				age = 0
			}
			fmt.Fprintf(&out, "vpn_peer_handshake_established{%s} 1\n", labels)
			fmt.Fprintf(&out, "vpn_peer_last_handshake_age_seconds{%s} %d\n", labels, age)
		}
		fmt.Fprintf(&out, "vpn_peer_received_bytes_total{%s} %d\n", labels, item.receivedBytes)
		fmt.Fprintf(&out, "vpn_peer_transmitted_bytes_total{%s} %d\n", labels, item.transmittedBytes)
	}
	out.WriteString("# HELP vpn_exporter_build_info Static build information.\n")
	out.WriteString("# TYPE vpn_exporter_build_info gauge\n")
	out.WriteString("vpn_exporter_build_info{version=\"1.0.0\"} 1\n")
	return out.String()
}

func collect() (snapshot, error) {
	output, err := exec.Command("wg", "show", "all", "dump").Output()
	if err != nil {
		return snapshot{}, fmt.Errorf("run wg: %w", err)
	}
	return parseDump(output)
}

func main() {
	address := os.Getenv("LISTEN_ADDRESS")
	if address == "" {
		address = ":9586"
	}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	http.HandleFunc("/metrics", func(w http.ResponseWriter, _ *http.Request) {
		snap, err := collect()
		if err != nil {
			log.Printf("WireGuard scrape failed: %v", err)
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(metrics(time.Now(), snap, err)))
	})

	log.Printf("vpn-exporter listening on %s", address)
	if err := http.ListenAndServe(address, nil); err != nil {
		log.Fatal(err)
	}
}
