// Package imagestore is the on-disk archive of generated and edited images
// under <configDir>/image, and the focus workbenches built on top of it.
//
// The directory is the source of truth: an image is a PNG plus a
// human-readable .txt sidecar, grouped by day (image/YYYYMMDD/<id>.png). The
// gateway writes into it on every successful generation/edit (best-effort,
// see protocolserver.persistImages); the control-plane API reads it back.
// Nothing here is cached — listing scans the directory, so a file a user
// drops in or deletes by hand is seen on the next read.
//
// See .design/image-workbench.md.
package imagestore

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ErrNotFound is returned when an image or workbench id names nothing on disk.
var ErrNotFound = errors.New("not found")

// ErrInvalidID is returned for an id that is not one this store could have
// produced. Checked before any path is built, so an id can never walk out of
// the image directory.
var ErrInvalidID = errors.New("invalid id")

// imageIDPattern matches ids written by this store (YYYYMMDD-HHMMSS-<hex>)
// and by its predecessor (YYYYMMDD-HHMMSS, with an optional -N for the Nth
// image of one response), so images saved before ids existed list too.
var imageIDPattern = regexp.MustCompile(`^(\d{8})-\d{6}(-[0-9a-z]{1,16}){0,2}$`)

var dateDirPattern = regexp.MustCompile(`^\d{8}$`)

const (
	imageExt = ".png"
	metaExt  = ".txt"
)

// Meta is what is known about how an image came to be. All fields optional.
type Meta struct {
	Prompt    string
	Operation string // "", "edit" or "import"
	Model     string
	Size      string
	Quality   string
	Format    string
	Style     string
}

// Image is one archived image.
type Image struct {
	ID        string
	CreatedAt time.Time
	Bytes     int64
	Meta      Meta
}

// Store reads and writes the archive rooted at one image directory.
type Store struct {
	root string
	now  func() time.Time
}

// New returns a store rooted at dir (normally constant.GetImageDir(configDir)).
func New(dir string) *Store {
	return &Store{root: dir, now: time.Now}
}

// Root is the directory this store reads and writes.
func (s *Store) Root() string { return s.root }

func newImageID(now time.Time) string {
	var b [3]byte
	_, _ = rand.Read(b[:])
	return now.Format("20060102-150405") + "-" + hex.EncodeToString(b[:])
}

// Save writes one image and its sidecar and returns the new id. The id carries
// a random suffix, so two responses landing in the same second never
// overwrite each other.
func (s *Store) Save(data []byte, meta Meta) (string, error) {
	now := s.now()
	dir := filepath.Join(s.root, now.Format("20060102"))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("create image directory: %w", err)
	}
	id := newImageID(now)
	if err := os.WriteFile(filepath.Join(dir, id+imageExt), data, 0600); err != nil {
		return "", fmt.Errorf("write image: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+metaExt), []byte(FormatMeta(meta, now)), 0600); err != nil {
		// The image is the thing worth keeping; a missing sidecar only costs
		// the prompt in the listing.
		return id, fmt.Errorf("write image metadata: %w", err)
	}
	return id, nil
}

// Path returns the PNG path for id, or ErrInvalidID / ErrNotFound.
func (s *Store) Path(id string) (string, error) {
	m := imageIDPattern.FindStringSubmatch(id)
	if m == nil {
		return "", ErrInvalidID
	}
	p := filepath.Join(s.root, m[1], id+imageExt)
	if _, err := os.Stat(p); err != nil {
		if os.IsNotExist(err) {
			return "", ErrNotFound
		}
		return "", err
	}
	return p, nil
}

// Exists reports whether id names an archived image.
func (s *Store) Exists(id string) bool {
	_, err := s.Path(id)
	return err == nil
}

// Get returns one image's entry.
func (s *Store) Get(id string) (Image, error) {
	p, err := s.Path(id)
	if err != nil {
		return Image{}, err
	}
	return s.entry(filepath.Dir(p), id)
}

