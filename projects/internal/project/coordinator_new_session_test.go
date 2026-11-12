package project

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/coordtest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestCoordinatorNewSessionFreshAfterReload(t *testing.T) {
	var seen prompts
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { seen.add(c); c.Reply("answer"); return nil })
	_, err := svc.Coordinator.Post(f.project.ID, "old secret", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 1 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	old, err := f.store.GetThread(f.project.MainThreadID)
	require.NoError(t, err)
	fresh, err := svc.Send(context.Background(), f.project.MainThreadID, "/new", nil, Source{})
	require.NoError(t, err)
	require.Equal(t, old.ID, fresh.ID)
	require.NotEqual(t, old.SessionID, fresh.SessionID)
	require.Equal(t, old.InputTokens, fresh.InputTokens)
	events, err := f.store.ListThreadEvents(old.ID)
	require.NoError(t, err)
	require.GreaterOrEqual(t, len(events), 3)
	// Reconstruct session manager, retaining only persistent state.
	svc.Coordinator.sessions = newCoordinatorSessions(svc.Coordinator, testHost{}, svc.Coordinator.sessions.tools, Agents{Service: f.agent.Service()})
	_, err = svc.Coordinator.Post(f.project.ID, "fresh message", nil, Source{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(seen.all()) == 2 && !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
	require.NotContains(t, seen.all()[1], "old secret")
	require.Contains(t, seen.all()[1], "Be careful.")
	require.Contains(t, seen.all()[1], "fresh message")
	launches := f.agent.Launches()
	require.Len(t, launches, 2)
	require.False(t, launches[1].Resume)
	require.Equal(t, fresh.SessionID, launches[1].SessionID)
}

func TestCoordinatorNewSessionRejectsUnresolvedDecision(t *testing.T) {
	f, svc := newServiceFixture(t, testHost{}, nil, nil)
	svc.Coordinator.ask(f.project.ID, Decision{Question: "Choose?", Options: []DecisionOption{{Label: "A"}, {Label: "B"}}})
	_, err := svc.Coordinator.NewSession(f.project.ID, Source{})
	require.Error(t, err)
}

func TestCoordinatorNewSessionRejectsPendingAndUncertain(t *testing.T) {
	for _, state := range []string{"pending", "running", "uncertain"} {
		t.Run(state, func(t *testing.T) {
			f, svc := newServiceFixture(t, testHost{}, nil, nil)
			e := &db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvUser, Content: "waiting"}
			require.NoError(t, f.store.AddCoordinatorInput(f.project.ID, e))
			if state != "pending" {
				input, err := f.store.ClaimCoordinatorInput(f.project.ID)
				require.NoError(t, err)
				if state == "uncertain" {
					require.NoError(t, f.store.FinishCoordinatorInput(input.ID, false))
				}
			}
			_, err := svc.Coordinator.NewSession(f.project.ID, Source{})
			require.Error(t, err)
		})
	}
}

func TestCoordinatorNewSessionRejectsActiveTurn(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { close(started); <-release; c.Reply("done"); return nil })
	_, err := svc.Coordinator.Post(f.project.ID, "start", nil, Source{})
	require.NoError(t, err)
	<-started
	_, err = svc.Coordinator.NewSession(f.project.ID, Source{})
	require.Error(t, err)
	close(release)
	require.Eventually(t, func() bool { return !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
}

func TestCoordinatorNewSessionSerializesAdmissions(t *testing.T) {
	for _, report := range []bool{false, true} {
		t.Run(map[bool]string{false: "post", true: "report"}[report], func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			f, svc := newServiceFixture(t, testHost{}, nil, func(c *coordtest.Turn) error { close(started); <-release; c.Reply("done"); return nil })
			done := make(chan struct{})
			svc.Coordinator.admission.Lock()
			go func() {
				defer close(done)
				if report {
					svc.Coordinator.onReport(Report{Thread: db.Thread{ID: "child", ProjectID: f.project.ID}, Kind: ReportFinished, Text: "report"})
				} else {
					_, _ = svc.Coordinator.Post(f.project.ID, "message", nil, Source{})
				}
			}()
			select {
			case <-done:
				t.Fatal("admission passed lock")
			case <-time.After(20 * time.Millisecond):
			}
			svc.Coordinator.admission.Unlock()
			<-done
			<-started
			_, err := svc.Coordinator.NewSession(f.project.ID, Source{})
			require.Error(t, err)
			close(release)
			require.Eventually(t, func() bool { return !svc.Coordinator.Busy(f.project.ID) }, 5*time.Second, 10*time.Millisecond)
		})
	}
}
