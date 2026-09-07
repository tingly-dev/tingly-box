// Package gitrepo materialises and inspects managed agent workspaces with
// the system git binary. It knows nothing about sessions: inputs are URLs,
// refs and directories, so it can back the local runtime today and the host
// side of the docker runtime later (git never runs inside the container,
// .design/managed-agent.md §5.3).
//
// Layout: one bare mirror per source under MirrorsDir, and one standalone
// clone per workspace. The clone borrows objects from the mirror at clone
// time and then dissociates, so the workspace is a plain directory — safe
// to bind-mount, safe to rm -rf — while repeat sessions on a source only
// pay an incremental fetch. Worktrees and shared alternates were rejected
// because both embed absolute host paths that break inside a container.
package gitrepo

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// Git runs git against a mirrors directory.
type Git struct {
	// MirrorsDir holds one bare mirror per source URL. Empty disables
	// mirroring and every workspace is a direct clone.
	MirrorsDir string
	// Bin is the git executable; empty means "git" on PATH.
	Bin string
	// Env is appended to git's environment (credential helpers, identity).
	Env []string
}

// ProvisionRequest describes one workspace checkout.
type ProvisionRequest struct {
	URL     string
	BaseRef string
	Branch  string
	Dir     string
	// Log receives one line per git step; nil discards.
	Log func(line string)
}

// Provision clones URL at BaseRef into Dir and creates Branch from it. Dir
// must not exist yet. A mirror failure degrades to a direct clone rather
// than failing the workspace: the mirror is an optimisation, not a source
// of truth.
func (g *Git) Provision(ctx context.Context, req ProvisionRequest) error {
	if req.URL == "" || req.Dir == "" || req.Branch == "" {
		return errors.New("gitrepo: url, dir and branch are required")
	}
	if _, err := os.Stat(req.Dir); err == nil {
		return fmt.Errorf("gitrepo: %s already exists", req.Dir)
	}
	if err := os.MkdirAll(filepath.Dir(req.Dir), 0o700); err != nil {
		return fmt.Errorf("gitrepo: create parent: %w", err)
	}

	args := []string{"clone", "--no-tags"}
	if req.BaseRef != "" {
		args = append(args, "--branch", req.BaseRef)
	}
	if mirror, err := g.ensureMirror(ctx, req.URL, req.Log); err != nil {
		logf(req.Log, "mirror unavailable (%v); cloning directly", err)
	} else if mirror != "" {
		args = append(args, "--reference-if-able", mirror, "--dissociate")
	}
	args = append(args, req.URL, req.Dir)
	if _, err := g.run(ctx, "", req.Log, args...); err != nil {
		return err
	}
	if _, err := g.run(ctx, req.Dir, req.Log, "checkout", "-b", req.Branch); err != nil {
		return err
	}
	return nil
}

// ensureMirror creates or refreshes the bare mirror for url and returns its
// path, or "" when mirroring is disabled.
func (g *Git) ensureMirror(ctx context.Context, url string, log func(string)) (string, error) {
	if g.MirrorsDir == "" {
		return "", nil
	}
	dir := g.MirrorPath(url)
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); err == nil {
		_, err := g.run(ctx, dir, log, "remote", "update", "--prune")
		return dir, err
	}
	if err := os.MkdirAll(g.MirrorsDir, 0o700); err != nil {
		return "", err
	}
	_, err := g.run(ctx, "", log, "clone", "--mirror", url, dir)
	return dir, err
}

// MirrorPath is the bare mirror directory for a URL: a readable stem plus a
// hash, so two URLs with the same repo name never share a mirror.
func (g *Git) MirrorPath(url string) string {
	sum := sha256.Sum256([]byte(url))
	stem := strings.TrimSuffix(filepath.Base(strings.TrimRight(url, "/")), ".git")
	stem = strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		}
		return '-'
	}, stem)
	return filepath.Join(g.MirrorsDir, stem+"-"+hex.EncodeToString(sum[:6])+".git")
}

// Diff summarises what a workspace changed relative to its base ref: the
// working tree (committed or not) against the merge base, plus untracked
// files, which `git diff` alone would miss.
type Diff struct {
	ChangedFiles int      `json:"changed_files"`
	Stat         string   `json:"stat"`
	Patch        string   `json:"patch"`
	Untracked    []string `json:"untracked,omitempty"`
	Truncated    bool     `json:"truncated"`
}

// MaxPatchBytes caps the patch returned by Diff; the stat is always complete.
const MaxPatchBytes = 512 * 1024

// Diff computes the workspace's change summary against baseRef.
func (g *Git) Diff(ctx context.Context, dir, baseRef string) (*Diff, error) {
	base, err := g.run(ctx, dir, nil, "merge-base", baseRef, "HEAD")
	if err != nil {
		// A base that is not fetched (shallow or foreign ref) still allows
		// a diff against the ref itself.
		base = baseRef
	}
	base = strings.TrimSpace(base)

	out := &Diff{}
	stat, err := g.run(ctx, dir, nil, "diff", "--stat", base)
	if err != nil {
		return nil, err
	}
	out.Stat = strings.TrimRight(stat, "\n")
	names, err := g.run(ctx, dir, nil, "diff", "--name-only", base)
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
	patch, err := g.run(ctx, dir, nil, "diff", base)
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

// Push pushes branch to origin, setting upstream. Credentials come from the
// host's git configuration (credential helper, ssh agent); a source-bound
// credential is a later step.
func (g *Git) Push(ctx context.Context, dir, branch string, log func(string)) error {
	_, err := g.run(ctx, dir, log, "push", "-u", "origin", branch)
	return err
}

// HasUncommitted reports whether the working tree has changes not yet in a
// commit, so a push can warn instead of silently pushing a stale branch.
func (g *Git) HasUncommitted(ctx context.Context, dir string) (bool, error) {
	out, err := g.run(ctx, dir, nil, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// Head returns the current commit of a workspace.
func (g *Git) Head(ctx context.Context, dir string) (string, error) {
	out, err := g.run(ctx, dir, nil, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
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
	logf(log, "$ git %s", strings.Join(args, " "))
	err := cmd.Run()
	if s := strings.TrimSpace(stderr.String()); s != "" && log != nil {
		for _, line := range strings.Split(s, "\n") {
			log(line)
		}
	}
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

func logf(log func(string), format string, args ...any) {
	if log != nil {
		log(fmt.Sprintf(format, args...))
	}
}
