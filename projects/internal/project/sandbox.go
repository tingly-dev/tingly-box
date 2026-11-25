package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf16"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/sandbox"
	// The runtimes this build ships: a new one is one more import here.
	_ "github.com/tingly-dev/tingly-box/sandbox/bwrap"
)

// db.Project.ThreadSandbox names the sandbox runtime a project's threads run
// in (the sandbox registry); empty, or the host runtime, runs them on the host
// as before. See .design/thread-runtime.md.

// ValidSandbox reports whether s is a value a project can be set to: empty, or
// a runtime this build ships.
func ValidSandbox(s string) bool {
	_, ok := sandbox.Get(s)
	return s == "" || ok
}

// sandboxed reports whether a project's threads run isolated.
func sandboxed(p *db.Project) bool {
	return p.ThreadSandbox != "" && p.ThreadSandbox != sandbox.HostName
}

// ThreadPolicyLevels are the levels, axis by axis, a project's threads run at
// with runtime: what the settings show.
func ThreadPolicyLevels(runtime string) map[sandbox.Axis]sandbox.Level {
	if runtime == "" || runtime == sandbox.HostName {
		return sandbox.Policy{
			Storage: sandbox.Storage{Level: sandbox.StorageHost}, Network: sandbox.Network{Level: sandbox.NetworkOpen},
			Exec: sandbox.Exec{Level: sandbox.ExecHost}, Credentials: sandbox.Credentials{Level: sandbox.CredentialsInherit},
			Resources: sandbox.Resources{Level: sandbox.ResourcesUnlimited},
		}.Levels()
	}
	return threadPolicy("", nil, nil).Levels()
}

// threadPolicy is what a sandboxed thread runs under. Storage: only the mounts
// (home is empty). Network: open, for now (the user's decision; §8). Exec:
// confined. Credentials: only the variables named. Resources: no limits yet.
func threadPolicy(home string, mounts []sandbox.Mount, env []string) sandbox.Policy {
	return sandbox.Policy{
		Storage:     sandbox.Storage{Level: sandbox.StorageConfined, Home: home, Mounts: mounts},
		Network:     sandbox.Network{Level: sandbox.NetworkOpen},
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit, Env: env},
		Resources:   sandbox.Resources{Level: sandbox.ResourcesUnlimited},
	}
}

// sandboxEnv are the host variables a sandboxed thread keeps besides the ones
// its launch sets itself: what tools need to run, nothing that carries a secret.
var sandboxEnv = []string{"PATH", "LANG", "LANGUAGE", "LC_*", "TERM", "TZ", "USER", "LOGNAME", "SHELL",
	"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "ALL_PROXY", "http_proxy", "https_proxy", "no_proxy", "all_proxy",
	"CLAUDE_CODE_*", "CLAUDE_CONFIG_DIR", "DISABLE_*"}

// sandboxFor is the sandbox a thread's launch runs in, or nil when the project
// runs threads on the host. Its storage is what the thread works with: its
// directory and repository, the project's library and memory, its own Claude
// Code transcripts, and read-only what it runs with (settings, MCP
// configuration, the CLI, toolchains on PATH). A repository's config and hooks
// are read-only inside: the host runs git there too, and must not run what the
// thread wrote into them (§10.5).
func (m *Threads) sandboxFor(ctx context.Context, p *db.Project, t *db.Thread, opts *agentboot.ExecutionOptions) (*process.Sandbox, error) {
	if !sandboxed(p) {
		return nil, nil
	}
	if _, ok := sandbox.Get(p.ThreadSandbox); !ok {
		return nil, fmt.Errorf("sandbox %q is not in this build", p.ThreadSandbox)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("sandbox: %w", err)
	}
	var ms []sandbox.Mount
	rw := func(path string) { ms = append(ms, sandbox.Mount{Path: path, Writable: true}) }
	ro := func(path string) { ms = append(ms, sandbox.Mount{Path: path, Optional: true}) }

	if t.Cwd != "" {
		rw(t.Cwd)
		ms = append(ms, gitMounts(ctx, t.Cwd, true)...)
		// CLAUDE.md files above the directory, which Claude Code reads too.
		for dir := filepath.Dir(t.Cwd); dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
			ro(filepath.Join(dir, "CLAUDE.md"))
			ro(filepath.Join(dir, "CLAUDE.local.md"))
		}
	}
	for _, r := range p.Repos {
		if r.Path != "" && r.Path != t.RepoPath {
			ms = append(ms, sandbox.Mount{Path: r.Path, Optional: true})
			ms = append(ms, gitMounts(ctx, r.Path, false)...)
		}
	}
	owner := UploadOwner(p, t)
	if err := os.MkdirAll(m.layout.LibraryDir(owner), 0o755); err == nil {
		rw(m.layout.LibraryDir(owner))
	}
	if fi, err := os.Stat(m.layout.MemoryDir(p.ID)); err == nil && fi.IsDir() {
		rw(m.layout.MemoryDir(p.ID))
	}

	// What the launch reads: settings, MCP configuration, the CLI.
	for _, path := range append([]string{opts.SettingsPath, opts.MCPConfigPath}, opts.AdditionalMCPConfigPaths...) {
		if path != "" && !strings.HasPrefix(strings.TrimSpace(path), "{") {
			ro(path)
		}
	}
	// The transcripts of this directory only, where the host reads them and
	// --resume finds them; other sessions' stay out of reach.
	config := envValue(opts.Env, "CLAUDE_CONFIG_DIR")
	if config == "" {
		config = filepath.Join(home, ".claude")
	}
	if t.Cwd != "" {
		dir := filepath.Join(config, "projects", claudeProjectDir(t.Cwd))
		if err := os.MkdirAll(dir, 0o700); err == nil {
			rw(dir)
		}
	}
	// The user's own Claude Code instructions, skills, agents and commands, as
	// on the host; not settings.json (it may carry an env block with keys).
	for _, name := range []string{"CLAUDE.md", "skills", "agents", "commands"} {
		ro(filepath.Join(config, name))
	}
	if gc, err := m.sandboxGitConfig(ctx, p, t); err == nil {
		ro(gc)
		opts.Env = setEnv(opts.Env, "GIT_CONFIG_GLOBAL", gc)
	}
	for _, dir := range toolchainDirs(home, envValue(opts.Env, "PATH")) {
		ro(dir)
	}
	if cli, err := exec.LookPath("claude"); err == nil {
		if real, err := filepath.EvalSymlinks(cli); err == nil && strings.HasPrefix(real, home+"/") {
			ro(filepath.Dir(real))
		}
	}

	keep := slices.Clone(sandboxEnv)
	for _, kv := range opts.Env {
		if name, _, ok := strings.Cut(kv, "="); ok {
			keep = append(keep, name)
		}
	}
	return &process.Sandbox{Runtime: p.ThreadSandbox, Policy: threadPolicy(home, ms, keep)}, nil
}

