package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// Layout is where project files live on disk:
//
//	<root>/<project>/memory/MEMORY.md, memory/*.md
//	<root>/<project>/library/...           uploads; threads write outputs/<thread>/
//	<root>/<project>/library/.tmp/<thread>/ a thread's TMPDIR (hidden from the Library tab)
//	<root>/<project>/threads/<thread>/     each new thread's working directory (older ones keep theirs)
//	<root>/<project>/threads/<thread>.settings.json
type Layout struct {
	Root string
}

// Dir is a project's directory.
func (l Layout) Dir(projectID string) string { return filepath.Join(l.Root, projectID) }

// MemoryDir holds a project's memory files.
func (l Layout) MemoryDir(projectID string) string { return filepath.Join(l.Dir(projectID), "memory") }

// LibraryDir holds uploaded files and thread outputs.
func (l Layout) LibraryDir(projectID string) string {
	return filepath.Join(l.Dir(projectID), "library")
}

// OutputDir is where a thread writes deliverables that are not code.
func (l Layout) OutputDir(projectID, slug string) string {
	return filepath.Join(l.LibraryDir(projectID), "outputs", slug)
}

// TmpDir is a thread's scratch directory, used as its TMPDIR. It sits inside
// the library, so the thread may write there and send_file may read from it,
// but under a dot directory so ListLibrary does not show scratch files. It is
// keyed by thread id (unlike OutputDir's title slug, which can repeat).
func (l Layout) TmpDir(projectID, threadID string) string {
	return filepath.Join(l.LibraryDir(projectID), ".tmp", threadID)
}

// ThreadDir is a project thread's working directory: its worktree, or a
// scratch directory when the project has no repository.
func (l Layout) ThreadDir(projectID, threadID string) string {
	return filepath.Join(l.Dir(projectID), "threads", threadID)
}

// RepoWorktreeDir is where a thread's worktree goes inside its repository:
// the directory Claude Code itself uses for worktrees, so tools and people
// find it there. The repository's .git/info/exclude hides it from status.
func RepoWorktreeDir(repoPath, slug, threadID string) string {
	return filepath.Join(repoPath, ".claude", "worktrees", slug+"-"+threadID[:6])
}

// SettingsFile is the Claude Code settings file a thread runs with.
func (l Layout) SettingsFile(projectID, threadID string) string {
	return filepath.Join(l.Dir(projectID), "threads", threadID+".settings.json")
}

// SessionSettingsFile is the Claude Code settings file a folder session runs with.
func (l Layout) SessionSettingsFile(threadID string) string {
	return filepath.Join(l.Root, "_sessions", "settings", threadID+".json")
}

// Ensure creates a project's directories.
func (l Layout) Ensure(projectID string) error {
	for _, d := range []string{l.MemoryDir(projectID), l.LibraryDir(projectID), filepath.Join(l.Dir(projectID), "threads")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	idx := filepath.Join(l.MemoryDir(projectID), MemoryIndex)
	if _, err := os.Stat(idx); errors.Is(err, fs.ErrNotExist) {
		return os.WriteFile(idx, []byte(memoryIndexHeader), 0o644)
	}
	return nil
}

// ---- Memory ----

// MemoryIndex is the file every thread and the coordinator read first.
const MemoryIndex = "MEMORY.md"

const memoryIndexHeader = "# Project memory\n\n"

// MemoryFile describes one memory file.
type MemoryFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
}

var memoryNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}\.md$`)

// MemoryFileName turns a topic into a safe memory file name.
func MemoryFileName(topic string) string {
	topic = strings.TrimSuffix(strings.TrimSpace(topic), ".md")
	var b strings.Builder
	for _, r := range strings.ToLower(topic) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_' || r == '.':
			if b.Len() > 0 && !strings.HasSuffix(b.String(), "-") {
				b.WriteRune('-')
			}
		}
	}
	name := strings.Trim(b.String(), "-")
	if name == "" {
		name = fmt.Sprintf("note-%d", time.Now().Unix())
	}
	if len(name) > 60 {
		name = name[:60]
	}
	return name + ".md"
}

func (l Layout) memoryPath(projectID, name string) (string, error) {
	if !memoryNameRe.MatchString(name) {
		return "", fmt.Errorf("invalid memory file name %q", name)
	}
	return filepath.Join(l.MemoryDir(projectID), name), nil
}

// ReadMemory returns a memory file's content.
func (l Layout) ReadMemory(projectID, name string) (string, error) {
	p, err := l.memoryPath(projectID, name)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(p)
	return string(b), err
}

// WriteMemory writes a memory file.
func (l Layout) WriteMemory(projectID, name, content string) error {
	p, err := l.memoryPath(projectID, name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(content), 0o644)
}

// DeleteMemory removes a memory file and its index line. MEMORY.md itself
// cannot be deleted.
func (l Layout) DeleteMemory(projectID, name string) error {
	if name == MemoryIndex {
		return errors.New("MEMORY.md is the index and cannot be deleted")
	}
	p, err := l.memoryPath(projectID, name)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	return l.removeIndexLine(projectID, name)
}

// ListMemory lists memory files, MEMORY.md first.
func (l Layout) ListMemory(projectID string) ([]MemoryFile, error) {
	entries, err := os.ReadDir(l.MemoryDir(projectID))
	if errors.Is(err, fs.ErrNotExist) {
		return []MemoryFile{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []MemoryFile{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, MemoryFile{Name: e.Name(), Size: info.Size(), UpdatedAt: info.ModTime()})
	}
	sort.Slice(out, func(i, j int) bool {
		if (out[i].Name == MemoryIndex) != (out[j].Name == MemoryIndex) {
			return out[i].Name == MemoryIndex
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// Remember writes a memory file for topic and indexes it in MEMORY.md with
// a one-line summary. It returns the file name.
func (l Layout) Remember(projectID, topic, content, summary string) (string, error) {
	name := MemoryFileName(topic)
	if err := l.WriteMemory(projectID, name, strings.TrimSpace(content)+"\n"); err != nil {
		return "", err
	}
	if err := l.removeIndexLine(projectID, name); err != nil {
		return "", err
	}
	idx, _ := l.ReadMemory(projectID, MemoryIndex)
	if idx == "" {
		idx = memoryIndexHeader
	}
	if !strings.HasSuffix(idx, "\n") {
		idx += "\n"
	}
	summary = strings.TrimSpace(strings.ReplaceAll(summary, "\n", " "))
	if summary == "" {
		summary = strings.TrimSpace(topic)
	}
	idx += fmt.Sprintf("- [%s](%s) — %s\n", strings.TrimSpace(topic), name, summary)
	return name, l.WriteMemory(projectID, MemoryIndex, idx)
}

func (l Layout) removeIndexLine(projectID, name string) error {
	idx, err := l.ReadMemory(projectID, MemoryIndex)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var keep []string
	for _, line := range strings.Split(idx, "\n") {
		if strings.Contains(line, "("+name+")") {
			continue
		}
		keep = append(keep, line)
	}
	return l.WriteMemory(projectID, MemoryIndex, strings.Join(keep, "\n"))
}

// ---- Library ----

// LibraryFile describes one file in the library.
type LibraryFile struct {
	// Path is relative to the library root, with forward slashes.
	Path      string    `json:"path"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	// Origin is "upload" or "output" (written by a thread).
	Origin string `json:"origin"`
}

