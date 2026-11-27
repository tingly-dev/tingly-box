package project

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude/session"
	"github.com/tingly-dev/tingly-box/agentboot/process"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/sandbox"
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
	return threadPolicy("", nil, nil, nil, sandbox.Resources{Level: sandbox.ResourcesUnlimited}).Levels()
}

// CheckThreadSandbox reports whether a project's threads can run in runtime
// here: the runtime is here, gives every level a thread asks for, and starts
// a process under the thread policy (so a missing tool for one level, such
// as pasta for the network, shows before a thread fails). A result is kept
// for a minute.
func CheckThreadSandbox(ctx context.Context, runtime string) error {
	if runtime == "" || runtime == sandbox.HostName {
		return nil
	}
	checkMu.Lock()
	defer checkMu.Unlock()
	if c, ok := checked[runtime]; ok && time.Since(c.at) < time.Minute {
		return c.err
	}
	err := probeThreadSandbox(ctx, runtime)
	checked[runtime] = checkResult{at: time.Now(), err: err}
	return err
}

type checkResult struct {
	at  time.Time
	err error
}

var (
	checkMu sync.Mutex
	checked = map[string]checkResult{}
)

func probeThreadSandbox(ctx context.Context, runtime string) error {
	rt, ok := sandbox.Get(runtime)
	if !ok {
		return fmt.Errorf("sandbox %q is not in this build", runtime)
	}
	if err := rt.Check(ctx); err != nil {
		return err
	}
	home, err := os.MkdirTemp("", "tb-sandbox-check-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(home)
	proc, err := sandbox.Start(ctx, runtime, sandbox.Cmd{Args: []string{"true"}, Env: []string{"PATH=/usr/bin:/bin"}}, threadPolicy(home, nil, []string{"PATH"}, nil, sandbox.Resources{Level: sandbox.ResourcesUnlimited}))
	if err != nil {
		return err
	}
	_ = proc.Stdin().Close()
	_, _ = io.Copy(io.Discard, proc.Stdout())
	return proc.Wait()
}

// failureText is why a turn's error ended the thread, in words: a sandbox
// that killed the process for running out of memory says so, with the limit.
func (m *Threads) failureText(t *db.Thread, err error) string {
	var exit *sandbox.ExitError
	if !errors.As(err, &exit) || exit.Reason != sandbox.EndOOM {
		return err.Error()
	}
	msg := "the sandbox ran out of memory"
	if p, perr := m.store.GetProject(t.ProjectID); perr == nil && p.ThreadMemoryBytes > 0 {
		msg += " (limit " + FormatBytes(p.ThreadMemoryBytes) + " per thread)"
	}
	return msg + "; raise the project's memory limit or unset it in the project settings"
}

// FormatBytes writes a byte count in the largest binary unit that keeps it whole.
func FormatBytes(n int64) string {
	for _, u := range []struct {
		size int64
		name string
	}{{1 << 30, "GiB"}, {1 << 20, "MiB"}} {
		if n >= u.size && n%u.size == 0 {
			return strconv.FormatInt(n/u.size, 10) + " " + u.name
		}
	}
	return strconv.FormatInt(n, 10) + " bytes"
}

// Quota minimums: Claude Code does not start in less memory than this.
const (
	MinThreadMemoryBytes int64 = 512 << 20
	MinThreadTasks             = 64
	MinThreadCPU               = 100
)

// threadResources are a project's per-thread limits: limited when any is set,
// else unlimited.
func threadResources(p *db.Project) sandbox.Resources {
	r := sandbox.Resources{MemoryBytes: p.ThreadMemoryBytes, Tasks: p.ThreadTasks, CPU: p.ThreadCPU}
	if r.MemoryBytes > 0 || r.Tasks > 0 || r.CPU > 0 {
		r.Level = sandbox.ResourcesLimited
	} else {
		r.Level = sandbox.ResourcesUnlimited
	}
	return r
}

// threadPolicy is what a sandboxed thread runs under (.design/sandbox.md §5).
// Storage: only the mounts (home is empty). Network: the public internet,
// and of the host only the ports its launch talks to (the model gateway,
// MCP servers). Exec: confined. Credentials: only the variables named.
// Resources: the project's limits, none by default.
func threadPolicy(home string, mounts []sandbox.Mount, env []string, expose []sandbox.Endpoint, res sandbox.Resources) sandbox.Policy {
	return sandbox.Policy{
		Storage:     sandbox.Storage{Level: sandbox.StorageConfined, Home: home, Mounts: mounts},
		Network:     sandbox.Network{Level: sandbox.NetworkInternet, Expose: expose},
		Exec:        sandbox.Exec{Level: sandbox.ExecConfined},
		Credentials: sandbox.Credentials{Level: sandbox.CredentialsExplicit, Env: env},
		Resources:   res,
	}
}

// imagePath is PATH inside a runtime's own system (Info.System): the guest
// image's standard directories.
const imagePath = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"

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
	rt, ok := sandbox.Get(p.ThreadSandbox)
	if !ok {
		return nil, fmt.Errorf("sandbox %q is not in this build", p.ThreadSandbox)
	}
	// A runtime with a system of its own (a Linux guest on a Mac) runs the
	// tools of its image: the host's executables do not run there.
	ownSystem := rt.Info().System != ""
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
		dir := filepath.Join(config, "projects", session.ProjectDirName(t.Cwd))
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
	if ownSystem {
		opts.Env = setEnv(opts.Env, "PATH", imagePath)
	} else {
		for _, dir := range toolchainDirs(home, envValue(opts.Env, "PATH")) {
			ro(dir)
		}
		if cli, err := exec.LookPath("claude"); err == nil {
			if real, err := filepath.EvalSymlinks(cli); err == nil && strings.HasPrefix(real, home+"/") {
				ro(filepath.Dir(real))
			}
		}
	}

	keep := slices.Clone(sandboxEnv)
	for _, kv := range opts.Env {
		if name, _, ok := strings.Cut(kv, "="); ok {
			keep = append(keep, name)
		}
	}
	return &process.Sandbox{Runtime: p.ThreadSandbox, Policy: threadPolicy(home, ms, keep, hostEndpoints(opts), threadResources(p))}, nil
}

