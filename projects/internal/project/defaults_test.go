package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Projects starts every new thread in auto.
func TestNewThreadsStartInAuto(t *testing.T) {
	require.Equal(t, "auto", DefaultPermissionMode)

	t.Run("project thread", func(t *testing.T) {
		f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
		th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "go"})
		require.NoError(t, err)
		require.Equal(t, "auto", th.PermissionMode)
		f.report(t)
		require.Equal(t, "auto", f.agent.Launches()[0].PermissionMode)
	})

	t.Run("project setting still wins", func(t *testing.T) {
		f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
		f.project.ThreadPermissionMode = "default"
		th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "go"})
		require.NoError(t, err)
		require.Equal(t, "default", th.PermissionMode)
		f.report(t)
	})

	t.Run("folder session", func(t *testing.T) {
		f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
		th, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go"})
		require.NoError(t, err)
		require.Equal(t, "auto", th.PermissionMode)
	})

	t.Run("folder session with an explicit mode", func(t *testing.T) {
		f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
		th, err := f.startFolder(t.TempDir(), FolderThreadSpec{Task: "go", PermissionMode: "plan"})
		require.NoError(t, err)
		require.Equal(t, "plan", th.PermissionMode)
	})

	t.Run("new project", func(t *testing.T) {
		store, err := db.Open(":memory:")
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		svc := NewService(store, Layout{Root: t.TempDir()}, Agents{}, testHost{})
		t.Cleanup(func() { svc.Stop(context.Background()) })
		p, err := svc.Create(context.Background(), NewProjectSpec{Name: "P"})
		require.NoError(t, err)
		require.Equal(t, "auto", p.ThreadPermissionMode)
	})
}

func TestForkStartsInAuto(t *testing.T) {
	svc, store, layout := forkFixture(t)
	p := mkProject(t, store, &db.Project{Name: "Origin"})
	require.NoError(t, layout.Ensure(p.ID))
	fork, err := svc.Fork(p.ID, "")
	require.NoError(t, err)
	require.Equal(t, "auto", fork.ThreadPermissionMode)
}
