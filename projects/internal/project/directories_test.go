package project

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestLegacyRepoPathsAcceptRelativeAndEmpty(t *testing.T) {
	repo := gitRepoForDirectoryTest(t)
	rel, err := filepath.Rel(mustGetwd(t), repo)
	require.NoError(t, err)
	repos, err := ResolveRepos(context.Background(), []string{"", "  ", rel})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Equal(t, repo, repos[0].Path)
}

func mustGetwd(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	return dir
}
func gitRepoForDirectoryTest(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	run(t, dir, "git", "init", "-b", "main")
	run(t, dir, "git", "-c", "user.name=Test", "-c", "user.email=test@example.invalid", "commit", "--allow-empty", "-m", "initial")
	return dir
}

func TestInspectDirectoryWithLocalizedGitError(t *testing.T) {
	dir := t.TempDir()
	gitPath, err := exec.LookPath("git")
	require.NoError(t, err)
	bin := t.TempDir()
	script := "#!/bin/sh\nif [ \"$3\" = rev-parse ] && [ \"$4\" = --show-toplevel ]; then if [ \"$LC_ALL\" = C ]; then printf 'fatal: not a git repository\\n' >&2; else printf 'fatal: kein Git-Repository\\n' >&2; fi; exit 128; fi\nexec \"" + gitPath + "\" \"$@\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(bin, "git"), []byte(script), 0755))
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("LC_ALL", "de_DE.UTF-8")
	info, err := InspectDirectory(context.Background(), dir)
	require.NoError(t, err)
	assert.Equal(t, db.RepoDirectory, info.Kind)
}

func TestResolveReposAcceptsNonGitDirectory(t *testing.T) {
	dir := t.TempDir()
	repos, err := ResolveRepos(context.Background(), []string{dir})
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Equal(t, dir, repos[0].Path)
}

func TestDirectoryModesAndPrimaryThread(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	dir := t.TempDir()
	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{
		{Path: f.repo, WorkspaceMode: db.WorkspaceWorktree},
		{Path: dir, WorkspaceMode: db.WorkspaceShared},
	}, nil, false)
	require.NoError(t, err)
	require.NoError(t, SetProjectDirectories(f.project, repos, dir))
	thread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Shared", Task: "work"})
	require.NoError(t, err)
	assert.Equal(t, dir, thread.Cwd)
	assert.Empty(t, thread.Branch)
	assert.Contains(t, threadSystemPrompt(f.threads.layout, f.project, thread), "shared directory")
	gitThread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Git", Task: "work", Repo: f.repo})
	require.NoError(t, err)
	assert.NotEmpty(t, gitThread.Branch)
	assert.NotEqual(t, f.repo, gitThread.Cwd)
	assert.Contains(t, coordinatorCurrentSettings(f.project), dir+" (directory, shared, primary/default)")
	_, err = ResolveDirectories(context.Background(), []DirectorySpec{{Path: dir, WorkspaceMode: db.WorkspaceWorktree}}, nil, false)
	require.Error(t, err)
	require.Error(t, SetProjectDirectories(f.project, repos, filepath.Join(dir, "missing")))
}

func TestLegacyDirectoryDefaultsAndJSON(t *testing.T) {
	for _, disabled := range []bool{false, true} {
		p := db.Project{DisableWorktree: disabled, Repos: []db.Repo{{Path: "/old", BaseBranch: "main"}}}
		mode := db.WorkspaceWorktree
		if disabled {
			mode = db.WorkspaceShared
		}
		assert.Equal(t, mode, p.EffectiveRepo(p.Repos[0]).WorkspaceMode)
		p.ApplyDirectoryDefaults()
		data, err := json.Marshal(p)
		require.NoError(t, err)
		assert.Contains(t, string(data), `"primary_repo_path":"/old"`)
		assert.Contains(t, string(data), `"workspace_mode":"`+mode+`"`)
	}
}

func TestDirectoryOverlapAndExistingBranchAreSafe(t *testing.T) {
	root := t.TempDir()
	workspace := filepath.Join(root, "data")
	require.NoError(t, os.MkdirAll(workspace, 0o755))
	svc := Service{Layout: Layout{Root: workspace}}
	require.Error(t, svc.validateDirectoryOverlap(&db.Project{Repos: []db.Repo{{Path: root}}}))
	require.Error(t, svc.validateDirectoryOverlap(&db.Project{Repos: []db.Repo{{Path: workspace}}}))
	require.NoError(t, svc.validateDirectoryOverlap(&db.Project{Repos: []db.Repo{{Path: t.TempDir()}}}))

	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	run(t, f.repo, "git", "branch", "tp/existing")
	err := AddWorktree(context.Background(), f.repo, filepath.Join(t.TempDir(), "worktree"), "tp/existing", "main")
	require.Error(t, err)
	assert.Contains(t, run(t, f.repo, "git", "branch", "--list", "tp/existing"), "tp/existing")
}

