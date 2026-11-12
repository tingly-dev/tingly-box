package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRotateRollsBackWhenBoundaryInsertFails(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	p := &Project{Name: "Project"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	th, err := s.GetThread(p.MainThreadID)
	require.NoError(t, err)
	th.SessionID, th.Turns = "old", 2
	require.NoError(t, s.SaveThread(th))
	require.NoError(t, s.db.Exec(`CREATE TRIGGER fail_boundary BEFORE INSERT ON thread_events WHEN NEW.kind = 'command' BEGIN SELECT RAISE(ABORT, 'cannot save boundary'); END`).Error)
	require.Error(t, s.RotateThreadSession(th.ID, "old", "new", &ThreadEvent{}))
	got, err := s.GetThread(th.ID)
	require.NoError(t, err)
	require.Equal(t, "old", got.SessionID)
	require.Equal(t, 2, got.Turns)
}

func TestRotateThreadSessionAtomicAndPersistent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projects.db")
	s, err := Open(path)
	require.NoError(t, err)
	p := &Project{Name: "Project"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	th, err := s.GetThread(p.MainThreadID)
	require.NoError(t, err)
	th.SessionID, th.Cwd, th.Turns, th.InputTokens, th.ContextTokens = "old", "/workspace", 3, 42, 12
	require.NoError(t, s.SaveThread(th))
	before := &ThreadEvent{ThreadID: th.ID, Kind: EvUser, Content: "old history"}
	require.NoError(t, s.AddThreadEvent(before))
	require.NoError(t, s.RotateThreadSession(th.ID, "old", "new", &ThreadEvent{}))
	require.Error(t, s.RotateThreadSession(th.ID, "old", "wrong", &ThreadEvent{}))
	require.NoError(t, s.Close())
	s, err = Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	got, err := s.GetThread(th.ID)
	require.NoError(t, err)
	require.Equal(t, "new", got.SessionID)
	require.Equal(t, "/workspace", got.Cwd)
	require.EqualValues(t, 42, got.InputTokens)
	require.Zero(t, got.Turns)
	require.Zero(t, got.ContextTokens)
	ok, err := s.HasCoordinatorSessionBoundary(th.ID, "new")
	require.NoError(t, err)
	require.True(t, ok)
	events, err := s.ListThreadEvents(th.ID)
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "old history", events[0].Content)
	require.Equal(t, "clear", events[1].Content)
	require.JSONEq(t, `{"name":"clear","state":"ok","previous_session_id":"old","session_id":"new"}`, events[1].Payload)
}

func TestLegacyNewBoundaryStillPreventsHistoryMigration(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	p := &Project{Name: "Project"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	require.NoError(t, s.AddThreadEvent(&ThreadEvent{ThreadID: p.MainThreadID, Kind: EvCommand, Content: "new", Payload: `{"name":"new","session_id":"legacy"}`}))
	ok, err := s.HasCoordinatorSessionBoundary(p.MainThreadID, "legacy")
	require.NoError(t, err)
	require.True(t, ok)
}