func (s *Store) entry(dir, id string) (Image, error) {
	info, err := os.Stat(filepath.Join(dir, id+imageExt))
	if err != nil {
		return Image{}, err
	}
	img := Image{ID: id, Bytes: info.Size(), CreatedAt: info.ModTime()}
	if t, err := time.ParseInLocation("20060102-150405", id[:15], time.Local); err == nil {
		img.CreatedAt = t
	}
	if raw, err := os.ReadFile(filepath.Join(dir, id+metaExt)); err == nil {
		img.Meta = ParseMeta(string(raw))
	}
	return img, nil
}

// List returns up to limit images, newest first, strictly older than before
// (an id; empty means start from the newest). Unrecognized files are skipped.
func (s *Store) List(limit int, before string) ([]Image, error) {
	if limit <= 0 {
		limit = 50
	}
	days, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var dayNames []string
	for _, d := range days {
		if d.IsDir() && dateDirPattern.MatchString(d.Name()) {
			dayNames = append(dayNames, d.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dayNames)))

	var out []Image
	for _, day := range dayNames {
		if before != "" && day > before[:min(8, len(before))] {
			continue
		}
		dir := filepath.Join(s.root, day)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		var ids []string
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, imageExt) {
				continue
			}
			id := strings.TrimSuffix(name, imageExt)
			if imageIDPattern.MatchString(id) && id[:8] == day {
				ids = append(ids, id)
			}
		}
		sort.Sort(sort.Reverse(sort.StringSlice(ids)))
		for _, id := range ids {
			if before != "" && id >= before {
				continue
			}
			img, err := s.entry(dir, id)
			if err != nil {
				continue
			}
			out = append(out, img)
			if len(out) == limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// Delete removes an image and its sidecar.
func (s *Store) Delete(id string) error {
	p, err := s.Path(id)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return err
	}
	_ = os.Remove(strings.TrimSuffix(p, imageExt) + metaExt)
	return nil
}

// FormatMeta renders the .txt sidecar. The format predates this package and
// is kept as-is: it is meant to be read by a person browsing the folder.
func FormatMeta(info Meta, at time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Prompt: %s\n\n", info.Prompt)
	if info.Operation != "" {
		fmt.Fprintf(&b, "Operation: %s\n", info.Operation)
	}
	fmt.Fprintf(&b, "Model: %s\nSize: %s\nQuality: %s\n", info.Model, info.Size, info.Quality)
	if info.Format != "" {
		fmt.Fprintf(&b, "Format: %s\n", info.Format)
	}
	fmt.Fprintf(&b, "Timestamp: %s\n", at.Format(time.RFC3339))
	if info.Style != "" {
		fmt.Fprintf(&b, "Style: %s\n", info.Style)
	}
	return b.String()
}

// ParseMeta reads a sidecar written by FormatMeta. The prompt is free text
// and may itself contain blank lines, so it runs up to the last blank line
// that is followed by a known key, not the first.
func ParseMeta(raw string) Meta {
	var m Meta
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	body := raw
	if strings.HasPrefix(raw, "Prompt: ") {
		rest := strings.TrimPrefix(raw, "Prompt: ")
		cut := -1
		for _, key := range []string{"\n\nOperation: ", "\n\nModel: "} {
			if i := strings.LastIndex(rest, key); i > cut {
				cut = i
			}
		}
		if cut >= 0 {
			m.Prompt = rest[:cut]
			body = rest[cut+2:]
		} else {
			m.Prompt = strings.TrimRight(rest, "\n")
			body = ""
		}
	}
	for _, line := range strings.Split(body, "\n") {
		key, value, ok := strings.Cut(line, ": ")
		if !ok {
			continue
		}
		switch key {
		case "Operation":
			m.Operation = value
		case "Model":
			m.Model = value
		case "Size":
			m.Size = value
		case "Quality":
			m.Quality = value
		case "Format":
			m.Format = value
		case "Style":
			m.Style = value
		}
	}
	return m
}
