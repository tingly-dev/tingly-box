package project

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func chips(t *testing.T, f *testFixture, id, state string) []db.ThreadEvent {
	t.Helper()
	events, err := f.store.ListThreadEvents(id)
	require.NoError(t, err)
	var out []db.ThreadEvent
	for _, e := range events {
		if e.Kind == db.EvQueued && strings.Contains(e.Payload, `"state":"`+state+`"`) {
			out = append(out, e)
		}
	}
	return out
}

func userTexts(t *testing.T, f *testFixture, id string) []string {
	t.Helper()
	events, err := f.store.ListThreadEvents(id)
	require.NoError(t, err)
	var out []string
	for _, e := range events {
		if e.Kind == db.EvUser {
			out = append(out, e.Content)
		}
	}
	return out
}

// A turn that cannot take a message mid-turn (here a one-shot turn) holds
// the default send as a chip; the message enters the transcript only when
// its turn comes.
func TestSendDuringATurnIsHeldWhenItCannotJoin(t *testing.T) {
	release := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			<-release
		}
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWorking)

	_, err = f.threads.SendWith(context.Background(), th.ID, "second", nil, Source{}, SendNow)
	require.NoError(t, err)
	held := chips(t, f, th.ID, "held")
	require.Len(t, held, 1)
	assert.Equal(t, "second", held[0].Content)
	assert.NotContains(t, userTexts(t, f, th.ID), "second", "a held message is not part of the conversation yet")

	close(release)
	assert.Equal(t, "did first", f.report(t).Text)
	assert.Equal(t, "did second", f.report(t).Text)
	assert.Empty(t, chips(t, f, th.ID, "held"))
	assert.Len(t, chips(t, f, th.ID, "sent"), 1)
	assert.Contains(t, userTexts(t, f, th.ID), "second")
}

func TestQueuedMessageCanBeTakenBack(t *testing.T) {
	release := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			<-release
		}
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	waitStatus(t, f.store, th.ID, db.ThreadWorking)

	_, err = f.threads.SendWith(context.Background(), th.ID, "never mind", nil, Source{}, SendQueue)
	require.NoError(t, err)
	held := chips(t, f, th.ID, "held")
	require.Len(t, held, 1)

	assert.True(t, f.threads.RemoveQueued(th.ID, held[0].ID))
	assert.False(t, f.threads.RemoveQueued(th.ID, held[0].ID), "a second removal finds nothing")
	assert.Len(t, chips(t, f, th.ID, "removed"), 1)

	close(release)
	assert.Equal(t, "did first", f.report(t).Text)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)
	assert.NotContains(t, userTexts(t, f, th.ID), "never mind")
	assert.Len(t, f.agent.Launches(), 1, "the removed message must not start a turn")
}

func TestSendWhileIdleIsAPlainSend(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	assert.Equal(t, "did first", f.report(t).Text)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)

	for _, mode := range []SendMode{SendQueue, SendStopAndSend, SendNow} {
		_, err = f.threads.SendWith(context.Background(), th.ID, "again "+string(mode), nil, Source{}, mode)
		require.NoError(t, err)
		assert.Equal(t, "did again "+string(mode), f.report(t).Text)
		assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)
	}
	assert.Empty(t, chips(t, f, th.ID, "held"), "nothing was running, so nothing is held")
}

// Stop and send replaces the running turn, and Stop alone discards what was
// held (the page hands that text back to the composer).
func TestStopAndSendReplacesTheTurnAndStopDropsChips(t *testing.T) {
	started := make(chan struct{})
	f := newFixture(t, func(turn *enginetest.Turn) error {
		if !turn.Opts.Resume {
			close(started)
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("did "+turn.Prompt, 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "long"})
	require.NoError(t, err)
	<-started

	_, err = f.threads.SendWith(context.Background(), th.ID, "instead", nil, Source{}, SendStopAndSend)
	require.NoError(t, err)
	assert.Equal(t, "did instead", f.report(t).Text)
	assert.Eventually(t, func() bool { return !f.threads.Running(th.ID) }, 5*time.Second, 10*time.Millisecond)

	t.Run("stop drops chips", func(t *testing.T) {
		release := make(chan struct{})
		g := newFixture(t, func(turn *enginetest.Turn) error {
			<-release
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		})
		th, err := g.threads.Start(context.Background(), g.project, StartSpec{Task: "long"})
		require.NoError(t, err)
		waitStatus(t, g.store, th.ID, db.ThreadWorking)
		_, err = g.threads.SendWith(context.Background(), th.ID, "later", nil, Source{}, SendQueue)
		require.NoError(t, err)
		require.Len(t, chips(t, g, th.ID, "held"), 1)
		g.threads.Stop(th.ID)
		close(release)
		assert.Eventually(t, func() bool { return len(chips(t, g, th.ID, "removed")) == 1 }, 5*time.Second, 10*time.Millisecond)
	})
}

func TestUnknownSendModeIsInvalid(t *testing.T) {
	assert.False(t, SendMode("later").Valid())
	for _, m := range []SendMode{"", SendNow, SendQueue, SendStopAndSend} {
		assert.True(t, m.Valid())
	}
}
