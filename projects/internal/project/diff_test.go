package project

import (
	"context"
	"encoding/json"
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
	d, err := LocalPRDiff(context.Background(), dir, 7, "main", "feature")
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
	d, err := LocalPRDiff(context.Background(), dir, 7, "main", "feature")
	require.NoError(t, err)
	for _, f := range d.Files {
		if f.Path == "keep.txt" {
			assert.True(t, f.Generated)
		}
	}
}

func TestPRFileDiffSkipsWhatIsTooLargeOrBinary(t *testing.T) {
	dir := diffRepo(t)
	ctx := context.Background()

	small, err := LocalPRFileDiff(ctx, dir, "main", "feature", "keep.txt", "")
	require.NoError(t, err)
	assert.Contains(t, small.Patch, "+added")
	assert.False(t, small.TooLarge)

	big, err := LocalPRFileDiff(ctx, dir, "main", "feature", "big.txt", "")
	require.NoError(t, err)
	assert.True(t, big.TooLarge)
	assert.Empty(t, big.Patch, "an over-size patch is not sent, and cannot be asked for")

	bin, err := LocalPRFileDiff(ctx, dir, "main", "feature", "pic.bin", "")
	require.NoError(t, err)
	assert.True(t, bin.Binary)
	assert.Empty(t, bin.Patch)
}

