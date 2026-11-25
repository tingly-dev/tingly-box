package project

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedLibrary(t *testing.T) (Layout, string) {
	t.Helper()
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	for _, f := range []string{"notes.md", "outputs/run-1/report.md", "outputs/run-1/data/rows.csv", "outputs/run-2/a.txt", "uploads/z.png", ".tmp/t1/scratch", ".hidden", ".cache/go-mod/cache.bin"} {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, f), []byte("x"), 0o644))
	}
	return l, lib
}

func names(es []LibraryEntry) []string {
	out := []string{}
	for _, e := range es {
		out = append(out, e.Name)
	}
	return out
}

func TestListLibraryDirOneLevelFoldersFirst(t *testing.T) {
	l, _ := seedLibrary(t)

	root, err := l.ListLibraryDir("p", "")
	require.NoError(t, err)
	// One level only; folders first; dot entries are hidden except the
	// scratch subtree (.tmp), which is listed like any other folder.
	assert.Equal(t, []string{".tmp", "outputs", "uploads", "notes.md"}, names(root))
	assert.Equal(t, "dir", root[0].Kind)
	assert.Equal(t, ".tmp", root[0].Path)
	assert.Equal(t, "scratch", root[0].Origin)
	assert.Equal(t, "dir", root[1].Kind)
	assert.Equal(t, "outputs", root[1].Path)
	assert.Equal(t, "file", root[3].Kind)
	assert.Equal(t, int64(1), root[3].Size)
	assert.Equal(t, "upload", root[3].Origin)

	scratch, err := l.ListLibraryDir("p", ".tmp")
	require.NoError(t, err)
	assert.Equal(t, []string{"t1"}, names(scratch))
	assert.Equal(t, ".tmp/t1", scratch[0].Path)
	assert.Equal(t, "scratch", scratch[0].Origin)
	inside, err := l.ListLibraryDir("p", ".tmp/t1")
	require.NoError(t, err)
	assert.Equal(t, []string{"scratch"}, names(inside))
	assert.Equal(t, ".tmp/t1/scratch", inside[0].Path)
	assert.Equal(t, "scratch", inside[0].Origin)

	sub, err := l.ListLibraryDir("p", "outputs/run-1")
	require.NoError(t, err)
	assert.Equal(t, []string{"data", "report.md"}, names(sub))
	assert.Equal(t, "outputs/run-1/report.md", sub[1].Path)
	assert.Equal(t, "output", sub[1].Origin)
	assert.Equal(t, "output", sub[0].Origin)
}

func TestListLibraryDirRefusesAnythingOutsideTheLibrary(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "link")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "notes.md"), filepath.Join(lib, "alias.md")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "outputs"), filepath.Join(lib, "out-alias")))

	for _, rel := range []string{"out-alias", "out-alias/run-2", "..", "../p", "outputs/../..", "/etc", "outputs/../../..", ".hidden", "outputs/.x", "link", "notes.md", "nope"} {
		_, err := l.ListLibraryDir("p", rel)
		assert.Error(t, err, rel)
	}
	// The scratch subtree itself is reachable, deeper hidden names are not.
	_, err := l.ListLibraryDir("p", ".tmp")
	assert.NoError(t, err)
	_, err = l.ListLibraryDir("p", ".tmp/.h")
	assert.Error(t, err)

	// Symlinks are not listed, so the tree never leads outside the library.
	root, err := l.ListLibraryDir("p", "")
	require.NoError(t, err)
	assert.NotContains(t, names(root), "link")
	assert.NotContains(t, names(root), "alias.md")
}

func TestListLibraryDirMissingLibraryIsEmpty(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	got, err := l.ListLibraryDir("p", "")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestListLibraryDirErrorKinds(t *testing.T) {
	l, _ := seedLibrary(t)
	_, err := l.ListLibraryDir("p", "nope")
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = l.ListLibraryDir("p", "notes.md")
	assert.ErrorIs(t, err, ErrNotDirectory)
	_, err = l.ListLibraryDir("p", "..")
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)
}

// An upload is shown under the name it was given; only the id SaveAttachment
// puts in front is dropped, and only for files directly in uploads/.
func TestListLibraryDirShowsUploadsByTheirOriginalName(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	files := []string{
		"uploads/0123456789ab-report final.pdf",
		"uploads/0123456789ab-a-b.txt",
		// An original name that itself looks like a stored one: the extra id is
		// the only one removed, so the name the person gave it comes back.
		"uploads/0123456789ab-fedcba987654-looks-stored.txt",
		"uploads/notes.md",
		"outputs/0123456789ab-x.txt",
	}
	for _, f := range files {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, f)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, f), []byte("x"), 0o644))
	}

	uploads, err := l.ListLibraryDirOrdered("p", "uploads", LibraryOrder{By: LibraryOrderName})
	require.NoError(t, err)
	assert.Equal(t, []string{"a-b.txt", "fedcba987654-looks-stored.txt", "notes.md", "report final.pdf"}, names(uploads))
	// The real path is untouched: that is what downloads and deletes address.
	assert.Equal(t, "uploads/0123456789ab-a-b.txt", uploads[0].Path)

	outputs, err := l.ListLibraryDir("p", "outputs")
	require.NoError(t, err)
	assert.Equal(t, []string{"0123456789ab-x.txt"}, names(outputs), "only uploads/ is rewritten")
}

