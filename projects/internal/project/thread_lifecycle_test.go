package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func systemNotes(t *testing.T, store *db.Store, threadID string) []string {
	t.Helper()
	all, err := store.ListThreadEvents(threadID)
	require.NoError(t, err)
	var out []string
	for _, e := range all {
		if e.Kind == db.EvSystem {
			out = append(out, e.Content)
		}
	}
	return out
}

func TestStopNamesWhoStoppedAndCountsDroppedMessages(t *testing.T) {
	for _, c := range []struct {
		by   Source
		want string
	}{
		{Source{}, "Stopped."},
		{Source{Kind: db.SourceIM}, "Stopped from a chat."},
		{fromThread("main"), "Stopped by the coordinator."},
	} {
		t.Run(c.want, func(t *testing.T) {
			f := newFixture(t, func(turn *enginetest.Turn) error {
				<-turn.Ctx.Done()
				return turn.Ctx.Err()
			})
			th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long", Repo: "none"})
			require.NoError(t, err)
			waitStatus(t, f.store, th.ID, db.ThreadWorking)
			_, err = f.threads.SendWith(context.Background(), th.ID, "later", nil, Source{}, SendQueue)
			require.NoError(t, err)

			res := f.threads.Stop(th.ID, c.by)
			assert.True(t, res.Interrupted)
			assert.Equal(t, 1, res.Dropped)
			waitStatus(t, f.store, th.ID, db.ThreadIdle)
			assert.Eventually(t, func() bool {
				for _, n := range systemNotes(t, f.store, th.ID) {
					if n == c.want {
						return true
					}
				}
				return false
			}, 5*time.Second, 10*time.Millisecond, "notes: %v", systemNotes(t, f.store, th.ID))

			again := f.threads.Stop(th.ID, c.by)
			assert.False(t, again.Interrupted, "stopping a thread with nothing running does nothing")
			assert.Zero(t, again.Dropped)
		})
	}
}

func TestReopenOnlyActsOnAResolvedThread(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		close(started)
		<-turn.Ctx.Done()
		return turn.Ctx.Err()
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long", Repo: "none"})
	require.NoError(t, err)
	<-started
	waitStatus(t, f.store, th.ID, db.ThreadWorking)

	got, err := f.threads.Reopen(th.ID, fromThread("main"))
	require.NoError(t, err)
	assert.Equal(t, db.ThreadWorking, got.Status, "a running thread is left alone")
	for _, n := range systemNotes(t, f.store, th.ID) {
		assert.NotContains(t, n, "Reopened")
	}

	f.threads.Stop(th.ID, Source{})
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)
	got, err = f.threads.Reopen(th.ID, fromThread("main"))
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, got.Status)
	assert.Nil(t, got.ResolvedAt)
	assert.Contains(t, systemNotes(t, f.store, th.ID), "Reopened by the coordinator.")

	again, err := f.threads.Reopen(th.ID, Source{})
	require.NoError(t, err)
	assert.Equal(t, db.ThreadIdle, again.Status)
}

func TestReopenRefusesAMainThread(t *testing.T) {
	f := newFixture(t, idleScript)
	_, err := f.threads.Reopen(f.project.MainThreadID, Source{})
	assert.ErrorIs(t, err, ErrNotWorker)
}
