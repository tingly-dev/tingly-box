package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// A project's thread defaults are copied onto a thread when it starts; later
// changes to them reach only threads started afterwards.
func TestProjectThreadDefaultsReachOnlyLaterThreads(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	f.project.ThreadModel, f.project.ThreadEffort, f.project.ThreadPermissionMode = "model-a", "low", "acceptEdits"
	require.NoError(t, f.store.SaveProject(f.project))
	a, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "a"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, a.ID, db.ThreadIdle)

	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	p.ThreadModel, p.ThreadEffort, p.ThreadPermissionMode = "model-b", "high", "auto"
	require.NoError(t, f.store.SaveProject(p))

	_, err = f.threads.Send(context.Background(), a.ID, "again", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, a.ID, db.ThreadIdle)
	b, err := f.threads.Start(context.Background(), p, StartSpec{Task: "b"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, b.ID, db.ThreadIdle)

	launches := f.agent.Launches()
	require.Len(t, launches, 3)
	for _, l := range launches[:2] {
		assert.Equal(t, "model-a", l.Model)
		assert.Equal(t, "low", l.Effort)
		assert.Equal(t, "acceptEdits", l.PermissionMode)
	}
	assert.Equal(t, "model-b", launches[2].Model)
	assert.Equal(t, "high", launches[2].Effort)
	assert.Equal(t, "auto", launches[2].PermissionMode)

	// Handoff of the older thread reads its own model and effort too.
	cmd, err := f.threads.Handoff(a.ID)
	require.NoError(t, err)
	assert.Contains(t, cmd, "model-a")
	assert.Contains(t, cmd, "low")
	assert.NotContains(t, cmd, "model-b")
}
