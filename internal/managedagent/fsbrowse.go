package managedagent

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Folder browsing is an allowlist. The only directories tingly-box will
// list are the ones the user has handed to it: the local sources (folders
// a task has been started in, or that were added on purpose). Everything
// else is opaque — a path outside the allowlist can still be *submitted*
// (typed into the picker, sent as local_path) and thereby joins the list,
// but it is never enumerated first. Nothing is inferred from the host
// (no home listing, no Claude Code project history).

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

// RecentFolder is a folder the user has handed to tingly-box: a local
// source. This is the browse allowlist, in the order it was added.
type RecentFolder struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	IsRepo bool   `json:"is_repo"`
}

// allowedFolders lists the local sources' paths, existing ones only.
func (s *Service) allowedFolders(ctx context.Context) ([]string, error) {
	sources, err := s.stores.Sources.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []string
	for i := range sources {
		if sources[i].Kind != SourceKindLocal {
			continue
		}
		clean := filepath.Clean(sources[i].URL)
		if seen[clean] {
			continue
		}
		if info, err := os.Stat(clean); err != nil || !info.IsDir() {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out, nil
}

// RecentFolders returns the allowlist as folders for the picker.
func (s *Service) RecentFolders(ctx context.Context, limit int) ([]RecentFolder, error) {
	roots, err := s.allowedFolders(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]RecentFolder, 0, len(roots))
	for _, r := range roots {
		out = append(out, RecentFolder{Path: r, Name: filepath.Base(r), IsRepo: isRepoDir(r)})
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// Browse lists directories within the allowlist; see Browse.
func (s *Service) Browse(ctx context.Context, path string) (*DirListing, error) {
	roots, err := s.allowedFolders(ctx)
	if err != nil {
		return nil, err
	}
	return Browse(path, roots)
}
