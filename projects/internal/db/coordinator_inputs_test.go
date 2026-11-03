package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCoordinatorInputOrderingAndCrashRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	s, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "Project"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	e1 := &ThreadEvent{ThreadID: p.MainThreadID, Kind: EvUser, Content: "first"}
	e2 := &ThreadEvent{ThreadID: p.MainThreadID, Kind: EvUser, Content: "second"}
	require.NoError(t, s.AddCoordinatorInput(p.ID, e1))
	require.NoError(t, s.AddCoordinatorInput(p.ID, e2))
	first, err := s.ClaimCoordinatorInput(p.ID)
	require.NoError(t, err)
	require.Equal(t, e1.ID, first.EventID)
	_, err = s.ClaimCoordinatorInput(p.ID)
	require.Error(t, err)
	require.NoError(t, s.Close())
	s, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	require.NoError(t, s.RecoverCoordinatorInputs())
	_, err = s.ClaimCoordinatorInput(p.ID)
	require.Error(t, err, "an ambiguous turn must not be silently retried or overtaken")
	var saved CoordinatorInput
	require.NoError(t, s.db.First(&saved, "id = ?", first.ID).Error)
	require.Equal(t, CoordinatorUncertain, saved.State)
	require.Error(t, s.ResolveCoordinatorInput("wrong-project", first.ID, false))
	require.NoError(t, s.ResolveCoordinatorInput(p.ID, first.ID, false))
	second, err := s.ClaimCoordinatorInput(p.ID)
	require.NoError(t, err)
	require.Equal(t, e2.ID, second.EventID)
	require.NoError(t, s.FinishCoordinatorInput(second.ID, true))
	_, err = s.ClaimCoordinatorInput(p.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCoordinatorInputRejectsForeignEvent(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	p := &Project{Name: "First"}
	other := &Project{Name: "Second"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	require.NoError(t, s.CreateProjectWithMain(other, &Thread{}, ""))
	e := &ThreadEvent{ThreadID: other.MainThreadID, Kind: EvUser, Content: "wrong project"}
	require.Error(t, s.AddCoordinatorInput(p.ID, e))
	events, err := s.ListThreadEvents(other.MainThreadID)
	require.NoError(t, err)
	require.Empty(t, events)
}
