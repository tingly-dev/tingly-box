package gitrepo

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func sh(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@x", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@x")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// newOrigin builds a bare "remote" with one commit on main.
func newOrigin(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	work := filepath.Join(root, "work")
	sh(t, root, "init", "-q", "-b", "main", work)
	os.WriteFile(filepath.Join(work, "README.md"), []byte("hello\n"), 0o644)
	sh(t, work, "add", ".")
	sh(t, work, "commit", "-q", "-m", "init")
	bare := filepath.Join(root, "origin.git")
	sh(t, root, "clone", "-q", "--bare", work, bare)
	return bare
}

func TestProvisionDiffPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	ctx := context.Background()
	origin := newOrigin(t)
	g := &Git{MirrorsDir: filepath.Join(t.TempDir(), "mirrors")}
	var lines []string
	log := func(s string) { lines = append(lines, s) }

	ws := filepath.Join(t.TempDir(), "ws", "repo")
	if err := g.Provision(ctx, ProvisionRequest{URL: origin, BaseRef: "main", Branch: "tb/x", Dir: ws, Log: log}); err != nil {
		t.Fatalf("provision: %v\n%s", err, strings.Join(lines, "\n"))
	}
	if _, err := os.Stat(filepath.Join(g.MirrorPath(origin), "HEAD")); err != nil {
		t.Fatalf("mirror not created: %v", err)
	}
	if got := strings.TrimSpace(sh(t, ws, "rev-parse", "--abbrev-ref", "HEAD")); got != "tb/x" {
		t.Fatalf("branch = %q", got)
	}
	// Dissociated: no alternates file pointing at the mirror.
	if _, err := os.Stat(filepath.Join(ws, ".git", "objects", "info", "alternates")); err == nil {
		t.Fatal("workspace still borrows objects from the mirror")
	}

	// A second workspace hits the existing mirror (remote update path).
	ws2 := filepath.Join(t.TempDir(), "ws2", "repo")
	if err := g.Provision(ctx, ProvisionRequest{URL: origin, BaseRef: "main", Branch: "tb/y", Dir: ws2}); err != nil {
		t.Fatal(err)
	}

	// Changes: one committed, one working-tree edit, one untracked.
	os.WriteFile(filepath.Join(ws, "a.txt"), []byte("a\n"), 0o644)
	sh(t, ws, "add", "a.txt")
	sh(t, ws, "commit", "-q", "-m", "add a")
	os.WriteFile(filepath.Join(ws, "README.md"), []byte("hello world\n"), 0o644)
	os.WriteFile(filepath.Join(ws, "new.txt"), []byte("n\n"), 0o644)

	d, err := g.Diff(ctx, ws, "main")
	if err != nil {
		t.Fatal(err)
	}
	if d.ChangedFiles != 3 || len(d.Untracked) != 1 || !strings.Contains(d.Patch, "hello world") || !strings.Contains(d.Stat, "a.txt") {
		t.Fatalf("diff = %+v", d)
	}
	if dirty, _ := g.HasUncommitted(ctx, ws); !dirty {
		t.Fatal("expected uncommitted changes")
	}

	if err := g.Push(ctx, ws, "tb/x", log); err != nil {
		t.Fatalf("push: %v\n%s", err, strings.Join(lines, "\n"))
	}
	if out := sh(t, origin, "branch", "--list", "tb/x"); !strings.Contains(out, "tb/x") {
		t.Fatalf("branch not on origin: %q", out)
	}

	// Existing dir is refused.
	if err := g.Provision(ctx, ProvisionRequest{URL: origin, Branch: "b", Dir: ws}); err == nil {
		t.Fatal("expected error for existing dir")
	}
	// Bad URL with mirroring falls back and still fails cleanly.
	if err := g.Provision(ctx, ProvisionRequest{URL: filepath.Join(t.TempDir(), "nope.git"), Branch: "b", Dir: filepath.Join(t.TempDir(), "x")}); err == nil {
		t.Fatal("expected clone failure")
	}
}
