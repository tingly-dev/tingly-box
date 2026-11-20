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

// Threads saved while they still followed their project's thread defaults get
// those defaults copied onto them once, when the database is opened; a field a
// thread already had is kept, and the move is not repeated.
func TestThreadSettingsMigrationPinsFollowingThreadsOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	store, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "P", Type: ProjectProject, ThreadProfile: "work", ThreadModel: "m-proj", ThreadEffort: "high", ThreadPermissionMode: "auto"}
	require.NoError(t, store.CreateProject(p))
	mk := func(th *Thread) *Thread {
		th.ProjectID, th.Agent, th.Status = p.ID, AgentClaudeCode, ThreadIdle
		require.NoError(t, store.CreateThread(th))
		return th
	}
	follower := mk(&Thread{})
	ownProfile := "mine"
	profiled := mk(&Thread{ProfileOverride: &ownProfile})
	modeled := mk(&Thread{ModelOverride: "m-own", PermissionMode: "plan"})
	main := mk(&Thread{})
	require.NoError(t, store.db.Exec("UPDATE threads SET agent = ? WHERE id = ?", AgentLLM, main.ID).Error)
	// The database as the previous version left it: following, and not yet moved.
	require.NoError(t, store.db.Exec("UPDATE threads SET profile_override = NULL, model_override = '', effort_override = '', permission_mode = '' WHERE id IN (?, ?)", follower.ID, main.ID).Error)
	require.NoError(t, store.db.Exec("UPDATE threads SET profile_override = NULL, effort_override = '' WHERE id = ?", modeled.ID).Error)
	require.NoError(t, store.db.Exec("UPDATE threads SET model_override = '', effort_override = '', permission_mode = '' WHERE id = ?", profiled.ID).Error)
	require.NoError(t, store.db.Exec("DELETE FROM settings WHERE key = ?", pinnedSettingsMarker).Error)
	require.NoError(t, store.Close())

	store, err = Open(path)
	require.NoError(t, err)
	get := func(id string) *Thread {
		th, err := store.GetThread(id)
		require.NoError(t, err)
		return th
	}
	got := get(follower.ID)
	require.NotNil(t, got.ProfileOverride)
	assert.Equal(t, "work", *got.ProfileOverride)
	assert.Equal(t, "m-proj", got.ModelOverride)
	assert.Equal(t, "high", got.EffortOverride)
	assert.Equal(t, "auto", got.PermissionMode)

	got = get(profiled.ID)
	require.NotNil(t, got.ProfileOverride)
	assert.Equal(t, "mine", *got.ProfileOverride, "its own profile is kept")
	assert.Empty(t, got.ModelOverride, "with its own profile, an empty model is that profile's default")
	assert.Equal(t, "high", got.EffortOverride)
	assert.Equal(t, "auto", got.PermissionMode)

	got = get(modeled.ID)
	require.NotNil(t, got.ProfileOverride)
	assert.Equal(t, "work", *got.ProfileOverride)
	assert.Equal(t, "m-own", got.ModelOverride, "its own model is kept")
	assert.Equal(t, "plan", got.PermissionMode)

	got = get(main.ID)
	assert.Nil(t, got.ProfileOverride, "the coordinator's thread is not a worker")
	assert.Empty(t, got.PermissionMode)

	// Once moved, a later change to a project's defaults and a thread reset to
	// the endpoint's default are left alone.
	follower = get(follower.ID)
	follower.EffortOverride = ""
	require.NoError(t, store.SaveThread(follower))
	require.NoError(t, store.Close())
	store, err = Open(path)
	require.NoError(t, err)
	defer store.Close()
	assert.Empty(t, get(follower.ID).EffortOverride)
}