func TestDirectoryInspectionAliasesAndInvalidGit(t *testing.T) {
	dir := t.TempDir()
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(dir, alias))
	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: dir}, {Path: alias}}, nil, false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, db.WorkspaceShared, repos[0].WorkspaceMode)
	_, err = ResolveDirectories(context.Background(), []DirectorySpec{{Path: dir}, {Path: alias, WorkspaceMode: db.WorkspaceWorktree}}, nil, false)
	require.Error(t, err)
	emptyGit := t.TempDir()
	run(t, emptyGit, "git", "init")
	empty, err := InspectDirectory(context.Background(), emptyGit)
	require.NoError(t, err)
	assert.True(t, empty.Empty, "a repository with no commits is still a usable directory")
	_, err = InspectDirectory(context.Background(), filepath.Join(dir, "missing"))
	require.Error(t, err)
}

// configureDirectory adds one directory to f's project in the given mode and
// returns the path the project stores for it.
func configureDirectory(t *testing.T, f *testFixture, path, mode string) string {
	t.Helper()
	repos, err := ResolveDirectories(context.Background(), []DirectorySpec{{Path: path, WorkspaceMode: mode}}, nil, false)
	require.NoError(t, err)
	require.NoError(t, SetProjectDirectories(f.project, repos, repos[0].Path))
	return repos[0].Path
}

// gitRepoWithCommit makes dir a repository with one commit.
func gitRepoWithCommit(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init", "-q", "-b", "main")
	run(t, dir, "git", "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
}

// A shared thread runs in its directory, so the directory's Git state is not
// part of its identity: a repository created or removed, or an outer one
// created around it, must not refuse the thread.
func TestSharedThreadStartsWhateverTheGitState(t *testing.T) {
	cases := []struct {
		name   string
		dir    func(t *testing.T) string
		change func(t *testing.T, dir string)
	}{
		{"plain directory becomes a repository", func(t *testing.T) string { return t.TempDir() }, gitRepoWithCommit},
		{"repository loses its .git", newRepo, func(t *testing.T, dir string) {
			require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git")))
		}},
		{"outer repository created around it", func(t *testing.T) string {
			dir := filepath.Join(t.TempDir(), "notes")
			require.NoError(t, os.MkdirAll(dir, 0o755))
			return dir
		}, func(t *testing.T, dir string) { gitRepoWithCommit(t, filepath.Dir(dir)) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, func(*enginetest.Turn) error { return nil })
			dir := tc.dir(t)
			stored := configureDirectory(t, f, dir, db.WorkspaceShared)
			tc.change(t, dir)
			thread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Shared", Task: "work"})
			require.NoError(t, err)
			assert.Equal(t, stored, thread.Cwd)
			assert.Empty(t, thread.Branch)
		})
	}
}

// Two spellings of one directory are one directory: symlinks, a trailing slash
// and ".." reach the same workspace, also when an old project row stored them.
func TestSharedThreadAcceptsEverySpellingOfItsDirectory(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	repo := newRepo(t)
	alias := filepath.Join(t.TempDir(), "alias")
	require.NoError(t, os.Symlink(repo, alias))
	configureDirectory(t, f, alias, db.WorkspaceShared)
	thread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Symlink", Task: "work"})
	require.NoError(t, err)
	assert.Equal(t, repo, thread.Cwd, "the stored path is the resolved directory")

	for _, stored := range []string{repo + string(filepath.Separator), filepath.Join(repo, "..", filepath.Base(repo))} {
		f := newFixture(t, func(*enginetest.Turn) error { return nil })
		f.project.Repos = []db.Repo{{Path: stored, Kind: db.RepoGit, WorkspaceMode: db.WorkspaceShared}}
		f.project.PrimaryRepoPath = stored
		thread, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Spelling", Task: "work"})
		require.NoError(t, err, "stored path %q", stored)
		assert.Equal(t, repo, thread.Cwd, "a thread starts in the resolved directory, whichever spelling was stored")
	}
}

// A worktree thread needs its configured directory to still be the root of a
// Git repository, and says which setting to change when it is not.
func TestWorktreeThreadRequiresItsGitRepository(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	configureDirectory(t, f, f.repo, db.WorkspaceWorktree)
	require.NoError(t, os.RemoveAll(filepath.Join(f.repo, ".git")))
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "W", Task: "work"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch it to shared mode")

	// The same directory, still inside a Git repository, but its own repository
	// is gone and an outer one covers it: a thread there would get a worktree of
	// a different workspace than the directory the user selected.
	parent := t.TempDir()
	dir := filepath.Join(parent, "repo")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	gitRepoWithCommit(t, dir)
	f = newFixture(t, func(*enginetest.Turn) error { return nil })
	configureDirectory(t, f, dir, db.WorkspaceWorktree)
	require.NoError(t, os.RemoveAll(filepath.Join(dir, ".git")))
	gitRepoWithCommit(t, parent)
	_, err = f.threads.Start(context.Background(), f.project, StartSpec{Title: "W", Task: "work"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "switch it to shared mode")
}

// A directory that is gone is a real change: the thread is refused with a way
// to fix the project rather than a message about the directory's contents.
func TestThreadRefusesDirectoryThatIsGone(t *testing.T) {
	f := newFixture(t, func(*enginetest.Turn) error { return nil })
	dir := t.TempDir()
	configureDirectory(t, f, dir, db.WorkspaceShared)
	require.NoError(t, os.RemoveAll(dir))
	_, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Gone", Task: "work"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "select it again in the project's settings")
}
