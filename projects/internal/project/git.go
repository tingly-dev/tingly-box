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
	// core.fsmonitor names a command git runs; a repository a sandboxed thread
	// could write must not get the host to run it (thread-runtime.md §10.5).
	cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.fsmonitor=false", "-C", dir}, args...)...)
	// Git diagnostics are interpreted by directory inspection; keep them stable
	// even when the user's shell uses another locale.
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANGUAGE=C")
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
	// Empty is true when the repository has no commits yet (an unborn HEAD).
	// It is still usable in shared mode; only worktree mode needs a commit to
	// branch from.
	Empty bool `json:"empty,omitempty"`
}

// InspectRepo checks that path is inside a git work tree and returns its root
// and current branch. A repository with no commits (a fresh `git init`, or a
// clone of an empty remote) is reported with Empty rather than refused: shared
// threads can work in it, and refusing it would block adding the directory.
func InspectRepo(ctx context.Context, path string) (RepoInfo, error) {
	root, err := git(ctx, path, "rev-parse", "--show-toplevel")
	if err != nil {
		return RepoInfo{}, fmt.Errorf("%s is not a git repository", path)
	}
	info := RepoInfo{Root: root}
	if _, err := git(ctx, root, "rev-parse", "--verify", "--quiet", "HEAD"); err != nil {
		// Unborn HEAD: there is no commit to resolve, but symbolic-ref still
		// names the branch the first commit will go to.
		info.Empty = true
		if branch, err := git(ctx, root, "symbolic-ref", "--short", "HEAD"); err == nil {
			info.Branch = branch
		}
	} else if branch, err := git(ctx, root, "rev-parse", "--abbrev-ref", "HEAD"); err == nil {
		// Includes a detached HEAD ("HEAD"), which worktree mode can branch from.
		info.Branch = branch
	}
	status, _ := git(ctx, root, "status", "--porcelain")
	info.Dirty = status != ""
	return info, nil
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
	if BranchExists(ctx, repo, branch) {
		return fmt.Errorf("branch %q already exists", branch)
	}
	_, err := git(ctx, repo, "worktree", "add", "-b", branch, dir, base)
	// Do not delete a branch on failure: it may have existed before this call
	// (or been created concurrently by another process).
	return err
}

// BranchExists reports whether repo has a local branch of that name.
func BranchExists(ctx context.Context, repo, name string) bool {
	_, err := git(ctx, repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+name)
	return err == nil
}

// checkNewBranch refuses a branch name git cannot use or that is taken.
func checkNewBranch(ctx context.Context, repo, name string) error {
	if err := checkBranchName(ctx, repo, name); err != nil {
		return err
	}
	if BranchExists(ctx, repo, name) {
		return fmt.Errorf("branch %q already exists in %s; choose another name", name, repo)
	}
	return nil
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

// checkBranchName refuses a name git cannot use as a branch name.
func checkBranchName(ctx context.Context, dir, name string) error {
	if strings.HasPrefix(name, "-") {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	if _, err := git(ctx, dir, "check-ref-format", "refs/heads/"+name); err != nil {
		return fmt.Errorf("%q is not a valid branch name", name)
	}
	return nil
}
