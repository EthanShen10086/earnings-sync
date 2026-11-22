// Package observability hosts log shipping and alert-rule loading helpers
// for ELK / Prometheus stubs. Not a full observability product surface.
package observability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// LogEvent is one structured log line ready for file / Elasticsearch.
type LogEvent struct {
	Timestamp time.Time         `json:"@timestamp"`
	Level     string            `json:"level"`
	Message   string            `json:"message"`
	Service   string            `json:"service,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
}

// Shipper writes JSON logs to a file and optionally POSTs to an ES-compatible stub.
type Shipper struct {
	filePath string
	esURL    string // e.g. http://127.0.0.1:9200 — empty skips HTTP
	index    string
	service  string

	mu     sync.Mutex
	file   *os.File
	sent   atomic.Int64
	failed atomic.Int64
	client *http.Client
}

// ShipperStatus is JSON-friendly health for /v1/platform/status.
type ShipperStatus struct {
	Backend    string `json:"backend"`
	Degraded   bool   `json:"degraded"`
	Configured bool   `json:"configured"`
	FilePath   string `json:"file_path,omitempty"`
	ESURL      string `json:"es_url,omitempty"`
	Sent       int64  `json:"sent"`
	Failed     int64  `json:"failed"`
	Detail     string `json:"detail,omitempty"`
}

// OpenShipper creates a shipper. filePath is required for the file backend;
// esURL may be empty (file-only) or point at compose Elasticsearch / a stub.
func OpenShipper(filePath, esURL, index, service string) (*Shipper, ShipperStatus, error) {
	filePath = filepath.Clean(filePath)
	if filePath == "." || filePath == "" {
		filePath = "data/logs/app.jsonl"
	}
	if index == "" {
		index = "earnings-sync"
	}
	if service == "" {
		service = "earnings-sync"
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return nil, ShipperStatus{}, err
	}
	fh, err := os.OpenFile(filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, ShipperStatus{}, err
	}
	s := &Shipper{
		filePath: filePath,
		esURL:    esURL,
		index:    index,
		service:  service,
		file:     fh,
		client:   &http.Client{Timeout: 2 * time.Second},
	}
	st := s.Status()
	return s, st, nil
}

// Status snapshot.
func (s *Shipper) Status() ShipperStatus {
	if s == nil {
		return ShipperStatus{Backend: "noop", Detail: "shipper unset"}
	}
	st := ShipperStatus{
		FilePath:   s.filePath,
		ESURL:      s.esURL,
		Sent:       s.sent.Load(),
		Failed:     s.failed.Load(),
		Configured: true,
	}
	switch {
	case s.esURL != "":
		st.Backend = "file+es"
		st.Detail = "shipping to file and Elasticsearch-compatible endpoint"
		st.Degraded = false
	default:
		st.Backend = "file"
		st.Detail = "ELASTICSEARCH_URL unset — file shipper only (runnable)"
		// File-only is the intended stub when ES is not configured — not a dial failure.
		st.Degraded = false
	}
	return st
}

// Ship writes one event to file and optionally to ES.
func (s *Shipper) Ship(ctx context.Context, ev LogEvent) error {
	if s == nil {
		return nil
	}
	if ev.Timestamp.IsZero() {
		ev.Timestamp = time.Now().UTC()
	}
	if ev.Service == "" {
		ev.Service = s.service
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		s.failed.Add(1)
		return err
	}
	s.mu.Lock()
	_, err = s.file.Write(append(raw, '\n'))
	s.mu.Unlock()
	if err != nil {
		s.failed.Add(1)
		return err
	}
	s.sent.Add(1)

	if s.esURL == "" {
		return nil
	}
	url := fmt.Sprintf("%s/%s/_doc", trimSlash(s.esURL), s.index)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	if err != nil {
		s.failed.Add(1)
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		// File already wrote — treat ES failure as soft (degraded path).
		s.failed.Add(1)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		s.failed.Add(1)
	}
	return nil
}

// Close flushes the file handle.
func (s *Shipper) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.file.Close()
	s.file = nil
	return err
}

// ESStubHandler is a minimal Elasticsearch-compatible indexer for local demos.
// POST /{index}/_doc stores the body; GET /_cluster/health returns green.
func ESStubHandler() http.Handler {
	var mu sync.Mutex
	docs := map[string][]json.RawMessage{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /_cluster/health", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "green", "cluster_name": "earnings-stub"})
	})
	mux.HandleFunc("POST /{index}/_doc", func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		var raw json.RawMessage
		if err := json.NewDecoder(r.Body).Decode(&raw); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		docs[index] = append(docs[index], raw)
		n := len(docs[index])
		mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"result": "created", "index": index, "_seq": n})
	})
	mux.HandleFunc("GET /{index}/_count", func(w http.ResponseWriter, r *http.Request) {
		index := r.PathValue("index")
		mu.Lock()
		n := len(docs[index])
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"count": n})
	})
	return mux
}

func trimSlash(s string) string {
	for len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	return s
}
