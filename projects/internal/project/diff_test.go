package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func diffRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	run(t, dir, "git", "config", "user.email", "a@b.c")
	run(t, dir, "git", "config", "user.name", "t")
	write := func(p, s string) {
		require.NoError(t, os.MkdirAll(filepath.Dir(filepath.Join(dir, p)), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, p), []byte(s), 0o644))
	}
	lines := func(n int, tag string) string {
		var b strings.Builder
		for i := 0; i < n; i++ {
			fmt.Fprintf(&b, "%s line %d\n", tag, i)
		}
		return b.String()
	}
	write("keep.txt", lines(20, "keep"))
	write("old/moved.txt", lines(40, "moved"))
	write("gone.txt", lines(5, "gone"))
	write("big.txt", lines(30000, "big"))
	write("package-lock.json", lines(10, "lock"))
	write("pic.bin", "a\x00b\x00c")
	write("-weird.txt", "x\n")
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "base")
	run(t, dir, "git", "checkout", "-qb", "feature")
	write("keep.txt", lines(20, "keep")+"added\n")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "new"), 0o755))
	run(t, dir, "git", "mv", "old/moved.txt", "new/moved.txt")
	write("new/moved.txt", lines(40, "moved")+"tail\n")
	run(t, dir, "git", "rm", "-q", "gone.txt")
	write("big.txt", strings.ReplaceAll(lines(30000, "big"), "line", "LINE"))
	write("package-lock.json", lines(10, "locked"))
	write("pic.bin", "a\x00b\x00d")
	write("fresh/new.txt", "n\n")
	write("-weird.txt", "y\n")
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "change")
	return dir
}

func TestBranchDiffSummarizesFilesWithoutContent(t *testing.T) {
	dir := diffRepo(t)
	d, err := BranchDiff(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	by := map[string]DiffFile{}
	for _, f := range d.Files {
		by[f.Path] = f
	}
	assert.Len(t, d.Files, 8)
	assert.Equal(t, DiffFile{Path: "keep.txt", Status: "modified", Additions: 1}, by["keep.txt"])
	assert.Equal(t, DiffFile{Path: "new/moved.txt", OldPath: "old/moved.txt", Status: "renamed", Additions: 1}, by["new/moved.txt"])
	assert.Equal(t, DiffFile{Path: "gone.txt", Status: "deleted", Deletions: 5}, by["gone.txt"])
	assert.Equal(t, DiffFile{Path: "fresh/new.txt", Status: "added", Additions: 1}, by["fresh/new.txt"])
	assert.Equal(t, DiffFile{Path: "pic.bin", Status: "modified", Binary: true}, by["pic.bin"])
	assert.True(t, by["package-lock.json"].Generated, "lock files are generated")
	assert.False(t, by["big.txt"].Generated)
	assert.Equal(t, 30000, by["big.txt"].Additions)
	assert.Equal(t, 30000+1+1+10+1+1, d.Additions)
}

func TestBranchDiffHonoursLinguistGenerated(t *testing.T) {
	dir := diffRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".gitattributes"), []byte("keep.txt linguist-generated\n"), 0o644))
	d, err := BranchDiff(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	for _, f := range d.Files {
		if f.Path == "keep.txt" {
			assert.True(t, f.Generated)
		}
	}
}

func TestBranchFileDiffCapsAndForces(t *testing.T) {
	dir := diffRepo(t)
	ctx := context.Background()

	small, err := BranchFileDiff(ctx, dir, "main", "keep.txt", "", false)
	require.NoError(t, err)
	assert.Contains(t, small.Patch, "+added")
	assert.False(t, small.TooLarge)

	big, err := BranchFileDiff(ctx, dir, "main", "big.txt", "", false)
	require.NoError(t, err)
	assert.True(t, big.TooLarge)
	assert.Empty(t, big.Patch, "an over-size patch is not sent")

	forced, err := BranchFileDiff(ctx, dir, "main", "big.txt", "", true)
	require.NoError(t, err)
	assert.False(t, forced.TooLarge)
	assert.False(t, forced.Truncated)
	assert.Contains(t, forced.Patch, "+big LINE 29999")

	bin, err := BranchFileDiff(ctx, dir, "main", "pic.bin", "", false)
	require.NoError(t, err)
	assert.True(t, bin.Binary)
	assert.Empty(t, bin.Patch)
}

