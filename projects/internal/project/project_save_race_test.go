package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func newSaveFixture(t *testing.T) (*Service, *db.Store) {
	t.Helper()
	agent := &enginetest.Agent{Script: func(*enginetest.Turn) error { return nil }}
	store, err := db.Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	svc := NewService(store, Layout{Root: t.TempDir()}, Agents{Service: agent.Service()}, testHost{})
	t.Cleanup(func() { shutdownAndSettle(svc.Threads) })
	return svc, store
}

func newSavedProject(t *testing.T, svc *Service, store *db.Store) *db.Project {
	t.Helper()
	p := &db.Project{Name: "Demo", Instructions: "old", MaxRunningThreads: 4}
	main := &db.Thread{Origin: "user"}
	require.NoError(t, store.CreateProjectWithMain(p, main, svc.Layout.Root))
	return p
}

// The settings dialog saves the whole project row. The coordinator's
// update_instructions tool also saved the whole row, from the copy it read
// before the settings save. Interleaved (tool reads, settings saves, tool
// saves), the tool's older copy put max_running_threads back: a just-saved
// raise silently vanished from the row (the in-memory limit held until the
// next settings save or a restart read the row again).
func TestInterleavedInstructionsUpdateDoesNotRevertTheSavedLimit(t *testing.T) {
	svc, store := newSaveFixture(t)
	p := newSavedProject(t, svc, store)

	// The coordinator's tool turn reads the project before its call.
	toolCopy, err := store.GetProject(p.ID)
	require.NoError(t, err)

	// The settings dialog saves a raised limit from its own copy.
	settings, err := store.GetProject(p.ID)
	require.NoError(t, err)
	settings.MaxRunningThreads = 7
	require.NoError(t, svc.Save(settings))

	// The tool now applies its instructions to the row it read and saves it.
	_, err = svc.Coordinator.updateInstructions(context.Background(), toolCopy, struct{ Instructions string }{Instructions: "new"})
	require.NoError(t, err)

	got, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, got.MaxRunningThreads, "the settings save's limit survives the tool's write")
	assert.Equal(t, "new", got.Instructions)
}

// The same interleaving the other way round: the settings form loaded the row
// before the coordinator's instruction edit and saves every column back. The
// column the form did not edit must keep what the coordinator wrote.
func TestInterleavedSettingsSaveDoesNotRevertTheInstructionsUpdate(t *testing.T) {
	svc, store := newSaveFixture(t)
	p := newSavedProject(t, svc, store)

	// The settings form reads the project before the tool's edit.
	settings, err := store.GetProject(p.ID)
	require.NoError(t, err)

	toolCopy, err := store.GetProject(p.ID)
	require.NoError(t, err)
	_, err = svc.Coordinator.updateInstructions(context.Background(), toolCopy, struct{ Instructions string }{Instructions: "new"})
	require.NoError(t, err)

	// Saving the general tab writes name, models, limit… but not instructions.
	settings.MaxRunningThreads = 7
	require.NoError(t, svc.SaveSettings(settings, "Instructions"))

	got, err := store.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, 7, got.MaxRunningThreads)
	assert.Equal(t, "new", got.Instructions, "the form did not edit instructions, so the coordinator's edit stands")
}
