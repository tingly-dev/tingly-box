package project

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// git runs a git command in dir and returns trimmed stdout.
func git(ctx context.Context, dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(errb.String())
		if msg == "" {
			msg = err.Error()
		}
		return "", fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return strings.TrimRight(out.String(), "\n"), nil
}

// RepoInfo describes a local repository.
type RepoInfo struct {
	Root   string `json:"root"`
	Branch string `json:"branch"`
	Dirty  bool   `json:"dirty"`
}

// InspectRepo checks that path is inside a git work tree with at least one
// commit and returns its root and current branch.
func InspectRepo(ctx context.Context, path string) (RepoInfo, error) {
	root, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoInfo{}, fmt.Errorf("%s is not a git repository", path)
	}
	if _, err := git(ctx, root, "rev-parse", "--verify", "HEAD"); err != nil {
		return RepoInfo{}, fmt.Errorf("%s has no commits yet", root)
	}
	branch, err := git(ctx, root, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return RepoInfo{}, err
	}
	status, _ := git(ctx, root, "status", "--porcelain")
	return RepoInfo{Root: root, Branch: branch, Dirty: status != ""}, nil
}

// HeadBranch is the branch a folder is on; empty when the folder is not in
// a repository or its HEAD is detached.
func HeadBranch(ctx context.Context, dir string) string {
	b, err := git(ctx, dir, "branch", "--show-current")
	if err != nil {
		return ""
	}
	return b
}

// AddWorktree creates branch from base in a new worktree at dir.
func AddWorktree(ctx context.Context, repo, dir, branch, base string) error {
	_, err := git(ctx, repo, "worktree", "add", "-b", branch, dir, base)
	if err != nil {
		// git may have made the branch before failing to make the directory;
		// leave no stray branch, so the caller can try another place.
		_, _ = git(ctx, repo, "branch", "-D", branch)
	}
	return err
}

// excludeWorktrees keeps <repo>/.claude/worktrees out of `git status` for
// every worktree of the repository, without touching the user's .gitignore.
func excludeWorktrees(ctx context.Context, repo string) error {
	p, err := git(ctx, repo, "rev-parse", "--git-path", "info/exclude")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(p) {
		p = filepath.Join(repo, p)
	}
	const line = "/.claude/worktrees/"
	if b, err := os.ReadFile(p); err == nil {
		for _, l := range strings.Split(string(b), "\n") {
			if strings.TrimSpace(l) == line {
				return nil
			}
		}
		if len(b) > 0 && b[len(b)-1] != '\n' {
			return appendFile(p, "\n"+line+"\n")
		}
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return appendFile(p, line+"\n")
}

func appendFile(path, text string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(text)
	return err
}

// RemoveWorktree removes a thread's worktree, keeping its branch.
func RemoveWorktree(ctx context.Context, repo, dir string) error {
	_, err := git(ctx, repo, "worktree", "remove", "--force", dir)
	return err
}

// BranchStats is what a thread's branch holds relative to its base.
type BranchStats struct {
	CommitsAhead int
	FilesChanged int
	Uncommitted  bool
}

// Stats compares a worktree's HEAD with base.
func Stats(ctx context.Context, worktree, base string) BranchStats {
	var s BranchStats
	if n, err := git(ctx, worktree, "rev-list", "--count", base+"..HEAD"); err == nil {
		s.CommitsAhead, _ = strconv.Atoi(n)
	}
	if names, err := git(ctx, worktree, "diff", "--name-only", base+"...HEAD"); err == nil && names != "" {
		s.FilesChanged = len(strings.Split(names, "\n"))
	}
	if st, err := git(ctx, worktree, "status", "--porcelain"); err == nil && st != "" {
		s.Uncommitted = true
	}
	return s
}

// Diff is a thread's changes for review.
type Diff struct {
	Base        string `json:"base"`
	Branch      string `json:"branch"`
	Stat        string `json:"stat"`
	Patch       string `json:"patch"`
	Uncommitted string `json:"uncommitted"`
	Truncated   bool   `json:"truncated"`
}

const maxPatch = 400 << 10

// BranchDiff returns base...HEAD of a worktree plus any uncommitted changes.
func BranchDiff(ctx context.Context, worktree, base, branch string) (Diff, error) {
	d := Diff{Base: base, Branch: branch}
	var err error
	if d.Stat, err = git(ctx, worktree, "diff", "--stat", base+"...HEAD"); err != nil {
		return d, err
	}
	if d.Patch, err = git(ctx, worktree, "diff", base+"...HEAD"); err != nil {
		return d, err
	}
	d.Uncommitted, _ = git(ctx, worktree, "status", "--short")
	if len(d.Patch) > maxPatch {
		d.Patch = d.Patch[:maxPatch]
		d.Truncated = true
	}
	return d, nil
}

// MergeBranch merges branch into base in the main checkout of repo. It
// refuses unless that checkout is on base with no local changes, so the
// user's working copy is never touched in a surprising state.
func MergeBranch(ctx context.Context, repo, base, branch, message string) error {
	info, err := InspectRepo(ctx, repo)
	if err != nil {
		return err
	}
	if info.Branch != base {
		return &MergeBlockedError{Reason: fmt.Sprintf("%s is checked out on %s, not %s", info.Root, info.Branch, base),
			Command: fmt.Sprintf("git -C %q checkout %s && git -C %q merge --no-ff %s", info.Root, base, info.Root, branch)}
	}
	if info.Dirty {
		return &MergeBlockedError{Reason: fmt.Sprintf("%s has uncommitted changes", info.Root),
			Command: fmt.Sprintf("git -C %q merge --no-ff %s", info.Root, branch)}
	}
	if _, err := git(ctx, repo, "merge", "--no-ff", "-m", message, branch); err != nil {
		_, _ = git(ctx, repo, "merge", "--abort")
		return &MergeBlockedError{Reason: "the merge has conflicts: " + err.Error(),
			Command: fmt.Sprintf("git -C %q merge --no-ff %s", info.Root, branch)}
	}
	return nil
}

// MergeBlockedError explains why a merge was not done and hands over the
// command to do it by hand.
type MergeBlockedError struct {
	Reason  string
	Command string
}

func (e *MergeBlockedError) Error() string { return e.Reason }