func TestListLibraryDirOrder(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	times := map[string]time.Time{
		"uploads/b.txt": time.Unix(300, 0),
		"uploads/a.txt": time.Unix(100, 0),
		"uploads/c.txt": time.Unix(200, 0),
	}
	for rel, at := range times {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(lib, rel)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(lib, rel), []byte("x"), 0o644))
		require.NoError(t, os.Chtimes(filepath.Join(lib, rel), at, at))
	}
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "uploads/z-dir"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "uploads/a-dir"), 0o755))

	order := func(o LibraryOrder) []string {
		es, err := l.ListLibraryDirOrdered("p", "uploads", o)
		require.NoError(t, err)
		return names(es)
	}

	// Default: folders by name, files newest first.
	assert.Equal(t, []string{"a-dir", "z-dir", "b.txt", "c.txt", "a.txt"}, order(LibraryNewestFirst))
	assert.Equal(t, []string{"a-dir", "z-dir", "b.txt", "c.txt", "a.txt"}, order(LibraryOrder{}), "empty order means newest first")
	assert.Equal(t, []string{"a-dir", "z-dir", "a.txt", "c.txt", "b.txt"}, order(LibraryOrder{By: LibraryOrderTime}))
	assert.Equal(t, []string{"a-dir", "z-dir", "a.txt", "b.txt", "c.txt"}, order(LibraryOrder{By: LibraryOrderName}))
	assert.Equal(t, []string{"z-dir", "a-dir", "c.txt", "b.txt", "a.txt"}, order(LibraryOrder{By: LibraryOrderName, Desc: true}))
}

// openRel opens a library file the way the HTTP layer does and returns its content.
func openRel(l Layout, rel string) (string, error) {
	f, _, err := l.OpenLibraryFile("p", rel)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(f)
	return string(b), err
}

func TestOpenLibraryFileServesOnlyRegularFilesInsideTheLibrary(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "linkdir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "secret-link")))
	require.NoError(t, os.Symlink("../../../../../../../../../../../../etc/passwd", filepath.Join(lib, "relative-escape")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "notes.md"), filepath.Join(lib, "alias.md")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "outputs"), filepath.Join(lib, "out-alias")))
	require.NoError(t, os.Symlink("notes.md", filepath.Join(lib, "rel-alias.md")))

	for _, rel := range []string{"notes.md", "/notes.md", "outputs/run-1/report.md", "outputs/run-2/a.txt"} {
		got, err := openRel(l, rel)
		require.NoError(t, err, rel)
		assert.Equal(t, "x", got, rel)
	}

	// Escapes, hidden entries and any symlink (the file itself or an ancestor,
	// absolute, relative, or pointing back into the library) are invalid. The
	// scratch subtree is the one hidden path that is served.
	got, err := openRel(l, ".tmp/t1/scratch")
	require.NoError(t, err)
	assert.Equal(t, "x", got)
	for _, rel := range []string{"..", "../p/notes.md", "outputs/../../p/notes.md", "outputs/../notes.md", "./notes.md", "outputs//run-2/a.txt",
		".hidden", ".tmp/.h", "outputs/.x", "linkdir/secret", "secret-link", "relative-escape", "alias.md", "rel-alias.md", "out-alias/run-2/a.txt",
		"outputs/run-1/../../../x", "\u0000", "notes.md\u0000", "no\u0000tes.md"} {
		_, _, err := l.OpenLibraryFile("p", rel)
		require.Error(t, err, rel)
	}
	for _, rel := range []string{"..", "outputs/../notes.md", ".tmp/.h", "linkdir/secret", "secret-link", "alias.md", "out-alias/run-2/a.txt"} {
		_, _, err := l.OpenLibraryFile("p", rel)
		assert.ErrorIs(t, err, ErrInvalidLibraryPath, rel)
	}

	// Missing files and folders are simply not found; case variants are other names.
	for _, rel := range []string{"nope", "outputs", "outputs/run-1", "/etc/passwd", "C:\\Windows\\win.ini", "\\\\host\\share\\f", "a\\..\\notes.md", "%2e%2e/notes.md", "%2e%2e%2fnotes.md"} {
		_, _, err := l.OpenLibraryFile("p", rel)
		assert.ErrorIs(t, err, os.ErrNotExist, rel)
	}
	if _, err := os.Stat(filepath.Join(lib, "NOTES.MD")); err != nil { // a case-sensitive file system
		for _, rel := range []string{"Notes.md", "OUTPUTS/run-2/a.txt", ".TMP/t1/scratch"} {
			_, _, err := l.OpenLibraryFile("p", rel)
			assert.Error(t, err, rel)
		}
	}
	_, _, err = l.OpenLibraryFile("p", "")
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)
	// Backslashes are ordinary name characters here (Windows turns them into separators, so ".." stays refused there too).
	for _, rel := range []string{"..\\notes.md", "..\\..\\etc\\passwd"} {
		_, _, err := l.OpenLibraryFile("p", rel)
		assert.Error(t, err, rel)
	}

	// A project without a library has nothing to open.
	_, _, err = Layout{Root: t.TempDir()}.OpenLibraryFile("p", "notes.md")
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// A writer of the library (a thread) can swap a file for a link while a request is
// being served. Whatever the timing, the content is never from outside the library.
func TestOpenLibraryFileSurvivesFilesBeingSwappedForLinks(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(outside, "dir"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "dir", "report.md"), []byte("SECRET"), 0o644))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	defer wg.Wait()
	defer close(stop)
	wg.Add(1)
	go func() {
		defer wg.Done()
		file, dir := filepath.Join(lib, "swap.md"), filepath.Join(lib, "swapdir")
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			_ = os.Remove(file)
			_ = os.RemoveAll(dir)
			if i%2 == 0 {
				_ = os.WriteFile(file, []byte("x"), 0o644)
				_ = os.MkdirAll(dir, 0o755)
				_ = os.WriteFile(filepath.Join(dir, "report.md"), []byte("x"), 0o644)
			} else {
				_ = os.Symlink(filepath.Join(outside, "secret"), file)
				_ = os.Symlink(filepath.Join(outside, "dir"), dir)
			}
		}
	}()
	for i := 0; i < 3000; i++ {
		for _, rel := range []string{"swap.md", "swapdir/report.md"} {
			// The file may be caught empty while the swapper is still writing it.
			if got, err := openRel(l, rel); err == nil {
				require.NotContains(t, got, "SECRET", rel)
			}
		}
	}
}