// ListLibrary walks the library.
func (l Layout) ListLibrary(projectID string) ([]LibraryFile, error) {
	root := l.LibraryDir(projectID)
	out := []LibraryFile{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") && p != root {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		rel = filepath.ToSlash(rel)
		origin := "upload"
		if strings.HasPrefix(rel, "outputs/") {
			origin = "output"
		}
		out = append(out, LibraryFile{Path: rel, Size: info.Size(), UpdatedAt: info.ModTime(), Origin: origin})
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, err
}

// LibraryPath resolves a library-relative path, refusing anything outside
// the library.
func (l Layout) LibraryPath(projectID, rel string) (string, error) {
	root := l.LibraryDir(projectID)
	p := filepath.Join(root, filepath.FromSlash(rel))
	if r, err := filepath.Rel(root, p); err != nil || r == "." || strings.HasPrefix(r, "..") {
		return "", fmt.Errorf("invalid library path %q", rel)
	}
	return p, nil
}

var (
	// ErrInvalidLibraryPath is returned for a path outside the library or into a hidden directory.
	ErrInvalidLibraryPath = errors.New("invalid library path")
	// ErrNotDirectory is returned when a listing is asked of a file.
	ErrNotDirectory = errors.New("not a directory")
)

// LibraryEntry is one file or folder directly inside a library directory.
type LibraryEntry struct {
	Name string `json:"name"`
	// Path is relative to the library root, with forward slashes.
	Path string `json:"path"`
	// Kind is "dir" or "file".
	Kind      string    `json:"kind"`
	Size      int64     `json:"size"`
	UpdatedAt time.Time `json:"updated_at"`
	// Origin is "upload" or "output" (under outputs/, written by a thread).
	Origin string `json:"origin"`
}

// ListLibraryDir lists one level of the library: rel is a library-relative
// directory ("" is the root). Folders come first, then files, each by name.
// Hidden (dot) entries, such as the threads' .tmp, and symlinks are left out,
// and a hidden or escaping rel is refused, so the tree never leads outside the
// library or into scratch space. A library that does not exist yet is empty.
func (l Layout) ListLibraryDir(projectID, rel string) ([]LibraryEntry, error) {
	root := l.LibraryDir(projectID)
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	dir := root
	if rel != "" {
		for _, part := range strings.Split(rel, "/") {
			if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
				return nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
			}
		}
		p, err := l.LibraryPath(projectID, rel)
		if err != nil {
			return nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
		}
		dir = p
	}
	info, err := os.Lstat(dir)
	if err != nil {
		if rel == "" && errors.Is(err, fs.ErrNotExist) {
			return []LibraryEntry{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		if info.Mode()&fs.ModeSymlink != 0 {
			return nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
		}
		return nil, fmt.Errorf("%w: %q", ErrNotDirectory, rel)
	}
	// An ancestor symlink would also lead out of the library.
	if real, err := filepath.EvalSymlinks(dir); err != nil {
		return nil, err
	} else if realRoot, err := filepath.EvalSymlinks(root); err != nil {
		return nil, err
	} else if r, err := filepath.Rel(realRoot, real); err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	out := []LibraryEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") || e.Type()&fs.ModeSymlink != 0 {
			continue
		}
		fi, err := e.Info()
		if err != nil {
			continue
		}
		p := e.Name()
		if rel != "" {
			p = rel + "/" + p
		}
		entry := LibraryEntry{Name: e.Name(), Path: p, Kind: "file", UpdatedAt: fi.ModTime(), Origin: "upload"}
		if e.IsDir() {
			entry.Kind = "dir"
		} else if fi.Mode().IsRegular() {
			entry.Size = fi.Size()
		} else {
			continue
		}
		if p == "outputs" || strings.HasPrefix(p, "outputs/") {
			entry.Origin = "output"
		}
		out = append(out, entry)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if (out[i].Kind == "dir") != (out[j].Kind == "dir") {
			return out[i].Kind == "dir"
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}
