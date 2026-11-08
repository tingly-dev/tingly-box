package project

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeLib(t *testing.T, lib string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(lib, filepath.FromSlash(name))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, []byte(body), 0o644))
	}
}

// archiveOf opens, writes and reads back a folder's archive: entry name to body.
func archiveOf(t *testing.T, l Layout, rel string) map[string]string {
	t.Helper()
	a, err := l.OpenLibraryArchive("p", rel)
	require.NoError(t, err)
	defer a.Close()
	var buf bytes.Buffer
	require.NoError(t, a.Stream(&buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	out := map[string]string{}
	for _, f := range zr.File {
		rc, err := f.Open()
		require.NoError(t, err)
		b, err := io.ReadAll(rc)
		require.NoError(t, err)
		rc.Close()
		out[f.Name] = string(b)
	}
	return out
}

func TestLibraryArchiveRoundTripsAFolderWithRelativeNames(t *testing.T) {
	l, lib := seedLibrary(t)
	writeLib(t, lib, map[string]string{
		"outputs/run-1/empty.txt":    "",
		"outputs/run-1/报告 final.txt": "中文",
	})
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "outputs/run-1/emptydir"), 0o755))

	got := archiveOf(t, l, "outputs/run-1")
	assert.Equal(t, map[string]string{
		"report.md":     "x",
		"data/rows.csv": "x",
		"empty.txt":     "",
		"报告 final.txt":  "中文",
	}, got)

	// A trailing or leading slash names the same folder.
	assert.Equal(t, got, archiveOf(t, l, "/outputs/run-1/"))

	for name := range got {
		assert.True(t, fs.ValidPath(name), name)
		assert.False(t, strings.HasPrefix(name, "/"), name)
		assert.NotContains(t, name, "..")
		assert.NotContains(t, name, `\`)
	}
}

func TestLibraryArchiveFilenameIsTheFolderName(t *testing.T) {
	l, _ := seedLibrary(t)
	a, err := l.OpenLibraryArchive("p", "outputs/run-1")
	require.NoError(t, err)
	defer a.Close()
	assert.Equal(t, "run-1.zip", a.Filename())

	root, err := l.OpenLibraryArchive("p", "")
	require.NoError(t, err)
	defer root.Close()
	assert.Equal(t, "library.zip", root.Filename())
}

func TestLibraryArchiveOfTheRootWorksWithinTheLimits(t *testing.T) {
	l, _ := seedLibrary(t)
	got := archiveOf(t, l, "")
	keys := []string{}
	for k := range got {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	// Dot entries (.tmp, .hidden) are not in it.
	assert.Equal(t, []string{"notes.md", "outputs/run-1/data/rows.csv", "outputs/run-1/report.md", "outputs/run-2/a.txt", "uploads/z.png"}, keys)
}

func TestLibraryArchiveSkipsDotEntriesSymlinksAndSpecialFiles(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o644))
	writeLib(t, lib, map[string]string{"d/keep.txt": "k", "d/.dot": "no", "d/.hid/inner": "no", "d/sub/.also": "no"})
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "d/leak.txt")))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "d/linkdir")))
	// A link that stays inside the library is skipped too.
	require.NoError(t, os.Symlink(filepath.Join(lib, "notes.md"), filepath.Join(lib, "d/alias.md")))
	require.NoError(t, os.Symlink(filepath.Join(lib, ".tmp"), filepath.Join(lib, "d/tmp-alias")))

	got := archiveOf(t, l, "d")
	assert.Equal(t, map[string]string{"keep.txt": "k"}, got)
	for _, body := range got {
		assert.NotContains(t, body, "SECRET")
	}
}

func TestLibraryArchiveSkipsNamesThatAreNotSafeZipPaths(t *testing.T) {
	l, lib := seedLibrary(t)
	writeLib(t, lib, map[string]string{"d/ok.txt": "ok", `d/a\b.txt`: "backslash"})
	assert.Equal(t, map[string]string{"ok.txt": "ok"}, archiveOf(t, l, "d"))
}

func TestLibraryArchiveRefusesPathsOutsideTheLibrary(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("s"), 0o644))
	require.NoError(t, os.Symlink(outside, filepath.Join(lib, "linkdir")))
	require.NoError(t, os.MkdirAll(filepath.Join(lib, "real"), 0o755))
	require.NoError(t, os.Symlink(filepath.Join(lib, "real"), filepath.Join(lib, "reallink")))

	for _, bad := range []string{"..", "../x", "outputs/../..", "outputs/../../x", ".tmp", ".tmp/t1", "outputs/.x", "linkdir", "linkdir/sub", "reallink", "a\x00b"} {
		_, err := l.OpenLibraryArchive("p", bad)
		assert.ErrorIs(t, err, ErrInvalidLibraryPath, bad)
	}
	// A leading slash is not absolute: it names the library's own etc.
	_, err := l.OpenLibraryArchive("p", "/etc")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = l.OpenLibraryArchive("p", "nope")
	assert.ErrorIs(t, err, fs.ErrNotExist)
	_, err = l.OpenLibraryArchive("p", "notes.md")
	assert.ErrorIs(t, err, ErrNotDirectory)
}

func TestLibraryArchiveOfAMissingLibraryIsEmptyForTheRootOnly(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	a, err := l.OpenLibraryArchive("p", "")
	require.NoError(t, err)
	defer a.Close()
	var buf bytes.Buffer
	require.NoError(t, a.Stream(&buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	assert.Empty(t, zr.File)
	_, err = l.OpenLibraryArchive("p", "sub")
	assert.ErrorIs(t, err, fs.ErrNotExist)
}

func fillFiles(t *testing.T, dir string, n int) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dir, 0o755))
	for i := 0; i < n; i++ {
		require.NoError(t, os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%03d.txt", i)), []byte("x"), 0o644))
	}
}

func TestLibraryArchiveFileCountLimit(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")

	fillFiles(t, filepath.Join(lib, "exact"), MaxArchiveFiles)
	a, err := l.OpenLibraryArchive("p", "exact")
	require.NoError(t, err, "exactly at the limit is allowed")
	assert.Len(t, a.Files(), MaxArchiveFiles)
	a.Close()

	fillFiles(t, filepath.Join(lib, "over"), MaxArchiveFiles+1)
	_, err = l.OpenLibraryArchive("p", "over")
	var lim *ArchiveLimitError
	require.True(t, errors.As(err, &lim), "one over is refused")
	assert.Equal(t, "files", lim.Kind)
	assert.Equal(t, int64(MaxArchiveFiles), lim.Limit)
	assert.Contains(t, err.Error(), "200 files")

	// Files in nested folders count; dot entries and links do not.
	fillFiles(t, filepath.Join(lib, "nested", "a"), 120)
	fillFiles(t, filepath.Join(lib, "nested", "b"), 81)
	_, err = l.OpenLibraryArchive("p", "nested")
	assert.True(t, errors.As(err, &lim))
	fillFiles(t, filepath.Join(lib, "ignored", "a"), 100)
	fillFiles(t, filepath.Join(lib, "ignored", ".hid"), 500)
	require.NoError(t, os.Symlink(filepath.Join(lib, "over"), filepath.Join(lib, "ignored", "link")))
	a, err = l.OpenLibraryArchive("p", "ignored")
	require.NoError(t, err)
	a.Close()
}

func TestLibraryArchiveByteLimit(t *testing.T) {
	l := Layout{Root: t.TempDir()}
	lib := l.LibraryDir("p")
	big := func(rel string, n int) {
		p := filepath.Join(lib, filepath.FromSlash(rel))
		require.NoError(t, os.MkdirAll(filepath.Dir(p), 0o755))
		require.NoError(t, os.WriteFile(p, bytes.Repeat([]byte("a"), n), 0o644))
	}

	big("exact/f.bin", MaxArchiveBytes)
	a, err := l.OpenLibraryArchive("p", "exact")
	require.NoError(t, err, "exactly at the limit is allowed")
	var buf bytes.Buffer
	require.NoError(t, a.Stream(&buf))
	a.Close()
	assert.Less(t, buf.Len(), 1<<20, "repetitive data compresses")

	big("over/f.bin", MaxArchiveBytes+1)
	_, err = l.OpenLibraryArchive("p", "over")
	var lim *ArchiveLimitError
	require.True(t, errors.As(err, &lim), "one byte over is refused")
	assert.Equal(t, "bytes", lim.Kind)
	assert.Equal(t, int64(MaxArchiveBytes), lim.Limit)
	assert.Contains(t, err.Error(), "40 MB")

	// The total over several files counts, not each file.
	big("sum/a.bin", MaxArchiveBytes/2)
	big("sum/b.bin", MaxArchiveBytes/2)
	a, err = l.OpenLibraryArchive("p", "sum")
	require.NoError(t, err)
	a.Close()
	big("sum/c.bin", 1)
	_, err = l.OpenLibraryArchive("p", "sum")
	assert.True(t, errors.As(err, &lim))
}

func TestLibraryArchiveNeverWritesMoreThanWasCounted(t *testing.T) {
	l, lib := seedLibrary(t)
	writeLib(t, lib, map[string]string{"d/grow.txt": "12345"})
	a, err := l.OpenLibraryArchive("p", "d")
	require.NoError(t, err)
	defer a.Close()
	// The file grows between the count and the write.
	require.NoError(t, os.WriteFile(filepath.Join(lib, "d/grow.txt"), bytes.Repeat([]byte("z"), 1<<20), 0o644))
	var buf bytes.Buffer
	require.NoError(t, a.Stream(&buf))
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)
	assert.Equal(t, uint64(5), zr.File[0].UncompressedSize64)
}

func TestLibraryArchiveDoesNotFollowALinkSwappedInAfterTheCount(t *testing.T) {
	l, lib := seedLibrary(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("SECRET"), 0o644))
	writeLib(t, lib, map[string]string{"d/a.txt": "a"})
	a, err := l.OpenLibraryArchive("p", "d")
	require.NoError(t, err)
	defer a.Close()
	require.NoError(t, os.Remove(filepath.Join(lib, "d/a.txt")))
	require.NoError(t, os.Symlink(filepath.Join(outside, "secret"), filepath.Join(lib, "d/a.txt")))
	var buf bytes.Buffer
	require.NoError(t, a.Stream(&buf))
	assert.NotContains(t, buf.String(), "SECRET")
}
