package localsessions

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalid   = errors.New("invalid session identifier or cursor")
	ErrChanged   = errors.New("session or listing changed; refresh to continue")
	ErrLimit     = errors.New("directory contains too many entries to preview")
	ErrMissing   = errors.New("session history is unavailable")
	errNoHistory = errors.New("no history directory")
)

type index struct {
	dir     string
	entries []Entry
	expires time.Time
}
type snapshot struct {
	rel     string
	info    fs.FileInfo
	expires time.Time
}
type token struct {
	Kind      string `json:"k"`
	Rel       string `json:"r,omitempty"`
	Key       string `json:"i,omitempty"`
	Offset    int64  `json:"o,omitempty"`
	Direction string `json:"d,omitempty"`
	Skip      bool   `json:"s,omitempty"`
}

type Store struct {
	root      string
	key       [32]byte
	mu        sync.Mutex
	indexes   map[string]index
	snapshots map[string]snapshot
}

// New resolves configuration only; it does no filesystem discovery at startup.
func New(root string) (*Store, error) {
	if root == "" {
		base := os.Getenv("CLAUDE_CONFIG_DIR")
		if base == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, err
			}
			base = filepath.Join(home, ".claude")
		}
		root = filepath.Join(base, "projects")
	}
	s := &Store{root: root, indexes: make(map[string]index), snapshots: make(map[string]snapshot)}
	if _, err := rand.Read(s.key[:]); err != nil {
		return nil, err
	}
	return s, nil
}
func randomID() string { return rand.Text() }
func (s *Store) encode(t token) string {
	b, _ := json.Marshal(t)
	part := base64.RawURLEncoding.EncodeToString(b)
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(part))
	return part + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func (s *Store) decode(raw, kind string) (token, error) {
	var t token
	if len(raw) > 4096 {
		return t, ErrInvalid
	}
	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return t, ErrInvalid
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return t, ErrInvalid
	}
	mac := hmac.New(sha256.New, s.key[:])
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return t, ErrInvalid
	}
	b, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(b, &t) != nil || t.Kind != kind {
		return token{}, ErrInvalid
	}
	if t.Rel != "" && (!filepath.IsLocal(t.Rel) || strings.Contains(t.Rel, "\\")) {
		return token{}, ErrInvalid
	}
	return t, nil
}

func (s *Store) openRoot() (*os.Root, error) {
	root, err := os.OpenRoot(s.root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errNoHistory
		}
		return nil, ErrMissing
	}
	return root, nil
}

