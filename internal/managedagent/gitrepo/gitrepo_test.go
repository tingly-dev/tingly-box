package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// newRepo builds a work tree with one commit and returns its path.
func newRepo(t *testing.T) (*Git, string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	dir := t.TempDir()
	g := &Git{Env: []string{
		"GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@example.com",
	}}
	ctx := context.Background()
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "."}} {
		if args[0] == "add" {
			if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# repo\n"), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := g.run(ctx, dir, nil, args...); err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
	}
	if _, err := g.run(ctx, dir, nil, "commit", "-q", "-m", "init"); err != nil {
		t.Fatal(err)
	}
	return g, dir
}

// The diff answers the question the UI asks: what changed since the session
// started — tracked edits, new files, and work the agent committed itself.
func TestDiffSinceBase(t *testing.T) {
	ctx := context.Background()
	g, dir := newRepo(t)

	if !g.IsRepo(ctx, dir) {
		t.Fatal("IsRepo should be true for a work tree")
	}
	base, err := g.Head(ctx, dir)
	if err != nil || base == "" {
		t.Fatalf("head: %q %v", base, err)
	}

	// Nothing has happened yet.
	d, err := g.Diff(ctx, dir, base)
	if err != nil || d.ChangedFiles != 0 {
		t.Fatalf("clean tree: %+v %v", d, err)
	}

	// An edit, an untracked file, and a commit: all three belong to this
	// session and must be in the summary.
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# repo\nedited\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err = g.Diff(ctx, dir, base)
	if err != nil || d.ChangedFiles != 2 || len(d.Untracked) != 1 || d.Untracked[0] != "new.txt" {
		t.Fatalf("dirty tree: %+v %v", d, err)
	}
	if n, _ := g.ChangedFiles(ctx, dir, base); n != 2 {
		t.Fatalf("ChangedFiles = %d, want 2", n)
	}

	if _, err := g.run(ctx, dir, nil, "add", "."); err != nil {
		t.Fatal(err)
	}
	if _, err := g.run(ctx, dir, nil, "commit", "-q", "-m", "agent work"); err != nil {
		t.Fatal(err)
	}
	d, err = g.Diff(ctx, dir, base)
	if err != nil || d.ChangedFiles != 2 {
		t.Fatalf("committed work must stay in the session's diff: %+v %v", d, err)
	}
	// Against HEAD, the same tree is clean again — that is the difference
	// the base commit buys.
	if d, err = g.Diff(ctx, dir, ""); err != nil || d.ChangedFiles != 0 {
		t.Fatalf("against HEAD: %+v %v", d, err)
	}
}

// A base commit that no longer resolves (a reset, a fresh clone) falls back
// to HEAD instead of failing the page.
func TestDiffUnknownBaseFallsBackToHead(t *testing.T) {
	ctx := context.Background()
	g, dir := newRepo(t)
	if err := os.WriteFile(filepath.Join(dir, "x.txt"), []byte("x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d, err := g.Diff(ctx, dir, "0000000000000000000000000000000000000000")
	if err != nil || d.ChangedFiles != 1 {
		t.Fatalf("unknown base: %+v %v", d, err)
	}
}

func TestIsRepoFalseForPlainDirectory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	g := &Git{}
	if g.IsRepo(context.Background(), t.TempDir()) {
		t.Fatal("a plain directory is not a repo")
	}
}
