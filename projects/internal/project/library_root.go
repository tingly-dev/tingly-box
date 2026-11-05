package project

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Everything that serves, lists, deletes or writes library files for a
// request goes through an os.Root on the library directory. A Root refuses
// any name that reaches outside the directory, by ".." or by a symlink, and it
// does so while resolving the name, not before, so a file swapped for a link
// between a check and the open cannot lead out. On top of that the library has
// its own policy, checked here: no hidden (dot) segments, no symlinks at all,
// and only regular files. A link that stays inside the library is also
// refused, so a thread cannot alias a hidden folder into view; if one is
// swapped in during a request the Root still keeps the access inside the
// library.

// cleanLibraryRel trims the outer slashes of a library-relative path and
// refuses what is not a plain relative path inside the library: empty, "." and
// ".." segments, hidden (dot) segments such as .tmp, NUL bytes, and whatever
// filepath.IsLocal rejects (reserved names and drive paths on Windows). A
// leading "/" is dropped, so "/etc" names the library's own etc.
func cleanLibraryRel(rel string) (string, error) {
	rel = strings.Trim(filepath.ToSlash(rel), "/")
	if rel == "" {
		return "", nil
	}
	if strings.ContainsRune(rel, 0) || !filepath.IsLocal(filepath.FromSlash(rel)) {
		return "", fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	for _, part := range strings.Split(rel, "/") {
		if part == "" || part == "." || part == ".." || strings.HasPrefix(part, ".") {
			return "", fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
		}
	}
	return rel, nil
}

// openLibraryRoot opens the project's library directory as a Root. With
// create it is made first; without it a missing library is fs.ErrNotExist.
func (l Layout) openLibraryRoot(projectID string, create bool) (*os.Root, error) {
	dir := l.LibraryDir(projectID)
	if create {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	return os.OpenRoot(dir)
}

// noLibrarySymlinks refuses a library-relative path when any component of it,
// the last included, is a symlink. A component that does not exist yields the
// Lstat error (fs.ErrNotExist), or ends the check when missingOK.
func noLibrarySymlinks(root *os.Root, rel string, missingOK bool) error {
	p := ""
	for _, part := range strings.Split(rel, "/") {
		p = filepath.Join(p, part)
		info, err := root.Lstat(p)
		if err != nil {
			if missingOK && errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if info.Mode()&fs.ModeSymlink != 0 {
			return fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
		}
	}
	return nil
}

// OpenLibraryFile opens a library-relative path for reading: an existing
// regular file, reached without any symlink. An invalid path wraps
// ErrInvalidLibraryPath; a missing file, or a folder, wraps fs.ErrNotExist.
// The caller closes the file.
func (l Layout) OpenLibraryFile(projectID, rel string) (*os.File, fs.FileInfo, error) {
	rel, err := cleanLibraryRel(rel)
	if err != nil {
		return nil, nil, err
	}
	if rel == "" {
		return nil, nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	root, err := l.openLibraryRoot(projectID, false)
	if err != nil {
		return nil, nil, err
	}
	defer root.Close()
	if err := noLibrarySymlinks(root, rel, false); err != nil {
		return nil, nil, err
	}
	name := filepath.FromSlash(rel)
	if info, err := root.Lstat(name); err != nil {
		return nil, nil, err
	} else if !info.Mode().IsRegular() {
		// Checked before opening: opening a pipe would block.
		return nil, nil, fmt.Errorf("%q is not a file: %w", rel, fs.ErrNotExist)
	}
	f, err := root.Open(name)
	if err != nil {
		return nil, nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, nil, err
	}
	// What was opened must still be what the name says now, not a link swapped in since.
	if cur, err := root.Lstat(name); err != nil || !info.Mode().IsRegular() || !os.SameFile(info, cur) {
		f.Close()
		return nil, nil, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	return f, info, nil
}

// RemoveLibraryFile deletes one library file, under the same checks as
// OpenLibraryFile. Folders are not deleted.
func (l Layout) RemoveLibraryFile(projectID, rel string) error {
	rel, err := cleanLibraryRel(rel)
	if err != nil {
		return err
	}
	if rel == "" {
		return fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	root, err := l.openLibraryRoot(projectID, false)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := noLibrarySymlinks(root, rel, false); err != nil {
		return err
	}
	name := filepath.FromSlash(rel)
	if info, err := root.Lstat(name); err != nil {
		return err
	} else if !info.Mode().IsRegular() {
		return fmt.Errorf("%q is not a file: %w", rel, fs.ErrNotExist)
	}
	return root.Remove(name)
}

// WriteLibraryFile copies src into the library at rel, creating its folders,
// and returns the bytes written. It refuses a hidden name and any path that
// goes through a symlink, so a link a thread left in the library cannot send
// an upload elsewhere. With exclusive an existing file is an error
// (fs.ErrExist); otherwise it is replaced.
func (l Layout) WriteLibraryFile(projectID, rel string, src io.Reader, exclusive bool) (int64, error) {
	rel, err := cleanLibraryRel(rel)
	if err != nil {
		return 0, err
	}
	if rel == "" {
		return 0, fmt.Errorf("%w %q", ErrInvalidLibraryPath, rel)
	}
	root, err := l.openLibraryRoot(projectID, true)
	if err != nil {
		return 0, err
	}
	defer root.Close()
	if err := noLibrarySymlinks(root, rel, true); err != nil {
		return 0, err
	}
	name := filepath.FromSlash(rel)
	if dir := path.Dir(rel); dir != "." {
		if err := root.MkdirAll(filepath.FromSlash(dir), 0o755); err != nil {
			return 0, err
		}
	}
	flags := os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	if exclusive {
		flags = os.O_WRONLY | os.O_CREATE | os.O_EXCL
	}
	out, err := root.OpenFile(name, flags, 0o644)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(out, src)
	if cerr := out.Close(); err == nil {
		err = cerr
	}
	return n, err
}

// ListLibraryDir lists one level of the library: rel is a library-relative
// directory ("" is the root). Folders come first, then files, each by name.
// Hidden (dot) entries, such as the threads' .tmp, and symlinks are left out,
// and a hidden, escaping or symlinked rel is refused, so the tree never leads
// outside the library or into scratch space. A library that does not exist yet
// is empty.
func (l Layout) ListLibraryDir(projectID, rel string) ([]LibraryEntry, error) {
	rel, err := cleanLibraryRel(rel)
	if err != nil {
		return nil, err
	}
	root, err := l.openLibraryRoot(projectID, false)
	if err != nil {
		if rel == "" && errors.Is(err, fs.ErrNotExist) {
			return []LibraryEntry{}, nil
		}
		return nil, err
	}
	defer root.Close()
	name := "."
	if rel != "" {
		if err := noLibrarySymlinks(root, rel, false); err != nil {
			return nil, err
		}
		name = filepath.FromSlash(rel)
	}
	if info, err := root.Lstat(name); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%w: %q", ErrNotDirectory, rel)
	}
	dir, err := root.Open(name)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	out := []LibraryEntry{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".") {
			continue
		}
		fi, err := root.Lstat(filepath.Join(name, e.Name()))
		if err != nil || fi.Mode()&fs.ModeSymlink != 0 {
			continue
		}
		p := e.Name()
		if rel != "" {
			p = rel + "/" + p
		}
		entry := LibraryEntry{Name: e.Name(), Path: p, Kind: "file", UpdatedAt: fi.ModTime(), Origin: "upload"}
		if fi.IsDir() {
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
