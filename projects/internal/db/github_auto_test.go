package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// A project from before the GitHub settings keeps what thread_github_access
// gave it; a new project auto-approves nothing, a folder keeps publishing.
func TestGitHubAutoOfOldAndNewProjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	fresh := &Project{Name: "New"}
	require.NoError(t, store.CreateProjectWithMain(fresh, &Thread{Origin: "user"}, ""))
	folder, err := store.OpenFolder(t.TempDir(), "")
	require.NoError(t, err)
	var old []*Project
	for _, access := range []string{"", "allow", "ask"} {
		p := &Project{Name: "Old " + access}
		require.NoError(t, store.CreateProjectWithMain(p, &Thread{Origin: "user"}, ""))
		old = append(old, p)
	}
	require.NoError(t, store.Close())

	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	for i, access := range []string{"", "allow", "ask"} {
		require.NoError(t, g.Exec("UPDATE projects SET github_auto = NULL, thread_git_hub_access = ? WHERE id = ?", access, old[i].ID).Error)
	}
	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	store, err = Open(path)
	require.NoError(t, err)
	defer store.Close()
	get := func(id string) []string {
		p, err := store.GetProject(id)
		require.NoError(t, err)
		return p.GitHubAuto
	}
	assert.Equal(t, []string{}, get(fresh.ID), "a new project asks for every write")
	assert.Equal(t, []string{"publish"}, get(folder.ID), "a folder has no settings to turn it on")
	assert.Equal(t, []string{"publish"}, get(old[0].ID), "empty was allow")
	assert.Equal(t, []string{"publish"}, get(old[1].ID))
	assert.Equal(t, []string{}, get(old[2].ID), "ask asked for every write")
}