func TestPRFileDiffRenameAndPathspecs(t *testing.T) {
	dir := diffRepo(t)
	ctx := context.Background()
	ren, err := LocalPRFileDiff(ctx, dir, "main", "feature", "new/moved.txt", "old/moved.txt")
	require.NoError(t, err)
	assert.Contains(t, ren.Patch, "rename from old/moved.txt")
	assert.Contains(t, ren.Patch, "+tail")

	// A name that starts with a dash, or looks like pathspec magic, is a file name.
	dash, err := LocalPRFileDiff(ctx, dir, "main", "feature", "-weird.txt", "")
	require.NoError(t, err)
	assert.Contains(t, dash.Patch, "+y")
	none, err := LocalPRFileDiff(ctx, dir, "main", "feature", ":(top)*", "")
	require.NoError(t, err)
	assert.Empty(t, none.Patch)

	_, err = LocalPRFileDiff(ctx, dir, "main", "feature", "", "")
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

func TestPRFileDiffShortensLongLines(t *testing.T) {
	dir := diffRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "min.js"), []byte("var a=["+strings.Repeat("1,", 50000)+"];\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "bundle")
	d, err := LocalPRFileDiff(context.Background(), dir, "main", "feature", "min.js", "")
	require.NoError(t, err)
	assert.False(t, d.TooLarge)
	assert.Equal(t, 1, d.LinesCut)
	assert.Less(t, len(d.Patch), 4000)
}

func TestBranchDiffListsAtMostMaxFiles(t *testing.T) {
	dir := diffRepo(t)
	old := maxListedFiles
	maxListedFiles = 3
	t.Cleanup(func() { maxListedFiles = old })
	d, err := LocalPRDiff(context.Background(), dir, 7, "main", "feature")
	require.NoError(t, err)
	assert.Len(t, d.Files, 3)
	assert.Equal(t, 5, d.FilesOmitted)
	assert.Equal(t, 30000+1+1+10+1+1, d.Additions, "the totals still cover every file")
}

// The review dialog reads files.length, so a branch with no changes lists
// [] and not null.
func TestBranchDiffOfAnUnchangedBranchListsNoFilesAsAnEmptyList(t *testing.T) {
	dir := diffRepo(t)
	run(t, dir, "git", "checkout", "-q", "main")
	run(t, dir, "git", "checkout", "-qb", "same")
	d, err := LocalPRDiff(context.Background(), dir, 7, "main", "same")
	require.NoError(t, err)
	assert.NotNil(t, d.Files)
	assert.Empty(t, d.Files)
	b, err := json.Marshal(d)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"files":[]`)
}

// originOf gives the repository a bare origin that both branches are pushed to.
func originOf(t *testing.T, dir string) string {
	t.Helper()
	remote := t.TempDir()
	run(t, remote, "git", "init", "-q", "--bare", "-b", "main")
	run(t, dir, "git", "remote", "add", "origin", remote)
	run(t, dir, "git", "push", "-q", "origin", "main", "feature")
	return remote
}

func TestLocalPRDiffReadsOriginsCopiesAndSaysWhatIsNotInThePR(t *testing.T) {
	dir := diffRepo(t)
	originOf(t, dir)
	ctx := context.Background()

	d, err := LocalPRDiff(ctx, dir, 7, "main", "feature")
	require.NoError(t, err)
	assert.Equal(t, "local", d.Source)
	assert.False(t, d.BaseStale, "origin's base was fetched")
	assert.Zero(t, d.Unpushed)
	assert.Len(t, d.Files, 8)

	// A commit that was not pushed is not in the PR: the diff stays what GitHub has, and says so.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "later.txt"), []byte("l\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "later")
	d, err = LocalPRDiff(ctx, dir, 7, "main", "feature")
	require.NoError(t, err)
	assert.Equal(t, 1, d.Unpushed)
	assert.Len(t, d.Files, 8, "the unpushed file is not listed")
	_, err = LocalPRFileDiff(ctx, dir, "main", "feature", "keep.txt", "")
	require.NoError(t, err)
}

func TestLocalPRDiffUsesTheBaseOriginHasNow(t *testing.T) {
	dir := diffRepo(t)
	remote := originOf(t, dir)
	// Someone else moved main on GitHub; a stale origin/main would list their file in this PR.
	other := t.TempDir()
	run(t, other, "git", "clone", "-q", "-b", "main", remote, ".")
	run(t, other, "git", "config", "user.email", "a@b.c")
	run(t, other, "git", "config", "user.name", "t")
	require.NoError(t, os.WriteFile(filepath.Join(other, "keep.txt"), []byte("keep\n"), 0o644))
	run(t, other, "git", "commit", "-qam", "main moved")
	run(t, other, "git", "push", "-q", "origin", "main")

	d, err := LocalPRDiff(context.Background(), dir, 7, "main", "feature")
	require.NoError(t, err)
	assert.False(t, d.BaseStale)
	assert.Equal(t, run(t, remote, "git", "rev-parse", "main"), run(t, dir, "git", "rev-parse", "refs/remotes/origin/main"), "origin's base was brought up to date")
	assert.Len(t, d.Files, 8, "the range starts at the merge-base, so the PR's files are the same")
}

func TestLocalPRDiffOfAStackIsEachPRsOwn(t *testing.T) {
	dir := diffRepo(t)
	run(t, dir, "git", "checkout", "-qb", "feature-2")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "second.txt"), []byte("2\n"), 0o644))
	run(t, dir, "git", "add", ".")
	run(t, dir, "git", "commit", "-qm", "second")
	d, err := LocalPRDiff(context.Background(), dir, 8, "feature", "feature-2")
	require.NoError(t, err)
	require.Len(t, d.Files, 1)
	assert.Equal(t, "second.txt", d.Files[0].Path)
	assert.True(t, d.BaseStale, "no origin: the base is the local branch, and the diff says it could not be refreshed")
}

func TestLocalPRDiffWithoutTheBranchesAsksForTheRemote(t *testing.T) {
	dir := diffRepo(t)
	for _, c := range [][2]string{{"main", "gone"}, {"gone", "feature"}, {"", "feature"}, {"-x", "feature"}} {
		_, err := LocalPRDiff(context.Background(), dir, 7, c[0], c[1])
		assert.ErrorIs(t, err, errNoLocalRefs, "%v", c)
		_, err = LocalPRFileDiff(context.Background(), dir, c[0], c[1], "keep.txt", "")
		assert.ErrorIs(t, err, errNoLocalRefs, "%v", c)
	}
}

func TestRemotePRDiffFromGitHubsFiles(t *testing.T) {
	files := []GitHubFile{
		{Path: "a.go", Status: "modified", Additions: 2, Deletions: 1, Patch: "@@ -1,2 +1,3 @@\n x\n-y\n+z\n+w"},
		{Path: "new/b.go", OldPath: "old/b.go", Status: "renamed", Additions: 1, Patch: "@@ -1 +1,2 @@\n x\n+y"},
		{Path: "gone.txt", Status: "removed", Deletions: 3, Patch: "@@ -1,3 +0,0 @@\n-a\n-b\n-c"},
		{Path: "pic.png", Status: "added"},
		{Path: "huge.txt", Status: "modified", Additions: 90000},
		{Path: "package-lock.json", Status: "modified", Additions: 1, Patch: "@@ -1 +1 @@\n-a\n+b"},
	}
	d := remotePRDiff(7, "main", "feature", files)
	assert.Equal(t, "github", d.Source)
	assert.Equal(t, 90004, d.Additions)
	require.Len(t, d.Files, 6)
	assert.Equal(t, "deleted", d.Files[2].Status)
	assert.True(t, d.Files[3].Binary)
	assert.False(t, d.Files[4].Binary, "a file with changes and no patch is too large, not binary")
	assert.True(t, d.Files[5].Generated)

	fd, err := remotePRFileDiff(files, "new/b.go")
	require.NoError(t, err)
	assert.Contains(t, fd.Patch, "diff --git a/old/b.go b/new/b.go")
	assert.Contains(t, fd.Patch, "@@ -1 +1,2 @@")
	assert.True(t, strings.HasSuffix(fd.Patch, "+y\n"))
	big, _ := remotePRFileDiff(files, "huge.txt")
	assert.True(t, big.TooLarge)
	bin, _ := remotePRFileDiff(files, "pic.png")
	assert.True(t, bin.Binary)
	_, err = remotePRFileDiff(files, "nope")
	assert.Error(t, err)

	empty := remotePRDiff(7, "main", "feature", nil)
	b, err := json.Marshal(empty)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"files":[]`)
}
