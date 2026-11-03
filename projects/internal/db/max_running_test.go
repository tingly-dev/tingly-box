package db

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestProjectsFromBeforeTheRunningLimitLoadAsNoLimitAndSaveTheSetting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "Old", Type: ProjectProject}
	require.NoError(t, store.CreateProjectWithMain(p, &Thread{Origin: "user"}, ""))
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
	assert.Zero(t, got.MaxRunningThreads, "no setting reads as 0, no limit")
	gotThread, err := store.GetThread(th.ID)
	require.NoError(t, err)
	assert.False(t, gotThread.Queued)

	for _, n := range []int{3, 0, 8} {
		got.MaxRunningThreads = n
		require.NoError(t, store.SaveProject(got))
		reloaded, err := store.GetProject(p.ID)
		require.NoError(t, err)
		assert.Equal(t, n, reloaded.MaxRunningThreads)
	}
	require.NoError(t, store.Close())
}

func TestProjectJSONWithoutTheRunningLimitDecodesAsNoLimit(t *testing.T) {
	var p Project
	require.NoError(t, json.Unmarshal([]byte(`{"id":"x","name":"Old","type":"project"}`), &p))
	assert.Zero(t, p.MaxRunningThreads)
	out, err := json.Marshal(Project{Name: "Default"})
	require.NoError(t, err)
	assert.NotContains(t, string(out), "max_running_threads", "no limit is not written out")
	out, err = json.Marshal(Project{MaxRunningThreads: 5})
	require.NoError(t, err)
	assert.Contains(t, string(out), `"max_running_threads":5`)
}
