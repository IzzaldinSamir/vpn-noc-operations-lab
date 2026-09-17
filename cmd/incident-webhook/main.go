package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type sink struct {
	path string
	mu   sync.Mutex
}

func (s *sink) append(raw json.RawMessage) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(s.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	record := struct {
		ReceivedAt string          `json:"received_at"`
		Payload    json.RawMessage `json:"payload"`
	}{time.Now().UTC().Format(time.RFC3339), raw}
	encoded, err := json.Marshal(record)
	if err != nil {
		return err
	}
	_, err = file.Write(append(encoded, '\n'))
	return err
}

func (s *sink) tail(limit int) ([]json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	file, err := os.Open(s.path)
	if os.IsNotExist(err) {
		return []json.RawMessage{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	var lines []json.RawMessage
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := append([]byte(nil), scanner.Bytes()...)
		lines = append(lines, json.RawMessage(line))
		if len(lines) > limit {
			lines = lines[1:]
		}
	}
	return lines, scanner.Err()
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--healthcheck" {
		response, err := http.Get("http://127.0.0.1:8081/healthz")
		if err != nil {
			log.Fatal(err)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			log.Fatalf("unhealthy status: %s", response.Status)
		}
		return
	}

	address := os.Getenv("LISTEN_ADDRESS")
	if address == "" {
		address = ":8081"
	}
	path := os.Getenv("INCIDENT_LOG")
	if path == "" {
		path = "/data/alerts.jsonl"
	}
	storage := &sink{path: path}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = io.WriteString(w, "ok\n")
	})
	http.HandleFunc("/alerts", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			defer r.Body.Close()
			body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
			if err != nil || !json.Valid(body) {
				http.Error(w, "invalid JSON payload", http.StatusBadRequest)
				return
			}
			if err := storage.append(json.RawMessage(body)); err != nil {
				log.Printf("persist alert: %v", err)
				http.Error(w, "could not persist alert", http.StatusInternalServerError)
				return
			}
			log.Printf("recorded Alertmanager webhook (%d bytes)", len(body))
			w.WriteHeader(http.StatusNoContent)
		case http.MethodGet:
			lines, err := storage.tail(20)
			if err != nil {
				http.Error(w, "could not read alerts", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(lines)
		default:
			w.Header().Set("Allow", strings.Join([]string{http.MethodGet, http.MethodPost}, ", "))
			http.Error(w, fmt.Sprintf("method %s not allowed", r.Method), http.StatusMethodNotAllowed)
		}
	})

	log.Printf("incident webhook listening on %s and writing %s", address, path)
	if err := http.ListenAndServe(address, nil); err != nil {
		log.Fatal(err)
	}
}
