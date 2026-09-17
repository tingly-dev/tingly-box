// fsbrowse.go lists a directory's subdirectories so a web session can pick
// a folder to work in, the same way @cc's own IM directory browser does:
// no persisted allowlist, no restriction on which absolute path is
// browsable — host access to this API is already the gate.
package managedagent

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEntry is one browsable subdirectory.
type DirEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	IsRepo bool   `json:"is_repo"`
	Hidden bool   `json:"hidden"`
}

// ListDirs lists the subdirectories of path. An empty path defaults to the
// user's home directory. Only directories are listed — a folder is what
// this feature works in, never a single file.
func (s *Service) ListDirs(ctx context.Context, path string) (string, []DirEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", nil, invalid("no path given and home directory is unavailable: %v", err)
		}
		path = home
	}
	clean, err := cleanFolderPath(path)
	if err != nil {
		return "", nil, err
	}

	entries, err := os.ReadDir(clean)
	if err != nil {
		return "", nil, invalid("%s: %v", clean, err)
	}

	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		full := filepath.Join(clean, name)
		out = append(out, DirEntry{
			Name:   name,
			Path:   full,
			IsRepo: s.isRepo(ctx, full),
			Hidden: strings.HasPrefix(name, "."),
		})
	}
	sort.Slice(out, func(i, j int) bool { return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name) })
	return clean, out, nil
}