// safeOpen rejects links and special files in addition to os.Root's escape
// protection. Metadata is checked on the opened descriptor to avoid a swap.
func safeOpen(root *os.Root, rel string, directory bool) (*os.File, error) {
	if !filepath.IsLocal(rel) {
		return nil, ErrInvalid
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	prefix := ""
	for _, part := range parts {
		prefix = filepath.Join(prefix, part)
		info, err := root.Lstat(prefix)
		if err != nil {
			return nil, ErrMissing
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return nil, ErrInvalid
		}
	}
	before, err := root.Lstat(rel)
	if err != nil {
		return nil, ErrMissing
	}
	if directory {
		if !before.IsDir() {
			return nil, ErrInvalid
		}
	} else if !before.Mode().IsRegular() {
		return nil, ErrInvalid
	}
	f, err := openReadOnly(root, rel)
	if err != nil {
		return nil, ErrMissing
	}
	after, err := f.Stat()
	if err != nil || !os.SameFile(before, after) || after.IsDir() != directory || (!directory && !after.Mode().IsRegular()) {
		f.Close()
		return nil, ErrChanged
	}
	return f, nil
}
func (s *Store) pruneLocked() {
	now := time.Now()
	for k, v := range s.indexes {
		if now.After(v.expires) {
			delete(s.indexes, k)
		}
	}
	for k, v := range s.snapshots {
		if now.After(v.expires) {
			delete(s.snapshots, k)
		}
	}
}
func oldestIndex(m map[string]index) string {
	var key string
	var at time.Time
	for k, v := range m {
		if key == "" || v.expires.Before(at) {
			key, at = k, v.expires
		}
	}
	return key
}
func oldestSnapshot(m map[string]snapshot) string {
	var key string
	var at time.Time
	for k, v := range m {
		if key == "" || v.expires.Before(at) {
			key, at = k, v.expires
		}
	}
	return key
}

// List snapshots directory metadata only. Content is read only for the 25
// displayed sessions, at most 8 KiB each. Indexes have an explicit count/TTL cap.
func (s *Store) List(ctx context.Context, projectID, cursor string) (Listing, error) {
	out := Listing{Entries: []Entry{}}
	dir := "."
	kind := "project"
	if projectID != "" {
		t, err := s.decode(projectID, "project")
		if err != nil {
			return out, err
		}
		dir = t.Rel
		kind = "session"
	}
	var idx index
	key := ""
	offset := int64(0)
	if cursor != "" {
		t, err := s.decode(cursor, "list")
		if err != nil || t.Rel != dir || t.Offset < 0 {
			return out, ErrInvalid
		}
		s.mu.Lock()
		s.pruneLocked()
		idx, _ = s.indexes[t.Key]
		s.mu.Unlock()
		if idx.dir != dir {
			return out, ErrChanged
		}
		key, offset = t.Key, t.Offset
	} else {
		root, err := s.openRoot()
		if err != nil {
			if projectID == "" && errors.Is(err, errNoHistory) {
				return out, nil
			}
			return out, ErrMissing
		}
		defer root.Close()
		f, err := safeOpen(root, dir, true)
		if err != nil {
			return out, err
		}
		defer f.Close()
		entries := make([]Entry, 0)
		seen := 0
		for {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			batch, err := f.ReadDir(128)
			if err != nil && err != io.EOF {
				return out, ErrMissing
			}
			seen += len(batch)
			if seen > MaxDirectoryEntries {
				return out, ErrLimit
			}
			for _, de := range batch {
				if de.Type()&os.ModeSymlink != 0 {
					continue
				}
				if kind == "project" && !de.IsDir() {
					continue
				}
				if kind == "session" && (de.IsDir() || !strings.HasSuffix(de.Name(), ".jsonl")) {
					continue
				}
				info, e := de.Info()
				if e != nil || (!info.IsDir() && !info.Mode().IsRegular()) {
					continue
				}
				rel := filepath.Join(dir, de.Name())
				entries = append(entries, Entry{ID: s.encode(token{Kind: kind, Rel: rel}), Name: de.Name(), Size: info.Size(), Modified: info.ModTime()})
			}
			if err == io.EOF {
				break
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].Modified.Equal(entries[j].Modified) {
				return entries[i].Name < entries[j].Name
			}
			return entries[i].Modified.After(entries[j].Modified)
		})
		idx = index{dir: dir, entries: entries, expires: time.Now().Add(5 * time.Minute)}
		key = randomID()
		s.mu.Lock()
		s.pruneLocked()
		if len(s.indexes) >= 4 {
			delete(s.indexes, oldestIndex(s.indexes))
		}
		s.indexes[key] = idx
		s.mu.Unlock()
	}
	if offset > int64(len(idx.entries)) {
		return out, ErrInvalid
	}
	end := min(int(offset)+PageSize, len(idx.entries))
	out.Entries = append(out.Entries, idx.entries[offset:end]...)
	out.Total = len(idx.entries)
	if end < len(idx.entries) {
		out.NextCursor = s.encode(token{Kind: "list", Rel: dir, Key: key, Offset: int64(end)})
	}
	if kind == "session" || kind == "project" {
		root, err := s.openRoot()
		if err != nil {
			return out, ErrMissing
		}
		defer root.Close()
		for i := range out.Entries {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			t, _ := s.decode(out.Entries[i].ID, kind)
			if kind == "project" {
				out.Entries[i].Title = readProjectTitle(root, t.Rel)
			} else {
				out.Entries[i].Title = readTitle(root, t.Rel)
			}
		}
	}
	return out, nil
}

func readTitle(root *os.Root, rel string) string {
	f, err := safeOpen(root, rel, false)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf := make([]byte, 8192)
	n, _ := f.Read(buf)
	lines := strings.Split(string(buf[:n]), "\n")
	for _, line := range lines {
		var event struct {
			Type    string `json:"type"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &event) != nil || event.Type != "user" {
			continue
		}
		var text string
		if json.Unmarshal(event.Message.Content, &text) == nil {
			return shorten(text, 160)
		}
		var blocks []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if json.Unmarshal(event.Message.Content, &blocks) == nil {
			for _, block := range blocks {
				if block.Type == "text" {
					return shorten(block.Text, 160)
				}
			}
		}
	}
	return ""
}

func shorten(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return strings.ToValidUTF8(text[:limit], "") + "…"
}

// Index paths are display metadata only. Never open fullPath/cwd from an index.
func readProjectTitle(root *os.Root, rel string) string {
	f, err := safeOpen(root, filepath.Join(rel, "sessions-index.json"), false)
	if err != nil {
		return ""
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, 8193))
	if err != nil || len(buf) > 8192 {
		return ""
	}
	var index struct {
		OriginalPath string `json:"originalPath"`
	}
	if json.Unmarshal(buf, &index) != nil {
		return ""
	}
	return shorten(index.OriginalPath, 256)
}