// loopbackURL finds a URL on the host's loopback.
var loopbackURL = regexp.MustCompile(`https?://(?:127\.0\.0\.1|localhost|\[::1\]):(\d+)`)

// hostEndpoints are the ports of the host's loopback a launch talks to: the
// URLs on it in its environment (the model gateway), its settings file and
// its MCP configuration. These, and nothing else of the host, are exposed.
func hostEndpoints(opts *agentboot.ExecutionOptions) []sandbox.Endpoint {
	var text []string
	for _, kv := range opts.Env {
		if _, v, ok := strings.Cut(kv, "="); ok {
			text = append(text, v)
		}
	}
	for _, path := range append([]string{opts.SettingsPath, opts.MCPConfigPath}, opts.AdditionalMCPConfigPaths...) {
		switch {
		case path == "":
		case strings.HasPrefix(strings.TrimSpace(path), "{"):
			text = append(text, path) // inline JSON
		default:
			if b, err := os.ReadFile(path); err == nil {
				text = append(text, string(b))
			}
		}
	}
	var out []sandbox.Endpoint
	seen := map[int]bool{}
	for _, t := range text {
		for _, m := range loopbackURL.FindAllStringSubmatch(t, -1) {
			if port, err := strconv.Atoi(m[1]); err == nil && port > 0 && port < 65536 && !seen[port] {
				seen[port] = true
				out = append(out, sandbox.Endpoint{Port: port})
			}
		}
	}
	slices.SortFunc(out, func(a, b sandbox.Endpoint) int { return a.Port - b.Port })
	return out
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
