package server

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDirectoryInspectHTTP(t *testing.T) {
	_, c := newServer(t, quietAgents())
	dir := t.TempDir()
	repo := gitRepo(t)
	for _, tc := range []struct {
		path, kind string
	}{
		{dir, "directory"},
		{repo, "git"},
	} {
		var got map[string]any
		require.Equal(t, http.StatusOK, c.do("POST", "/projects/directories/inspect", map[string]any{"path": tc.path}, &got))
		require.Equal(t, true, got["ok"])
		require.Equal(t, tc.path, got["root"])
		require.Equal(t, tc.kind, got["kind"])
		if tc.kind == "git" {
			require.Equal(t, "main", got["branch"])
			require.Equal(t, false, got["dirty"])
		}
	}
	require.NoError(t, os.WriteFile(filepath.Join(repo, "untracked"), []byte("change"), 0600))
	var dirty map[string]any
	require.Equal(t, http.StatusOK, c.do("POST", "/projects/directories/inspect", map[string]any{"path": repo}, &dirty))
	require.Equal(t, true, dirty["dirty"])
	var invalid map[string]any
	require.Equal(t, http.StatusOK, c.do("POST", "/projects/directories/inspect", map[string]any{"path": filepath.Join(dir, "missing")}, &invalid))
	require.Equal(t, false, invalid["ok"])
	require.NotEmpty(t, invalid["error"])
	require.Equal(t, http.StatusBadRequest, c.do("POST", "/projects/directories/inspect", map[string]any{}, nil))
	// The legacy endpoint still accepts only Git repositories.
	require.Equal(t, http.StatusOK, c.do("POST", "/projects/repos/inspect", map[string]any{"path": dir}, &invalid))
	require.Equal(t, false, invalid["ok"])
}

func TestPrimaryModeUpdatePreservesConcurrentDirectoryChanges(t *testing.T) {
	_, c := newServer(t, quietAgents())
	git1, git2, dir := gitRepo(t), gitRepo(t), t.TempDir()
	var p map[string]any
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "concurrent", "directories": []map[string]any{{"path": git1, "workspace_mode": "worktree"}, {"path": git2, "workspace_mode": "worktree"}}, "primary_repo_path": git1}, &p))
	id := p["id"].(string)
	// A different client changes another directory and adds a third.
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"directories": []map[string]any{{"path": git1, "workspace_mode": "worktree"}, {"path": git2, "workspace_mode": "shared"}, {"path": dir, "workspace_mode": "shared"}}, "primary_repo_path": git1}, &p))
	// The stale client only requests a change to its original primary.
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"primary_directory_mode": map[string]any{"path": git1, "workspace_mode": "shared"}}, &p))
	repos := p["repos"].([]any)
	require.Len(t, repos, 3)
	require.Equal(t, "shared", repos[0].(map[string]any)["workspace_mode"])
	require.Equal(t, "shared", repos[1].(map[string]any)["workspace_mode"])
	// A stale client must not silently change the wrong primary.
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"primary_repo_path": git2}, &p))
	require.Equal(t, http.StatusConflict, c.do("PATCH", "/projects/"+id, map[string]any{"primary_directory_mode": map[string]any{"path": git1, "workspace_mode": "worktree"}}, nil))
	require.Equal(t, http.StatusBadRequest, c.do("PATCH", "/projects/"+id, map[string]any{"primary_directory_mode": map[string]any{"path": git2, "workspace_mode": "bad"}}, nil))
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"primary_repo_path": dir}, &p))
	require.Equal(t, http.StatusBadRequest, c.do("PATCH", "/projects/"+id, map[string]any{"primary_directory_mode": map[string]any{"path": dir, "workspace_mode": "worktree"}}, nil))
}

func TestProjectDirectoriesHTTP(t *testing.T) {
	_, c := newServer(t, quietAgents())
	git1, git2, dir := gitRepo(t), gitRepo(t), t.TempDir()
	dirs := []map[string]any{{"path": git1, "workspace_mode": "shared"}, {"path": dir, "workspace_mode": "shared"}, {"path": git2, "workspace_mode": "worktree"}}
	var p map[string]any
	require.Equal(t, http.StatusCreated, c.do("POST", "/projects", map[string]any{"name": "directories", "directories": dirs, "primary_repo_path": git2}, &p))
	id := p["id"].(string)
	require.Equal(t, git2, p["primary_repo_path"])
	repos := p["repos"].([]any)
	require.Len(t, repos, 3)
	require.Equal(t, "shared", repos[0].(map[string]any)["workspace_mode"])
	require.Equal(t, "worktree", repos[2].(map[string]any)["workspace_mode"])

	// An omitted primary survives reordering; deleting it selects the first remaining directory.
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"directories": []map[string]any{dirs[2], dirs[0]}}, &p))
	require.Equal(t, git2, p["primary_repo_path"])
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"directories": []map[string]any{dirs[0]}}, &p))
	require.Equal(t, git1, p["primary_repo_path"])
	require.Equal(t, http.StatusBadRequest, c.do("PATCH", "/projects/"+id, map[string]any{"primary_repo_path": dir}, nil))

	for _, body := range []map[string]any{
		{"repos": []string{git1}, "directories": dirs},
		{"directories": dirs, "disable_worktree": true},
	} {
		require.Equal(t, http.StatusBadRequest, c.do("PATCH", "/projects/"+id, body, nil))
	}
	require.Equal(t, http.StatusBadRequest, c.do("POST", "/projects", map[string]any{"name": "conflict", "repos": []string{git1}, "directories": dirs}, nil))
	require.Equal(t, http.StatusBadRequest, c.do("POST", "/projects", map[string]any{"name": "conflict", "directories": dirs, "disable_worktree": false}, nil))

	// Legacy repos keeps an existing Git directory's explicit mode.
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"repos": []string{git1, git2}}, &p))
	repos = p["repos"].([]any)
	require.Equal(t, "shared", repos[0].(map[string]any)["workspace_mode"])
	require.Equal(t, http.StatusOK, c.do("PATCH", "/projects/"+id, map[string]any{"disable_worktree": false}, &p))
	for _, r := range p["repos"].([]any) {
		require.Equal(t, "worktree", r.(map[string]any)["workspace_mode"])
	}
}

func TestFolderRejectsDirectoryConfigurationHTTP(t *testing.T) {
	_, c := newServer(t, quietAgents())
	dir := t.TempDir()
	require.NoError(t, os.Mkdir(filepath.Join(dir, "child"), 0700))
	var folder map[string]any
	require.Equal(t, http.StatusOK, c.do("POST", "/projects", map[string]any{"type": "folder", "path": dir}, &folder))
	for _, body := range []map[string]any{
		{"directories": []any{}},
		{"primary_repo_path": dir},
	} {
		require.Equal(t, http.StatusBadRequest, c.do("PATCH", "/projects/"+folder["id"].(string), body, nil))
		body["type"], body["path"] = "folder", dir
		require.Equal(t, http.StatusBadRequest, c.do("POST", "/projects", body, nil))
	}
}
