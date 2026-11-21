package search

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileIndex stores documents as JSON files and does substring matching.
type FileIndex struct {
	dir string
	mu  sync.Mutex
}

// NewFileIndex creates the index directory.
func NewFileIndex(dir string) (*FileIndex, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "data/search"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &FileIndex{dir: dir}, nil
}

func (f *FileIndex) Backend() string { return "file" }

func (f *FileIndex) Close() error { return nil }

func (f *FileIndex) path(id string) string {
	safe := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			return r
		default:
			return '_'
		}
	}, id)
	return filepath.Join(f.dir, safe+".json")
}

func (f *FileIndex) Upsert(_ context.Context, doc Document) error {
	if strings.TrimSpace(doc.ID) == "" {
		return nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	b, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(f.path(doc.ID), b, 0o644)
}

func (f *FileIndex) Query(_ context.Context, q string, limit int) ([]Hit, error) {
	q = strings.ToLower(strings.TrimSpace(q))
	if limit <= 0 {
		limit = 20
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	entries, err := os.ReadDir(f.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var hits []Hit
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := os.ReadFile(filepath.Join(f.dir, e.Name()))
		if err != nil {
			continue
		}
		var doc Document
		if err := json.Unmarshal(b, &doc); err != nil {
			continue
		}
		hay := strings.ToLower(doc.Title + " " + doc.Body)
		if q != "" && !strings.Contains(hay, q) {
			continue
		}
		snip := doc.Title
		if snip == "" {
			snip = doc.Body
		}
		if len(snip) > 120 {
			snip = snip[:120]
		}
		hits = append(hits, Hit{ID: doc.ID, Score: 1, Snippet: snip})
		if len(hits) >= limit {
			break
		}
	}
	return hits, nil
}
