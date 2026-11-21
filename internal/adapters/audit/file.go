package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// FileLogger appends JSONL events to a local file.
type FileLogger struct {
	path string
	mu   sync.Mutex
}

// NewFileLogger ensures the parent directory exists.
func NewFileLogger(path string) (*FileLogger, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		path = "data/audit.jsonl"
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return &FileLogger{path: path}, nil
}

func (f *FileLogger) Backend() string { return "file" }

func (f *FileLogger) Close() error { return nil }

func (f *FileLogger) Append(_ context.Context, e Event) error {
	if e.At.IsZero() {
		e.At = time.Now().UTC()
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.OpenFile(f.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = file.Write(append(b, '\n'))
	return err
}

func (f *FileLogger) List(_ context.Context, limit int) ([]Event, error) {
	if limit <= 0 {
		limit = 100
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	file, err := os.Open(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer file.Close()
	var all []Event
	sc := bufio.NewScanner(file)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	if len(all) > limit {
		all = all[len(all)-limit:]
	}
	return all, nil
}
