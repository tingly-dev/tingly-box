package project

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// isolateGit keeps a test away from the real ~/.gitconfig (which can carry a
// credential helper, an init.defaultBranch or commit hooks).
func isolateGit(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
}

// emptyGitRepo is a repository with no commits yet: HEAD is unborn.
func emptyGitRepo(t *testing.T) string {
	t.Helper()
	isolateGit(t)
	dir := t.TempDir()
	run(t, dir, "git", "init", "-q", "-b", "main")
	return dir
}

// A fresh `git init` is a valid directory to add; it must not be refused for
// lacking commits, and its unborn branch still names the future base.
func TestInspectEmptyRepoReportsNoCommits(t *testing.T) {
	repo := emptyGitRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(repo, "README.md"), []byte("hi\n"), 0o644))

	info, err := InspectDirectory(context.Background(), repo)
	require.NoError(t, err)
	assert.Equal(t, db.RepoGit, info.Kind)
	assert.Equal(t, repo, info.Root)
	assert.True(t, info.Empty)
	assert.Equal(t, "main", info.Branch)
	assert.True(t, info.Dirty)
}

// Adding an empty repository with no explicit mode defaults to shared, the only
// mode that works without a commit to branch a worktree from.
func TestResolveEmptyRepoDefaultsToShared(t *testing.T) {
	repo := emptyGitRepo(t)
	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: repo}}, nil, false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, repo, repos[0].Path)
	assert.Equal(t, db.RepoGit, repos[0].Kind)
	assert.Equal(t, db.WorkspaceShared, repos[0].WorkspaceMode)
	assert.Equal(t, "main", repos[0].BaseBranch)
}

// Explicitly choosing worktree for an empty repository cannot work yet, so it
// is refused with a message that says why and what to do — not a raw git error.
func TestResolveEmptyRepoExplicitWorktreeExplains(t *testing.T) {
	repo := emptyGitRepo(t)
	_, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: repo, WorkspaceMode: db.WorkspaceWorktree}}, nil, false)
	require.Error(t, err)
	msg := err.Error()
	assert.Contains(t, msg, repo)
	assert.Contains(t, msg, "no commits")
	assert.Contains(t, msg, "initial commit")
	assert.Contains(t, msg, "shared mode")
	assert.NotContains(t, msg, "fatal:")
}

// A subdirectory inside an empty repository resolves to that repository, and
// is added the same way.
func TestResolveSubdirectoryOfEmptyRepo(t *testing.T) {
	repo := emptyGitRepo(t)
	sub := filepath.Join(repo, "pkg", "api")
	require.NoError(t, os.MkdirAll(sub, 0o755))

	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: sub}}, nil, false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, repo, repos[0].Path)
	assert.Equal(t, db.RepoGit, repos[0].Kind)
	assert.Equal(t, db.WorkspaceShared, repos[0].WorkspaceMode)
}

// A clone of an empty remote is the same situation as a fresh init.
func TestInspectCloneOfEmptyRemote(t *testing.T) {
	isolateGit(t)
	remote := t.TempDir()
	run(t, remote, "git", "init", "-q", "--bare", "-b", "main")
	local := filepath.Join(t.TempDir(), "clone")
	run(t, filepath.Dir(local), "git", "clone", "-q", remote, local)

	info, err := InspectDirectory(context.Background(), local)
	require.NoError(t, err)
	assert.Equal(t, db.RepoGit, info.Kind)
	assert.True(t, info.Empty)
	assert.Equal(t, "main", info.Branch)
}

// A repository with commits keeps the old behaviour: worktree by default.
func TestResolveNormalRepoStillDefaultsToWorktree(t *testing.T) {
	repo := gitRepoForDirectoryTest(t)
	info, err := InspectDirectory(context.Background(), repo)
	require.NoError(t, err)
	assert.False(t, info.Empty)

	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: repo}}, nil, false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, db.WorkspaceWorktree, repos[0].WorkspaceMode)
	assert.Equal(t, "main", repos[0].BaseBranch)
}

// A detached HEAD with commits is not empty, and keeps branching from HEAD.
func TestDetachedHeadRepoStillDefaultsToWorktree(t *testing.T) {
	repo := gitRepoForDirectoryTest(t)
	sha := run(t, repo, "git", "rev-parse", "HEAD")
	run(t, repo, "git", "-c", "advice.detachedHead=false", "checkout", "--detach", sha)

	info, err := InspectDirectory(context.Background(), repo)
	require.NoError(t, err)
	assert.False(t, info.Empty)
	assert.Equal(t, "HEAD", info.Branch)

	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: repo}}, nil, false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, db.WorkspaceWorktree, repos[0].WorkspaceMode)
	assert.Equal(t, "HEAD", repos[0].BaseBranch)
}

// Shared mode is fully usable on an empty repository: a thread starts in it.
func TestEmptyRepoWorksAsSharedThreadDir(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	repo := emptyGitRepo(t)
	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: repo}}, nil, false)
	require.NoError(t, err)
	require.NoError(t, SetProjectDirectories(f.project, repos, ""))

	thread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Empty repo", Task: "begin"})
	require.NoError(t, err)
	assert.Equal(t, db.CanonicalPath(repo), thread.Cwd)
	assert.Empty(t, thread.Branch)
}

// A worktree configuration that predates this fix (or a repository whose
// commits were removed) still explains itself instead of leaking a git error.
func TestStartWorktreeThreadInEmptyRepoExplains(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	repo := emptyGitRepo(t)
	f.project.Repos = []db.Repo{{Path: repo, BaseBranch: "main", Kind: db.RepoGit, WorkspaceMode: db.WorkspaceWorktree}}

	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Work", Task: "go", Repo: repo})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no commits")
	assert.Contains(t, err.Error(), "shared mode")
}
