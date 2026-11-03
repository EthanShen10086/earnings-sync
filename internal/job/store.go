package job

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/EthanShen10086/earnings-sync/internal/model"
	"github.com/google/uuid"
)

// Store is a JSON file of jobs. It is the idempotency log and the debug record.
type Store struct {
	path   string
	mu     sync.Mutex
	jobs   map[string]*model.Job
	byHash map[string]string
	byLoc  map[string]string
}

func Open(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return nil, err
	}
	s := &Store{
		path:   path,
		jobs:   map[string]*model.Job{},
		byHash: map[string]string{},
		byLoc:  map[string]string{},
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s, nil
		}
		return nil, err
	}
	if len(b) == 0 {
		return s, nil
	}
	var list []*model.Job
	if err := json.Unmarshal(b, &list); err != nil {
		return nil, err
	}
	for _, j := range list {
		s.jobs[j.ID] = j
		if j.Status == model.StatusSucceeded && j.DocHash != "" {
			s.byHash[j.DocHash] = j.ID
		}
		if j.Status == model.StatusSucceeded && j.Locator != "" {
			s.byLoc[j.Locator] = j.ID
		}
	}
	return s, nil
}

func (s *Store) Create(ticker, hash, locator, source string, dry bool) *model.Job {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	j := &model.Job{
		ID:        uuid.NewString(),
		Ticker:    ticker,
		Status:    model.StatusRunning,
		DocHash:   hash,
		Locator:   locator,
		Source:    source,
		DryRun:    dry,
		CreatedAt: now,
		UpdatedAt: now,
	}
	s.jobs[j.ID] = j
	s.persist()
	return clone(j)
}

func (s *Store) ByHash(hash string) (*model.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byHash[hash]
	if !ok {
		return nil, false
	}
	return clone(s.jobs[id]), true
}

func (s *Store) ByLocator(locator string) (*model.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byLoc[locator]
	if !ok {
		return nil, false
	}
	return clone(s.jobs[id]), true
}

func (s *Store) Get(id string) (*model.Job, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil, false
	}
	return clone(j), true
}

func (s *Store) Save(j *model.Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	j.UpdatedAt = time.Now().UTC()
	cp := clone(j)
	s.jobs[cp.ID] = cp
	if cp.Status == model.StatusSucceeded && cp.DocHash != "" {
		s.byHash[cp.DocHash] = cp.ID
	}
	if cp.Status == model.StatusSucceeded && cp.Locator != "" {
		s.byLoc[cp.Locator] = cp.ID
	}
	return s.persist()
}

func (s *Store) persist() error {
	list := make([]*model.Job, 0, len(s.jobs))
	for _, j := range s.jobs {
		list = append(list, j)
	}
	b, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		return fmt.Errorf("rename job store: %w", err)
	}
	return nil
}

func clone(j *model.Job) *model.Job {
	if j == nil {
		return nil
	}
	b, _ := json.Marshal(j)
	var out model.Job
	_ = json.Unmarshal(b, &out)
	return &out
}
