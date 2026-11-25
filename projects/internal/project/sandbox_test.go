package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestClaudeProjectDirMatchesClaudeCode(t *testing.T) {
	// As Claude Code names ~/.claude/projects/<dir> (seen on this host).
	assert.Equal(t, "-home-x0079-tingly-box--claude-worktrees-find-why-threads-still-call-gh-p-294d6c",
		claudeProjectDir("/home/x0079/tingly-box/.claude/worktrees/find-why-threads-still-call-gh-p-294d6c"))
}

func TestToolchainDirsKeepToolsNotHomes(t *testing.T) {
	home := "/home/u"
	got := toolchainDirs(home, strings.Join([]string{
		"/usr/bin",
		"/home/u/.nvm/versions/node/v22/bin",
		"/home/u/.local/bin",
		"/home/u/.cargo/bin",
		"/home/u/bin",
		"/home/u/go/bin",
		"/home/u/.nvm/versions/node/v22/bin",
	}, ":"))
	assert.Equal(t, []string{
		"/home/u/.nvm/versions/node/v22", // npm lives in ../lib
		"/home/u/.local/bin",             // not ~/.local (keyrings, state)
		"/home/u/.cargo/bin",             // not ~/.cargo (credentials.toml)
		"/home/u/bin",                    // not home itself
		"/home/u/go",
	}, got)
}

func mountOf(s *process.Sandbox, path string) (process.Mount, bool) {
	for _, m := range s.Mounts {
		if m.Path == path {
			return m, true
		}
	}
	return process.Mount{}, false
}

func TestSandboxedThreadGetsItsWorkAndNothingElseWritable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Fix login", Task: "fix"})
	require.NoError(t, err)
	f.report(t)

	_, opts, err := f.threads.turnLaunch(context.Background(), th.ID, th, f.project, false)
	require.NoError(t, err)
	assert.Nil(t, opts.Sandbox, "a project runs threads on the host unless it says otherwise")

	f.project.ThreadSandbox = SandboxBwrap
	require.NoError(t, f.store.SaveProject(f.project))
	_, opts, err = f.threads.turnLaunch(context.Background(), th.ID, th, f.project, false)
	require.NoError(t, err)
	s := opts.Sandbox
	require.NotNil(t, s)
	assert.Equal(t, home, s.Home)

	m, ok := mountOf(s, th.Cwd)
	require.True(t, ok, "the thread's directory")
	assert.False(t, m.ReadOnly)
	common := filepath.Join(f.project.Repos[0].Path, ".git")
	m, ok = mountOf(s, common)
	require.True(t, ok, "the repository's git directory, for commits")
	assert.False(t, m.ReadOnly)
	for _, guarded := range []string{"config", "hooks"} {
		m, ok = mountOf(s, filepath.Join(common, guarded))
		require.True(t, ok, guarded)
		assert.True(t, m.ReadOnly, "the host runs git there too: %s must not be the thread's to write", guarded)
	}
	m, ok = mountOf(s, f.threads.layout.LibraryDir(f.project.ID))
	require.True(t, ok)
	assert.False(t, m.ReadOnly)
	m, ok = mountOf(s, opts.SettingsPath)
	require.True(t, ok)
	assert.True(t, m.ReadOnly, "a thread must not rewrite its own permission rules")
	transcripts := filepath.Join(home, ".claude", "projects", claudeProjectDir(th.Cwd))
	m, ok = mountOf(s, transcripts)
	require.True(t, ok, "its own transcripts, where the host reads them")
	assert.False(t, m.ReadOnly)
	assert.DirExists(t, transcripts)
	_, ok = mountOf(s, filepath.Join(home, ".claude"))
	assert.False(t, ok, "not every session's transcripts")
	_, ok = mountOf(s, filepath.Join(home, ".claude", "settings.json"))
	assert.False(t, ok, "the user's settings may carry keys")

	for _, mt := range s.Mounts {
		if !mt.ReadOnly {
			assert.NotEqual(t, home, mt.Path, "home is never writable as a whole")
		}
	}
	gc := envValue(opts.Env, "GIT_CONFIG_GLOBAL")
	require.NotEmpty(t, gc, "commits need a name, not the host's whole git config")
	m, ok = mountOf(s, gc)
	require.True(t, ok)
	assert.True(t, m.ReadOnly)
	assert.Contains(t, s.KeepEnv, "TMPDIR", "what the launch sets is kept")
	assert.Contains(t, s.KeepEnv, "GIT_CONFIG_GLOBAL")
	assert.NotContains(t, s.KeepEnv, "GH_TOKEN")
	assert.NotContains(t, s.KeepEnv, "SSH_AUTH_SOCK")
}

func TestHostGitDoesNotRunAThreadsFsmonitor(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	repo := newRepo(t)
	marker := filepath.Join(t.TempDir(), "ran")
	_, err := git(context.Background(), repo, "config", "core.fsmonitor", "touch "+marker+"; false")
	require.NoError(t, err)
	_, _ = git(context.Background(), repo, "status", "--porcelain")
	_, err = os.Stat(marker)
	assert.True(t, os.IsNotExist(err), "the host ran a command a sandboxed thread could have planted")
}
