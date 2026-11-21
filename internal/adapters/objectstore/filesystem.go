package objectstore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Filesystem stores objects under a local directory.
type Filesystem struct {
	root string
	cdn  CDNConfig
}

// NewFilesystem creates root if needed.
func NewFilesystem(root string, cdn CDNConfig) (*Filesystem, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		root = "data/objects"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	return &Filesystem{root: root, cdn: cdn}, nil
}

func (f *Filesystem) Backend() string { return "filesystem" }

func (f *Filesystem) pathFor(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" || strings.Contains(key, "..") {
		return "", fmt.Errorf("invalid object key %q", key)
	}
	clean := filepath.Clean("/" + key)
	rel := strings.TrimPrefix(clean, "/")
	return filepath.Join(f.root, rel), nil
}

func (f *Filesystem) publicURL(key string) string {
	if !f.cdn.Enabled || f.cdn.BaseURL == "" {
		return ""
	}
	return strings.TrimRight(f.cdn.BaseURL, "/") + "/" + strings.TrimLeft(key, "/")
}

func (f *Filesystem) Put(_ context.Context, key string, r io.Reader, contentType string) (Object, error) {
	path, err := f.pathFor(key)
	if err != nil {
		return Object{}, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Object{}, err
	}
	tmp := path + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return Object{}, err
	}
	n, copyErr := io.Copy(out, r)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmp)
		return Object{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return Object{}, closeErr
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return Object{}, err
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	return Object{Key: key, ContentType: contentType, Size: n, URL: f.publicURL(key)}, nil
}

func (f *Filesystem) Get(_ context.Context, key string) (io.ReadCloser, Object, error) {
	path, err := f.pathFor(key)
	if err != nil {
		return nil, Object{}, err
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, Object{}, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, Object{}, err
	}
	return file, Object{Key: key, Size: fi.Size(), URL: f.publicURL(key), ContentType: "application/octet-stream"}, nil
}

func (f *Filesystem) Delete(_ context.Context, key string) error {
	path, err := f.pathFor(key)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Open returns a filesystem store. Empty dir still works (default data/objects).
func Open(dir, cdnBase string) (Store, Status, error) {
	cdn := CDNConfig{
		BaseURL: strings.TrimSpace(cdnBase),
		Enabled: strings.TrimSpace(cdnBase) != "",
	}
	if !cdn.Enabled {
		cdn.Detail = "CDN_BASE_URL unset — local paths only"
	} else {
		cdn.Detail = "placeholder: rewrite object URLs with CDN base; edge cache not provisioned"
	}
	fs, err := NewFilesystem(dir, cdn)
	if err != nil {
		return nil, Status{Backend: "filesystem", Degraded: true, Detail: err.Error(), CDN: cdn}, err
	}
	configured := strings.TrimSpace(dir) != "" || cdn.Enabled
	return fs, Status{Backend: "filesystem", Configured: configured, CDN: cdn}, nil
}
