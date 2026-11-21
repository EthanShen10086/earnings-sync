package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileRunStore persists RunState snapshots and product events to JSON files.
// Engine should use Memory(); Put hooks flush runs to disk automatically.
type FileRunStore struct {
	mu         sync.Mutex
	mem        *RunStore
	runsPath   string
	eventsPath string
	events     []ProductEvent
	maxEvents  int
}

// OpenFileRunStore loads existing runs/events from disk (missing files are ok).
func OpenFileRunStore(runsPath, eventsPath string) (*FileRunStore, error) {
	if err := os.MkdirAll(filepath.Dir(runsPath), 0o755); err != nil && filepath.Dir(runsPath) != "." {
		return nil, err
	}
	if eventsPath != "" {
		if err := os.MkdirAll(filepath.Dir(eventsPath), 0o755); err != nil && filepath.Dir(eventsPath) != "." {
			return nil, err
		}
	}
	fs := &FileRunStore{
		mem:        NewRunStore(),
		runsPath:   runsPath,
		eventsPath: eventsPath,
		maxEvents:  2000,
	}
	if err := fs.loadRuns(); err != nil {
		return nil, err
	}
	if err := fs.loadEvents(); err != nil {
		return nil, err
	}
	fs.mem.OnPut = func(*RunState) {
		fs.mu.Lock()
		defer fs.mu.Unlock()
		_ = fs.persistRunsLocked()
	}
	return fs, nil
}

// Memory exposes the in-process RunStore used by Engine.Runs.
func (f *FileRunStore) Memory() *RunStore { return f.mem }

// Get delegates to the memory store.
func (f *FileRunStore) Get(runID string) (*RunState, bool) {
	return f.mem.Get(runID)
}

// ByClientRequestID delegates to the memory store.
func (f *FileRunStore) ByClientRequestID(clientID string) (*RunState, bool) {
	return f.mem.ByClientRequestID(clientID)
}

// List returns recent runs (newest first), capped at limit (default 50).
func (f *FileRunStore) List(limit int) []*RunState {
	if f == nil {
		return nil
	}
	return f.mem.List(limit)
}

// Append implements EventSink.
func (f *FileRunStore) Append(ev ProductEvent) error {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	f.events = append(f.events, ev)
	if f.maxEvents > 0 && len(f.events) > f.maxEvents {
		f.events = f.events[len(f.events)-f.maxEvents:]
	}
	return f.persistEventsLocked()
}

// Events returns a copy of product events (oldest first).
func (f *FileRunStore) Events(limit int) []ProductEvent {
	if f == nil {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if limit <= 0 || limit > len(f.events) {
		limit = len(f.events)
	}
	start := len(f.events) - limit
	out := make([]ProductEvent, limit)
	copy(out, f.events[start:])
	return out
}

func (f *FileRunStore) loadRuns() error {
	b, err := os.ReadFile(f.runsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var list []*RunState
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("load runs: %w", err)
	}
	for _, st := range list {
		if st != nil {
			// load without OnPut
			f.mem.mu.Lock()
			cp := cloneRun(st)
			f.mem.byID[st.RunID] = cp
			if st.ClientRequestID != "" {
				f.mem.byClient[st.ClientRequestID] = st.RunID
			}
			f.mem.mu.Unlock()
		}
	}
	return nil
}

func (f *FileRunStore) persistRunsLocked() error {
	list := f.mem.List(500)
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.runsPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.runsPath)
}

func (f *FileRunStore) loadEvents() error {
	if f.eventsPath == "" {
		return nil
	}
	b, err := os.ReadFile(f.eventsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if len(b) == 0 {
		return nil
	}
	var list []ProductEvent
	if err := json.Unmarshal(b, &list); err != nil {
		return fmt.Errorf("load events: %w", err)
	}
	f.events = list
	return nil
}

func (f *FileRunStore) persistEventsLocked() error {
	if f.eventsPath == "" {
		return nil
	}
	b, err := json.MarshalIndent(f.events, "", "  ")
	if err != nil {
		return err
	}
	tmp := f.eventsPath + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, f.eventsPath)
}
