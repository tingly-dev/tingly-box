package managedagent

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Folder browsing is an allowlist. The only directories tingly-box will
// list are the Folders the user has handed to it. Everything else is opaque
// — a path outside the allowlist can still be *submitted* (typed into the
// picker, sent as a session's path) and thereby joins the list, but it is
// never enumerated first. Nothing is inferred from the host (no home
// listing, no Claude Code project history).

// DirEntry is one sub-directory in a Browse listing.
type DirEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	IsRepo bool   `json:"is_repo"`
}

// DirListing is the result of Browse. Path is empty for the top level,
// which lists the allowed folders themselves.
type DirListing struct {
	Path    string     `json:"path"`
	Parent  string     `json:"parent,omitempty"`
	IsRepo  bool       `json:"is_repo"`
	Entries []DirEntry `json:"entries"`
}

// Browse lists the sub-directories of path, hidden ones excluded, provided
// path is one of the allowed roots or inside one. An empty path lists the
// roots. Directories only: this exists so a person can pick a folder from
// the web UI, not to read files.
func Browse(path string, roots []string) (*DirListing, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		out := &DirListing{Path: "", Entries: []DirEntry{}}
		for _, r := range roots {
			if info, err := os.Stat(r); err == nil && info.IsDir() {
				out.Entries = append(out.Entries, DirEntry{Name: filepath.Base(r), Path: r, IsRepo: isRepoDir(r)})
			}
		}
		return out, nil
	}
	if !filepath.IsAbs(path) {
		return nil, invalid("path must be absolute")
	}
	path = filepath.Clean(path)
	root, ok := allowedRoot(path, roots)
	if !ok {
		return nil, forbidden("%s is not inside a folder you have added; use it as typed to add it", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, invalid("%s: %v", path, err)
	}
	if !info.IsDir() {
		return nil, invalid("%s is not a directory", path)
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, invalid("%s: %v", path, err)
	}
	out := &DirListing{Path: path, IsRepo: isRepoDir(path)}
	if path != root {
		out.Parent = filepath.Dir(path)
	}
	for _, e := range entries {
		if !e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		full := filepath.Join(path, e.Name())
		out.Entries = append(out.Entries, DirEntry{Name: e.Name(), Path: full, IsRepo: isRepoDir(full)})
	}
	sort.Slice(out.Entries, func(i, j int) bool {
		return strings.ToLower(out.Entries[i].Name) < strings.ToLower(out.Entries[j].Name)
	})
	if out.Entries == nil {
		out.Entries = []DirEntry{}
	}
	return out, nil
}

// allowedRoot returns the root that contains path (or equals it).
func allowedRoot(path string, roots []string) (string, bool) {
	for _, r := range roots {
		r = filepath.Clean(r)
		if path == r || strings.HasPrefix(path, r+string(filepath.Separator)) {
			return r, true
		}
	}
	return "", false
}

func isRepoDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// allowedRoots is the browse allowlist: the folders the user has handed
// over, existing ones only.
func (s *Service) allowedRoots(ctx context.Context) ([]string, error) {
	folders, err := s.stores.Folders.ListFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(folders))
	for i := range folders {
		clean := filepath.Clean(folders[i].Path)
		if info, err := os.Stat(clean); err == nil && info.IsDir() {
			out = append(out, clean)
		}
	}
	return out, nil
}

// Browse lists directories within the allowlist; see the package-level
// Browse for the rules.
func (s *Service) Browse(ctx context.Context, path string) (*DirListing, error) {
	roots, err := s.allowedRoots(ctx)
	if err != nil {
		return nil, err
	}
	return Browse(path, roots)
}
