package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
