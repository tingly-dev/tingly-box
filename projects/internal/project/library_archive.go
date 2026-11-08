package project

import (
	"archive/zip"
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

// What one folder download may hold. Both are checked before anything is
// streamed; they are meant to be revisited.
const (
	MaxArchiveFiles = 200
	MaxArchiveBytes = 40 << 20
)

// ArchiveLimitError says a folder is too big to download as one zip. Kind is
// "files" or "bytes"; Limit is the limit that was passed. The count stops as
// soon as a limit is passed, so the folder's real size is not known.
type ArchiveLimitError struct {
	Kind  string
	Limit int64
}

func (e *ArchiveLimitError) Error() string {
	if e.Kind == "files" {
		return fmt.Sprintf("this folder has more than %d files (a zip download is limited to %d files and %d MB)", e.Limit, MaxArchiveFiles, MaxArchiveBytes>>20)
	}
	return fmt.Sprintf("this folder is larger than %d MB (a zip download is limited to %d files and %d MB)", e.Limit>>20, MaxArchiveFiles, MaxArchiveBytes>>20)
}

// ArchiveFile is one file in a LibraryArchive: Rel is its library-relative
// path, Name its name inside the zip (relative to the folder).
type ArchiveFile struct {
	Rel  string
	Name string
	Size int64
}

// LibraryArchive is a folder of the library, counted and ready to be written
// as a zip. It keeps the library's Root open; the caller closes it.
type LibraryArchive struct {
	root  *os.Root
	name  string
	files []ArchiveFile
}

// OpenLibraryArchive counts a library folder ("" is the root) for a zip. It
// is subject to the library's policy (see library_root.go): a hidden, escaping
// or symlinked rel is ErrInvalidLibraryPath, and inside the folder dot
// entries, symlinks and non-regular files are left out. More than
// MaxArchiveFiles files or MaxArchiveBytes bytes is an *ArchiveLimitError.
func (l Layout) OpenLibraryArchive(projectID, rel string) (*LibraryArchive, error) {
	rel, err := cleanLibraryRel(rel)
	if err != nil {
		return nil, err
	}
	root, err := l.openLibraryRoot(projectID, false)
	if err != nil {
		if rel == "" && errors.Is(err, fs.ErrNotExist) {
			return &LibraryArchive{name: "library.zip"}, nil
		}
		return nil, err
	}
	a := &LibraryArchive{root: root, name: "library.zip"}
	ok := false
	defer func() {
		if !ok {
			root.Close()
		}
	}()
	dir := "."
	if rel != "" {
		if err := noLibrarySymlinks(root, rel, false); err != nil {
			return nil, err
		}
		dir = filepath.FromSlash(rel)
		a.name = path.Base(rel) + ".zip"
	}
	if info, err := root.Lstat(dir); err != nil {
		return nil, err
	} else if !info.IsDir() {
		return nil, fmt.Errorf("%w: %q", ErrNotDirectory, rel)
	}
	var total int64
	if err := a.scan(dir, rel, "", &total); err != nil {
		return nil, err
	}
	ok = true
	return a, nil
}

// scan adds the files under dir. rel is dir's library-relative path and
// prefix is its path inside the zip, both "" at the top.
func (a *LibraryArchive) scan(dir, rel, prefix string, total *int64) error {
	d, err := a.root.Open(dir)
	if err != nil {
		return err
	}
	entries, err := d.ReadDir(-1)
	d.Close()
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		name := e.Name()
		// A backslash is a separator to some unzip tools, so such a name
		// could not be stored safely.
		if strings.HasPrefix(name, ".") || strings.ContainsAny(name, `\`+"\x00") {
			continue
		}
		// Info is an lstat relative to the open directory, so it neither
		// follows a link nor resolves the whole path again.
		fi, err := e.Info()
		if err != nil {
			continue
		}
		childRel, childName := name, name
		if rel != "" {
			childRel = rel + "/" + name
		}
		if prefix != "" {
			childName = prefix + "/" + name
		}
		switch {
		case fi.Mode()&fs.ModeSymlink != 0:
		case fi.IsDir():
			if err := a.scan(filepath.Join(dir, name), childRel, childName, total); err != nil {
				return err
			}
		case fi.Mode().IsRegular():
			if len(a.files) >= MaxArchiveFiles {
				return &ArchiveLimitError{Kind: "files", Limit: MaxArchiveFiles}
			}
			*total += fi.Size()
			if *total > MaxArchiveBytes {
				return &ArchiveLimitError{Kind: "bytes", Limit: MaxArchiveBytes}
			}
			a.files = append(a.files, ArchiveFile{Rel: childRel, Name: childName, Size: fi.Size()})
		}
	}
	return nil
}

// Filename is the name to download the zip as: the folder's name plus .zip,
// or library.zip for the root.
func (a *LibraryArchive) Filename() string { return a.name }

// Files lists what the zip will hold.
func (a *LibraryArchive) Files() []ArchiveFile { return a.files }

// Close releases the library Root.
func (a *LibraryArchive) Close() error {
	if a.root == nil {
		return nil
	}
	return a.root.Close()
}

// Stream writes the zip to w. Each file is opened through the Root and must
// still be the regular file that was counted, so a file swapped for a link
// since then is left out; and at most the counted size of each is read, so
// the zip stays within the limits if a file has grown.
func (a *LibraryArchive) Stream(w io.Writer) error {
	zw := zip.NewWriter(w)
	for _, f := range a.files {
		if err := a.writeFile(zw, f); err != nil {
			return err
		}
	}
	return zw.Close()
}

func (a *LibraryArchive) writeFile(zw *zip.Writer, f ArchiveFile) error {
	name := filepath.FromSlash(f.Rel)
	if info, err := a.root.Lstat(name); err != nil || !info.Mode().IsRegular() {
		return nil
	}
	src, err := a.root.Open(name)
	if err != nil {
		return nil
	}
	defer src.Close()
	info, err := src.Stat()
	if err != nil {
		return nil
	}
	if cur, err := a.root.Lstat(name); err != nil || !info.Mode().IsRegular() || !os.SameFile(info, cur) {
		return nil
	}
	hdr := &zip.FileHeader{Name: f.Name, Method: zip.Deflate, Modified: info.ModTime()}
	hdr.SetMode(0o644)
	out, err := zw.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, io.LimitReader(src, f.Size))
	return err
}
