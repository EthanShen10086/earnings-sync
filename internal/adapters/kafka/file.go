package kafka

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

// File persists events as JSONL and mirrors Memory fan-out.
type File struct {
	path string
	mem  *Memory
	mu   sync.Mutex
}

// NewFile opens or creates a JSONL event log under path.
func NewFile(path string) (*File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f := &File{path: path, mem: NewMemory()}
	if err := f.reload(); err != nil {
		return nil, err
	}
	return f, nil
}

func (f *File) Backend() string { return "file" }

func (f *File) Close() error { return nil }

func (f *File) Ping(context.Context) error { return nil }

func (f *File) Len() int { return f.mem.Len() }

func (f *File) Snapshot() []Event { return f.mem.Snapshot() }

func (f *File) reload() error {
	fh, err := os.Open(f.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		f.mem.mu.Lock()
		if ev.Offset > f.mem.offset {
			f.mem.offset = ev.Offset
		}
		f.mem.events = append(f.mem.events, ev)
		f.mem.mu.Unlock()
	}
	return sc.Err()
}

func (f *File) Publish(ctx context.Context, topic, key string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.mem.Publish(ctx, topic, key, payload); err != nil {
		return err
	}
	snap := f.mem.Snapshot()
	if len(snap) == 0 {
		return nil
	}
	last := snap[len(snap)-1]
	fh, err := os.OpenFile(f.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	enc := json.NewEncoder(fh)
	return enc.Encode(last)
}

func (f *File) Subscribe(ctx context.Context, topic string, handler func(Event) error) error {
	return f.mem.Subscribe(ctx, topic, handler)
}
