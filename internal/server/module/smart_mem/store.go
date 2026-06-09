package smart_mem

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// ErrNotFound is returned when a requested UUID has no persisted document.
var ErrNotFound = errors.New("smart_mem: document not found")

var uuidShape = regexp.MustCompile(`^[a-fA-F0-9-]{8,64}$`)

// FileStore persists raw JSON documents to disk keyed by UUID.
type FileStore struct {
	baseDir string
}

// NewFileStore creates a FileStore rooted at baseDir, creating the
// directory if it doesn't exist.
func NewFileStore(baseDir string) (*FileStore, error) {
	if baseDir == "" {
		return nil, errors.New("smart_mem: baseDir required")
	}
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("smart_mem: mkdir %s: %w", baseDir, err)
	}
	return &FileStore{baseDir: baseDir}, nil
}

// Put writes raw bytes for the given UUID atomically (write+rename).
func (s *FileStore) Put(uuid string, raw []byte) error {
	path, err := s.pathFor(uuid)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return fmt.Errorf("smart_mem: write tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("smart_mem: rename: %w", err)
	}
	return nil
}

// Get returns the raw bytes for uuid, or ErrNotFound.
func (s *FileStore) Get(uuid string) ([]byte, error) {
	path, err := s.pathFor(uuid)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("smart_mem: read: %w", err)
	}
	return raw, nil
}

func (s *FileStore) pathFor(uuid string) (string, error) {
	if !uuidShape.MatchString(uuid) {
		return "", fmt.Errorf("smart_mem: invalid uuid %q", uuid)
	}
	return filepath.Join(s.baseDir, uuid+".json"), nil
}
