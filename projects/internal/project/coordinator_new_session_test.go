package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
)

func TestCoordinatorClearIsRejectedWithoutChangingHistory(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { seen.add(c); c.Reply("answer"); return nil })
	_, err := svc.Coordinator.Post(f.project.ID, "old secret", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	before, err := f.store.GetThread(f.project.MainThreadID)
	require.NoError(t, err)
	events, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)
	for _, mode := range []SendMode{SendNow, SendQueue, SendStopAndSend} {
		_, err := svc.SendWith(context.Background(), before.ID, "/clear", nil, Source{}, mode)
		require.ErrorContains(t, err, "coordinator")
	}
	_, err = svc.Threads.NewSession(before.ID, Source{})
	require.ErrorIs(t, err, ErrMainThread)
	after, err := f.store.GetThread(before.ID)
	require.NoError(t, err)
	require.Equal(t, before.SessionID, after.SessionID)
	require.Equal(t, before.Turns, after.Turns)
	got, err := f.store.ListThreadEvents(before.ID)
	require.NoError(t, err)
	require.Equal(t, events, got)
	require.Len(t, seen.all(), 1)
}
