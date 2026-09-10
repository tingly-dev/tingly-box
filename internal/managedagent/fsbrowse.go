package managedagent

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirEntry is one sub-directory in a Browse listing.
type DirEntry struct {
	Name   string `json:"name"`
	Path   string `json:"path"`
	IsRepo bool   `json:"is_repo"`
}

// DirListing is the result of Browse.
type DirListing struct {
	Path    string     `json:"path"`
	Parent  string     `json:"parent,omitempty"`
	IsRepo  bool       `json:"is_repo"`
	Entries []DirEntry `json:"entries"`
}

// Browse lists the sub-directories of path (the home directory when empty),
// hidden ones excluded. Directories only: this exists so a person can pick
// a folder on the host from the web UI, not to read files.
func Browse(path string) (*DirListing, error) {
	path = strings.TrimSpace(path)
	if path == "" || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		path = home
	}
	if !filepath.IsAbs(path) {
		return nil, invalid("path must be absolute")
	}
	path = filepath.Clean(path)
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
	if parent := filepath.Dir(path); parent != path {
		out.Parent = parent
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

func isRepoDir(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, ".git"))
	return err == nil
}

// RecentFolder is a directory the user has worked in before: a local source
// of this control plane, or a project Claude Code itself remembers.
type RecentFolder struct {
	Path   string `json:"path"`
	Name   string `json:"name"`
	IsRepo bool   `json:"is_repo"`
	// Source is "tasks" when a local source already exists for it, or
	// "claude_code" when it comes from Claude Code's own project history.
	Source string `json:"source"`
}

// RecentProjectsFunc supplies Claude Code's remembered project paths.
type RecentProjectsFunc func(ctx context.Context) ([]string, error)

// RecentFolders merges local sources with Claude Code's recent projects,
// keeping only directories that still exist, local sources first.
func (s *Service) RecentFolders(ctx context.Context, recent RecentProjectsFunc, limit int) ([]RecentFolder, error) {
	seen := map[string]bool{}
	var out []RecentFolder
	add := func(path, origin string) {
		clean := filepath.Clean(path)
		if seen[clean] {
			return
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			return
		}
		seen[clean] = true
		out = append(out, RecentFolder{Path: clean, Name: filepath.Base(clean), IsRepo: isRepoDir(clean), Source: origin})
	}
	sources, err := s.stores.Sources.ListSources(ctx)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		if sources[i].Kind == SourceKindLocal {
			add(sources[i].URL, "tasks")
		}
	}
	if recent != nil {
		paths, err := recent(ctx)
		if err == nil {
			for _, p := range paths {
				add(p, "claude_code")
			}
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	if out == nil {
		out = []RecentFolder{}
	}
	return out, nil
}
