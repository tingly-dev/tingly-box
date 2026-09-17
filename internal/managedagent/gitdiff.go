// gitdiff.go is the read-only git view of a folder the agent works in.
// It answers one question — what changed while the agent was working — by
// running git in the user's own working tree. Nothing here writes: no
// clone, no commit, no branch, no push. Committing and publishing stay with
// the person who owns the folder (.design/managed-agent.md §13).
package managedagent

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Git runs git commands against a working tree.
type Git struct {
	// Bin is the git executable; empty means "git" on PATH.
	Bin string
	// Env is appended to git's environment (identity for tests).
	Env []string
}

// Diff summarises what changed in a working tree since a base commit: the
// tree (committed or not) against that base, plus untracked files, which
// `git diff` alone would miss.
type Diff struct {
	ChangedFiles int      `json:"changed_files"`
	Stat         string   `json:"stat"`
	Patch        string   `json:"patch"`
	Untracked    []string `json:"untracked,omitempty"`
	Truncated    bool     `json:"truncated"`
}

// MaxPatchBytes caps the patch returned by Diff; the stat is always complete.
const MaxPatchBytes = 512 * 1024

// baseOrHead resolves the diff baseline: the given commit, or HEAD when it
// is empty or no longer resolvable (a rebase, a reset, a fresh repository).
func (g *Git) baseOrHead(ctx context.Context, dir, base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return "HEAD"
	}
	if _, err := g.run(ctx, dir, nil, "cat-file", "-e", base+"^{commit}"); err != nil {
		return "HEAD"
	}
	return base
}

// Diff computes the change summary of dir since base ("" = HEAD).
func (g *Git) Diff(ctx context.Context, dir, base string) (*Diff, error) {
	ref := g.baseOrHead(ctx, dir, base)
	out := &Diff{}
	stat, err := g.run(ctx, dir, nil, "diff", "--stat", ref)
	if err != nil {
		return nil, err
	}
	out.Stat = strings.TrimRight(stat, "\n")
	names, err := g.run(ctx, dir, nil, "diff", "--name-only", ref)
	if err != nil {
		return nil, err
	}
	for _, n := range strings.Split(strings.TrimSpace(names), "\n") {
		if n != "" {
			out.ChangedFiles++
		}
	}
	untracked, err := g.run(ctx, dir, nil, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, n := range strings.Split(strings.TrimSpace(untracked), "\n") {
		if n != "" {
			out.Untracked = append(out.Untracked, n)
		}
	}
	out.ChangedFiles += len(out.Untracked)
	patch, err := g.run(ctx, dir, nil, "diff", ref)
	if err != nil {
		return nil, err
	}
	if len(patch) > MaxPatchBytes {
		patch = patch[:MaxPatchBytes]
		out.Truncated = true
	}
	out.Patch = patch
	return out, nil
}

// ChangedFiles counts changed and untracked files without building a patch.
func (g *Git) ChangedFiles(ctx context.Context, dir, base string) (int, error) {
	ref := g.baseOrHead(ctx, dir, base)
	names, err := g.run(ctx, dir, nil, "diff", "--name-only", ref)
	if err != nil {
		return 0, err
	}
	untracked, err := g.run(ctx, dir, nil, "ls-files", "--others", "--exclude-standard")
	if err != nil {
		return 0, err
	}
	return countLines(names) + countLines(untracked), nil
}

func countLines(s string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}

// IsRepo reports whether dir is inside a git work tree.
func (g *Git) IsRepo(ctx context.Context, dir string) bool {
	out, err := g.run(ctx, dir, nil, "rev-parse", "--is-inside-work-tree")
	return err == nil && strings.TrimSpace(out) == "true"
}

// Head is the current commit id, empty for a repository with no commits yet.
func (g *Git) Head(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, nil, "rev-parse", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(out), nil
}

func (g *Git) run(ctx context.Context, dir string, log func(string), args ...string) (string, error) {
	bin := g.Bin
	if bin == "" {
		bin = "git"
	}
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	// Never block on a credential or host-key prompt: a managed run has no
	// terminal to answer it, so fail fast and surface the error instead.
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GCM_INTERACTIVE=never")
	cmd.Env = append(cmd.Env, g.Env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if log != nil {
		log("$ git " + strings.Join(args, " "))
	}
	err := cmd.Run()
	if err != nil {
		var exit *exec.ExitError
		code := ""
		if errors.As(err, &exit) {
			code = " (exit " + strconv.Itoa(exit.ExitCode()) + ")"
		}
		return stdout.String(), fmt.Errorf("git %s%s: %s", args[0], code, firstLine(stderr.String(), err.Error()))
	}
	return stdout.String(), nil
}

func firstLine(s, fallback string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return fallback
	}
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
