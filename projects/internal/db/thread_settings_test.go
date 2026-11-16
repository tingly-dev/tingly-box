package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestModelOverrideMigrationDoesNotPinObservedModels(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "P", Type: ProjectProject, ThreadModel: "project-default"}
	require.NoError(t, store.CreateProject(p))
	th := &Thread{ProjectID: p.ID, Agent: AgentClaudeCode, Model: "observed-upstream", Status: ThreadIdle}
	require.NoError(t, store.CreateThread(th))
	require.NoError(t, store.Close())

	// Simulate the immediately preceding schema, already marked as migrated.
	g, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, g.Exec("ALTER TABLE threads DROP COLUMN model_override").Error)
	sql, err := g.DB()
	require.NoError(t, err)
	require.NoError(t, sql.Close())

	store, err = Open(path)
	require.NoError(t, err)
	got, err := store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, "observed-upstream", got.Model)
	assert.Empty(t, got.ModelOverride)
	got.ModelOverride = "requested-model"
	got.PermissionMode = "plan"
	require.NoError(t, store.SaveThread(got))
	require.NoError(t, store.Close())
	store, err = Open(path)
	require.NoError(t, err)
	defer store.Close()
	got, err = store.GetThread(th.ID)
	require.NoError(t, err)
	assert.Equal(t, "observed-upstream", got.Model)
	assert.Equal(t, "requested-model", got.ModelOverride)
	assert.Equal(t, "plan", got.PermissionMode)
}