// gitMounts are a repository's git directory, writable (commits, branches)
// when writable is set, with its config and hooks read-only on top, also a
// submodule's and a worktree's own. Nothing for a directory outside git.
func gitMounts(ctx context.Context, dir string, writable bool) []sandbox.Mount {
	common, err := git(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return nil
	}
	common = strings.TrimSpace(common)
	ms := []sandbox.Mount{{Path: common, Writable: writable, Optional: true}}
	guard := func(path string) { ms = append(ms, sandbox.Mount{Path: path, Optional: true}) }
	guard(filepath.Join(common, "config"))
	guard(filepath.Join(common, "hooks"))
	for _, pattern := range []string{"modules/*/config", "modules/*/hooks", "worktrees/*/config.worktree"} {
		matches, _ := filepath.Glob(filepath.Join(common, pattern))
		for _, p := range matches {
			guard(p)
		}
	}
	return ms
}

// sandboxGitConfig writes the global git config a sandboxed thread runs with:
// the user's name and email from the host's, nothing else (the host's may
// hold credential helpers or tokens).
func (m *Threads) sandboxGitConfig(ctx context.Context, p *db.Project, t *db.Thread) (string, error) {
	var b strings.Builder
	b.WriteString("[user]\n")
	for _, key := range []string{"name", "email"} {
		out, err := exec.CommandContext(ctx, "git", "config", "--global", "--get", "user."+key).Output()
		if v := strings.TrimSpace(string(out)); err == nil && v != "" && !strings.ContainsAny(v, "\n\"\\") {
			fmt.Fprintf(&b, "\t%s = %q\n", key, v)
		}
	}
	path := filepath.Join(m.layout.Dir(p.ID), "threads", t.ID+".gitconfig")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte(b.String()), 0o644)
}

// toolchainDirs are the PATH directories under home a sandboxed thread gets
// read-only, so the tools the user installed there run: a bin directory with
// its install prefix (an nvm Node version keeps npm in ../lib), except where the
// prefix holds more than tools (home itself, ~/.local, ~/.cargo, ~/.config).
func toolchainDirs(home, path string) []string {
	var out []string
	for _, dir := range filepath.SplitList(path) {
		dir = filepath.Clean(dir)
		if !strings.HasPrefix(dir, home+"/") {
			continue
		}
		if filepath.Base(dir) == "bin" {
			prefix := filepath.Dir(dir)
			switch prefix {
			case home, filepath.Join(home, ".local"), filepath.Join(home, ".cargo"), filepath.Join(home, ".config"):
			default:
				dir = prefix
			}
		}
		if !slices.Contains(out, dir) {
			out = append(out, dir)
		}
	}
	return out
}

var notAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

// claudeProjectDir is the directory name Claude Code keeps a working
// directory's transcripts under (~/.claude/projects/<name>): every character
// that is not a letter or digit becomes "-", and a name over 200 characters is
// cut there and given a hash of the path, as Claude Code does.
func claudeProjectDir(cwd string) string {
	name := notAlnum.ReplaceAllString(cwd, "-")
	if len(name) <= claudeProjectDirMax {
		return name
	}
	var h int32
	for _, c := range utf16.Encode([]rune(cwd)) {
		h = h*31 + int32(c)
	}
	n := int64(h)
	if n < 0 {
		n = -n
	}
	return name[:claudeProjectDirMax] + "-" + strconv.FormatInt(n, 36)
}

const claudeProjectDirMax = 200

// envValue is name's value in env, falling back to this process's own (a
// launch env only lists what it changes).
func envValue(env []string, name string) string {
	for i := len(env) - 1; i >= 0; i-- {
		if v, ok := strings.CutPrefix(env[i], name+"="); ok {
			return v
		}
	}
	return os.Getenv(name)
}
