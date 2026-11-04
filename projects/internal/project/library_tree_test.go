package project

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seedLibrary(t *testing.T) (Layout, string) {
	t.Helper()
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	for _, f := range []string{"notes.md", "outputs/run-1/report.md", "outputs/run-1/data/rows.csv", "outputs/run-2/a.txt", "uploads/z.png", ".tmp/t1/scratch", ".hidden"} {
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
	// One level only; folders first; dot entries (.tmp, .hidden) are hidden.
	assert.Equal(t, []string{"outputs", "uploads", "notes.md"}, names(root))
	assert.Equal(t, "dir", root[0].Kind)
	assert.Equal(t, "outputs", root[0].Path)
	assert.Equal(t, "file", root[2].Kind)
	assert.Equal(t, int64(1), root[2].Size)
	assert.Equal(t, "upload", root[2].Origin)

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

	for _, rel := range []string{"out-alias", "out-alias/run-2", "..", "../p", "outputs/../..", "/etc", "outputs/../../..", ".tmp", ".tmp/t1", ".hidden", "outputs/.x", "link", "notes.md", "nope"} {
		_, err := l.ListLibraryDir("p", rel)
		assert.Error(t, err, rel)
	}

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

func TestLibraryFileResolvesOnlyRegularFilesInsideTheLibrary(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "linkdir")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "secret-link")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "notes.md"), filepath.Join(lib, "alias.md")))
	require.NoError(t, os.Symlink(filepath.Join(lib, "outputs"), filepath.Join(lib, "out-alias")))

	for _, rel := range []string{"notes.md", "/notes.md", "outputs/run-1/report.md"} {
		p, err := l.LibraryFile("p", rel)
		require.NoError(t, err, rel)
		assert.FileExists(t, p)
	}

	// Escapes, hidden entries and any symlink (the file itself or an ancestor) are invalid.
	for _, rel := range []string{"..", "../p/notes.md", "outputs/../../p/notes.md", "outputs/../notes.md", "./notes.md", "outputs//run-2/a.txt",
		".hidden", ".tmp/t1/scratch", "outputs/.x", "linkdir/secret", "secret-link", "alias.md", "out-alias/run-2/a.txt"} {
		_, err := l.LibraryFile("p", rel)
		assert.ErrorIs(t, err, ErrInvalidLibraryPath, rel)
	}

	// Missing files and folders are simply not found.
	for _, rel := range []string{"nope", "outputs", "outputs/run-1", "/etc/passwd", ""} {
		_, err := l.LibraryFile("p", rel)
		require.Error(t, err, rel)
		assert.False(t, errors.Is(err, ErrInvalidLibraryPath) && rel == "nope", rel)
	}
	_, err := l.LibraryFile("p", "nope")
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = l.LibraryFile("p", "outputs")
	assert.ErrorIs(t, err, os.ErrNotExist)
}
