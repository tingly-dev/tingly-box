package session

import (
	"os"
	"path/filepath"

	"github.com/google/uuid"
)

// ReadLocalSession reads a session only from the given working folder's history.
func (s *Store) ReadLocalSession(folder, id string) ([]byte, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, err
	}
	return os.ReadFile(filepath.Join(s.resolveProjectPath(folder), id+".jsonl"))
}