func TestRemoveLibraryFileAppliesTheSameChecks(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "linkdir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "secret-link")))

	for _, rel := range []string{"..", ".hidden", "linkdir/secret", "secret-link", "outputs/../../x"} {
		assert.ErrorIs(t, l.RemoveLibraryFile("p", rel), ErrInvalidLibraryPath, rel)
	}
	for _, rel := range []string{"outputs", "nope"} {
		assert.ErrorIs(t, l.RemoveLibraryFile("p", rel), os.ErrNotExist, rel)
	}
	// Scratch is deletable, like anything else that is listed.
	assert.NoError(t, l.RemoveLibraryFile("p", ".tmp/t1/scratch"))
	assert.NoFileExists(t, filepath.Join(lib, ".tmp/t1/scratch"))
	assert.FileExists(t, filepath.Join(outside, "secret"))
	assert.DirExists(t, filepath.Join(lib, "outputs"))

	require.NoError(t, l.RemoveLibraryFile("p", "notes.md"))
	assert.NoFileExists(t, filepath.Join(lib, "notes.md"))
}

func TestWriteLibraryFileNeverWritesThroughALinkOrOutside(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	outside := t.TempDir()
	require.NoError(t, os.MkdirAll(lib, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(outside, "target"), []byte("orig"), 0o644))
	// A thread leaves links in its library.
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "uploads")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "target"), filepath.Join(lib, "report.md")))
	require.NoError(t, os.WriteFile(filepath.Join(lib, "real.md"), []byte("old"), 0o644))
	require.NoError(t, os.Symlink(filepath.Join(lib, "real.md"), filepath.Join(lib, "inner.md")))

	_, err := l.WriteLibraryFile("p", "uploads/a-x.txt", strings.NewReader("pwn"), true)
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)
	_, err = l.WriteLibraryFile("p", "report.md", strings.NewReader("pwn"), false)
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)
	_, err = l.WriteLibraryFile("p", "inner.md", strings.NewReader("pwn"), false)
	assert.ErrorIs(t, err, ErrInvalidLibraryPath)
	for _, rel := range []string{"../x", ".env", ".tmp/x", ".tmp/t1/scratch", "a/../../x", ""} {
		_, err = l.WriteLibraryFile("p", rel, strings.NewReader("pwn"), false)
		assert.ErrorIs(t, err, ErrInvalidLibraryPath, rel)
	}

	entries, _ := os.ReadDir(outside)
	assert.Len(t, entries, 1, "nothing was created outside the library")
	b, _ := os.ReadFile(filepath.Join(outside, "target"))
	assert.Equal(t, "orig", string(b))
	b, _ = os.ReadFile(filepath.Join(lib, "real.md"))
	assert.Equal(t, "old", string(b))

	// Normal writes: new folders, overwrite, and exclusive creation.
	n, err := l.WriteLibraryFile("p", "docs/a/b.md", strings.NewReader("hello"), false)
	require.NoError(t, err)
	assert.Equal(t, int64(5), n)
	_, err = l.WriteLibraryFile("p", "docs/a/b.md", strings.NewReader("again"), false)
	require.NoError(t, err)
	b, _ = os.ReadFile(filepath.Join(lib, "docs/a/b.md"))
	assert.Equal(t, "again", string(b))
	_, err = l.WriteLibraryFile("p", "docs/a/b.md", strings.NewReader("x"), true)
	assert.ErrorIs(t, err, os.ErrExist)
}
