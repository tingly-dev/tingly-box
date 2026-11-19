package project

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func settled(f *testFixture, svc *Service, turns int, seen *prompts) func() bool {
	return func() bool { return len(seen.all()) == turns && !svc.Coordinator.Busy(f.project.ID) }
}

func TestCoordinatorClearStartsFreshSessionAndKeepsTheConversation(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { seen.add(c); c.Reply("answer"); return nil })
	_, err := svc.Coordinator.Post(f.project.ID, "old secret", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, settled(f, svc, 1, &seen), 5*time.Second, 10*time.Millisecond)
	before, err := f.store.GetThread(f.project.MainThreadID)
	require.NoError(t, err)
	require.NotZero(t, before.Turns)
	eventsBefore, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)

	got, err := svc.SendWith(context.Background(), before.ID, "/clear", nil, Source{}, SendNow)
	require.NoError(t, err)
	require.NotEqual(t, before.SessionID, got.SessionID)
	require.Zero(t, got.Turns)
	require.Equal(t, before.Cwd, got.Cwd)
	require.Equal(t, before.InputTokens, got.InputTokens)
	require.False(t, svc.Coordinator.Busy(f.project.ID))

	events, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)
	require.Len(t, events, len(eventsBefore)+1)
	require.Equal(t, eventsBefore, events[:len(eventsBefore)])
	card := events[len(events)-1]
	require.Equal(t, db.EvCommand, card.Kind)
	require.Equal(t, "clear", card.Content)
	require.JSONEq(t, `{"name":"clear","state":"ok","previous_session_id":"`+before.SessionID+`","session_id":"`+got.SessionID+`"}`, card.Payload)
	require.Len(t, seen.all(), 1, "/clear is never sent to the model")
	proj, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	require.Equal(t, f.project.Goal, proj.Goal)
	require.Equal(t, before.ID, proj.MainThreadID)

	_, err = svc.Coordinator.Post(f.project.ID, "after clear", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, settled(f, svc, 2, &seen), 5*time.Second, 10*time.Millisecond)
	prompt := seen.all()[1]
	require.Contains(t, prompt, "after clear")
	require.Contains(t, prompt, "Current project context", "the new session is given the project context again")
	require.NotContains(t, prompt, "old secret", "the previous conversation is not carried into the new session")
	require.NotContains(t, prompt, "Previous project conversation")
	launches := f.agent.Launches()
	require.Len(t, launches, 2)
	require.False(t, launches[1].Resume)
	require.Equal(t, got.SessionID, launches[1].SessionID)
	after, err := f.store.GetThread(before.ID)
	require.NoError(t, err)
	require.Equal(t, got.SessionID, after.SessionID)
	require.NotZero(t, after.Turns)
}

func TestCoordinatorClearIsRefusedWhileItWorks(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error {
		seen.add(c)
		once.Do(func() { close(started) })
		<-release
		c.Reply("answer")
		return nil
	})
	_, err := svc.Coordinator.Post(f.project.ID, "first", nil, Source{})
	require.NoError(t, err)
	<-started
	_, err = svc.Coordinator.Post(f.project.ID, "queued behind it", nil, Source{})
	require.NoError(t, err)
	before, err := f.store.GetThread(f.project.MainThreadID)
	require.NoError(t, err)
	events, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)
	for _, mode := range []SendMode{SendNow, SendQueue, SendStopAndSend} {
		_, err := svc.SendWith(context.Background(), before.ID, "/clear", nil, Source{}, mode)
		require.ErrorIs(t, err, ErrBusy)
		require.ErrorContains(t, err, "cannot start a new session")
	}
	after, err := f.store.GetThread(before.ID)
	require.NoError(t, err)
	require.Equal(t, before.SessionID, after.SessionID)
	got, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)
	require.Equal(t, events, got, "a refused /clear leaves no event behind")

	close(release)
	require.Eventually(t, settled(f, svc, 2, &seen), 5*time.Second, 10*time.Millisecond)
	_, err = svc.SendWith(context.Background(), before.ID, "/clear", nil, Source{}, SendNow)
	require.NoError(t, err)
}

func TestCoordinatorClearRefusesArgumentsAndAnInterruptedInput(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { seen.add(c); c.Reply("answer"); return nil })
	main := f.project.MainThreadID
	_, err := svc.SendWith(context.Background(), main, "/clear now", nil, Source{}, SendNow)
	require.ErrorContains(t, err, "does not take arguments")
	require.Empty(t, seen.all())

	ev := &db.ThreadEvent{ThreadID: main, Kind: db.EvUser, Content: "interrupted"}
	require.NoError(t, f.store.AddCoordinatorInput(f.project.ID, ev))
	input, err := f.store.ClaimCoordinatorInput(f.project.ID)
	require.NoError(t, err)
	require.NoError(t, f.store.FinishCoordinatorInput(input.ID, false))
	_, err = svc.SendWith(context.Background(), main, "/clear", nil, Source{}, SendNow)
	require.ErrorIs(t, err, ErrBusy)
	require.ErrorContains(t, err, "recovery")
}

func TestCoordinatorClearFollowsTheProjectState(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, nil, nil)
	require.NoError(t, svc.SetPaused(f.project, true))
	_, err := svc.SendWith(context.Background(), f.project.MainThreadID, "/clear", nil, Source{}, SendNow)
	require.ErrorIs(t, err, ErrPaused)
}