func TestBranchFileDiffRenameAndPathspecs(t *testing.T) {
	dir := diffRepo(t)
	ctx := context.Background()
	ren, err := BranchFileDiff(ctx, dir, "main", "new/moved.txt", "old/moved.txt", false)
	require.NoError(t, err)
	assert.Contains(t, ren.Patch, "rename from old/moved.txt")
	assert.Contains(t, ren.Patch, "+tail")

	// A name that starts with a dash, or looks like pathspec magic, is a file name.
	dash, err := BranchFileDiff(ctx, dir, "main", "-weird.txt", "", false)
	require.NoError(t, err)
	assert.Contains(t, dash.Patch, "+y")
	none, err := BranchFileDiff(ctx, dir, "main", ":(top)*", "", false)
	require.NoError(t, err)
	assert.Empty(t, none.Patch)

	_, err = BranchFileDiff(ctx, dir, "main", "", "", false)
	assert.Error(t, err)
}

func TestCutLongLines(t *testing.T) {
	short := "@@ -1 +1 @@\n-a\n+b\n"
	got, n := cutLongLines(short)
	assert.Equal(t, short, got)
	assert.Zero(t, n)

	long := strings.Repeat("x", 5000)
	got, n = cutLongLines("@@ -1 +1 @@\n+" + long + "\n+tail\n")
	assert.Equal(t, 1, n)
	assert.Equal(t, "@@ -1 +1 @@\n+"+strings.Repeat("x", maxLineBytes-1)+" … [+"+"3001"+" chars]\n+tail\n", got)

	// A cut never splits a character, and a last line with no newline stays so.
	multi := "+" + strings.Repeat("é", 3000)
	got, n = cutLongLines(multi)
	assert.Equal(t, 1, n)
	assert.True(t, utf8.ValidString(got))
	assert.True(t, strings.HasSuffix(got, "chars]"))
}

func TestBranchFileDiffShortensLongLines(t *testing.T) {
	dir := diffRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "min.js"), []byte("var a=["+strings.Repeat("1,", 350000)+"];\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "bundle")
	d, err := BranchFileDiff(context.Background(), dir, "main", "min.js", "", false)
	require.NoError(t, err)
	assert.True(t, d.TooLarge, "a 700 KB line is more than the default read")
	forced, err := BranchFileDiff(context.Background(), dir, "main", "min.js", "", true)
	require.NoError(t, err)
	assert.Equal(t, 1, forced.LinesCut)
	assert.Less(t, len(forced.Patch), 4000)
}

func TestBranchDiffListsAtMostMaxFiles(t *testing.T) {
	dir := diffRepo(t)
	old := maxListedFiles
	maxListedFiles = 3
	t.Cleanup(func() { maxListedFiles = old })
	d, err := BranchDiff(context.Background(), dir, "main", "feature")
	require.NoError(t, err)
	assert.Len(t, d.Files, 3)
	assert.Equal(t, 5, d.FilesOmitted)
	assert.Equal(t, 30000+1+1+10+1+1, d.Additions, "the totals still cover every file")
}

func TestBranchFileDiffTruncatesOnAWholeLine(t *testing.T) {
	dir := diffRepo(t)
	old := maxForcedPatch
	maxForcedPatch = 1000
	t.Cleanup(func() { maxForcedPatch = old })
	d, err := BranchFileDiff(context.Background(), dir, "main", "big.txt", "", true)
	require.NoError(t, err)
	assert.True(t, d.Truncated)
	assert.True(t, strings.HasSuffix(d.Patch, "\n"))
	assert.LessOrEqual(t, len(d.Patch), 1000)
}
