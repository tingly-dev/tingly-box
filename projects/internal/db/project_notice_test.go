package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveProjectWithEventRollsBackWhenNoticeFails(t *testing.T) {
	s, err := Open(":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { s.Close() })
	p := &Project{Name: "Before"}
	require.NoError(t, s.CreateProjectWithMain(p, &Thread{}, ""))
	require.NoError(t, s.db.Exec(`CREATE TRIGGER reject_notice BEFORE INSERT ON thread_events
        BEGIN SELECT RAISE(ABORT, 'cannot persist notice'); END`).Error)
	p.Name = "After"
	err = s.SaveProjectWithEvent(p, &ThreadEvent{ThreadID: p.MainThreadID, Kind: EvSystem, Content: "changed"})
	require.ErrorContains(t, err, "cannot persist notice")
	saved, err := s.GetProject(p.ID)
	require.NoError(t, err)
	assert.Equal(t, "Before", saved.Name, "settings and the coordinator notice commit together")
	events, err := s.ListThreadEvents(p.MainThreadID)
	require.NoError(t, err)
	assert.Empty(t, events)
}
