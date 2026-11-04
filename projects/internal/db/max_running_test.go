package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProjectsFromBeforeTheRunningLimitLoadAsTheDefaultAndSaveTheSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "Old", Type: ProjectProject}
	require.NoError(t, store.CreateProjectWithMain(p, &Thread{Origin: "user"}, ""))
	folder, err := store.OpenFolder(t.TempDir(), "")
	require.NoError(t, err)
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode, Status: ThreadWorking}
	require.NoError(t, store.CreateThread(th))
	require.NoError(t, store.Close())

	// The schema before the limit existed: neither column is there.
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, g.Exec("ALTER TABLE projects DROP COLUMN max_running_threads").Error)
	require.NoError(t, g.Exec("ALTER TABLE threads DROP COLUMN queued").Error)
	sqlDB, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	store, err = Open(path)
	require.NoError(t, err)
	got, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxRunningThreads, got.MaxRunningThreads, "a project from before the setting gets the default")
	gotFolder, err := store.GetProject(folder.ID)
	require.NoError(t, err)
	assert.Zero(t, gotFolder.MaxRunningThreads, "a folder does not use the setting")
	gotThread, err := store.GetThread(th.ID)
	require.NoError(t, err)
	assert.False(t, gotThread.Queued)

	for _, n := range []int{3, 8} {
		got.MaxRunningThreads = n
		require.NoError(t, store.SaveProject(got))
		reloaded, err := store.GetProject(p.ID)
		require.NoError(t, err)
		assert.Equal(t, n, reloaded.MaxRunningThreads)
	}
	require.NoError(t, store.Close())
}

func TestOpenRepairsAProjectWithoutAValidRunningLimit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "Zero", Type: ProjectProject}
	require.NoError(t, store.CreateProjectWithMain(p, &Thread{Origin: "user"}, ""))
	require.Zero(t, p.MaxRunningThreads, "the store itself sets no default")
	require.NoError(t, store.Close())

	store, err = Open(path)
	require.NoError(t, err)
	defer store.Close()
	got, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, DefaultMaxRunningThreads, got.MaxRunningThreads)
}
