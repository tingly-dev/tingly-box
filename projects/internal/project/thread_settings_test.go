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

func TestWorkerLaunchReadsDefaultsAndOverridesAfterWaitingForCapacity(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	slot := f.threads.slot(f.project.ID)
	for range MaxRunningPerProject {
		slot <- struct{}{}
	}
	// Cleanup must release blocked launches even if an assertion fails.
	defer func() {
		for len(slot) > 0 {
			<-slot
		}
	}()
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "queued"})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		events, _ := f.store.ListThreadEvents(th.ID)
		for _, e := range events {
			if strings.HasPrefix(e.Content, "Queued:") {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	p.ThreadModel, p.ThreadPermissionMode = "fresh-default", "auto"
	require.NoError(t, f.store.SaveProject(p))
	<-slot
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	launches := f.agent.Launches()
	assert.Equal(t, "fresh-default", launches[0].Model)
	assert.Equal(t, "auto", launches[0].PermissionMode)
}

func TestWorkerHandoffUsesConfiguredModelNotLastObservedModel(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Init("observed-model")
		turn.Say("done", 1, 1)
		return nil
	})
	f.project.ThreadModel = "project-default"
	require.NoError(t, f.store.SaveProject(f.project))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "handoff"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	_, err = f.threads.SetModel(context.Background(), th.ID, "chosen-override")
	require.NoError(t, err)
	cmd, err := f.threads.Handoff(th.ID)
	require.NoError(t, err)
	assert.Contains(t, cmd, "--model chosen-override")
	assert.NotContains(t, cmd, "observed-model")

	_, err = f.threads.SetModel(context.Background(), th.ID, "")
	require.NoError(t, err)
	cmd, err = f.threads.Handoff(th.ID)
	require.NoError(t, err)
	assert.Contains(t, cmd, "--model project-default")
	assert.NotContains(t, cmd, "observed-model")
}

func TestWorkerModelOverrideIsIndependentAndSurvivesSettlement(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Init("actual-reported-model")
		turn.Say("done", 1, 1)
		return nil
	})
	f.project.ThreadModel = "project-model"
	f.project.ThreadPermissionMode = "auto"
	require.NoError(t, f.store.SaveProject(f.project))
	first, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "first"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, first.ID, db.ThreadIdle)
	second, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "second"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, second.ID, db.ThreadIdle)

	_, err = f.threads.SetModel(context.Background(), first.ID, " custom-model ")
	require.NoError(t, err, "workers must support independent model selection")
	_, err = f.threads.SetPermissionMode(first.ID, "plan")
	require.NoError(t, err)
	_, err = f.threads.Send(context.Background(), first.ID, "continue", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	got := waitStatus(t, f.store, first.ID, db.ThreadIdle)
	assert.Equal(t, "custom-model", got.ModelOverride)
	assert.Equal(t, "actual-reported-model", got.Model)
	assert.Equal(t, "actual-reported-model", got.ReportedModel)
	launches := f.agent.Launches()
	assert.Equal(t, "custom-model", launches[len(launches)-1].Model)
	assert.Equal(t, "plan", launches[len(launches)-1].PermissionMode)
	p, err := f.store.GetProject(f.project.ID)
	require.NoError(t, err)
	assert.Equal(t, "project-model", p.ThreadModel)
	assert.Equal(t, "auto", p.ThreadPermissionMode)
	sibling, err := f.store.GetThread(second.ID)
	require.NoError(t, err)
	assert.Empty(t, sibling.ModelOverride)
	assert.Empty(t, sibling.PermissionMode)

	// An observed model is history, not a request for the next turn.
	got.Model = "observed-provider-model"
	require.NoError(t, f.store.SaveThread(got))
	_, err = f.threads.SetModel(context.Background(), first.ID, "")
	require.NoError(t, err)
	_, err = f.threads.SetPermissionMode(first.ID, "")
	require.NoError(t, err)
	p.ThreadModel = "new-project-model"
	p.ThreadPermissionMode = "default"
	require.NoError(t, f.store.SaveProject(p))
	_, err = f.threads.Send(context.Background(), first.ID, "inherit again", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	got = waitStatus(t, f.store, first.ID, db.ThreadIdle)
	assert.Empty(t, got.ModelOverride)
	launches = f.agent.Launches()
	assert.Equal(t, "new-project-model", launches[len(launches)-1].Model)
	assert.Equal(t, "default", launches[len(launches)-1].PermissionMode)
}

func TestWorkerReportedModelDoesNotUseLaunchFallback(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error {
		turn.Say("done", 1, 1)
		return nil
	})
	f.project.ThreadModel = "configured-alias"
	require.NoError(t, f.store.SaveProject(f.project))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "check model"})
	require.NoError(t, err)
	f.report(t)
	got := waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Equal(t, "configured-alias", got.Model)
	assert.Empty(t, got.ReportedModel)
}

func TestThreadEffortInheritsAndOverridesPerTurn(t *testing.T) {
	f := newFixture(t, func(turn *enginetest.Turn) error { turn.Say("done", 1, 1); return nil })
	f.project.ThreadEffort = "medium"
	require.NoError(t, f.store.SaveProject(f.project))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Task: "work"})
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Equal(t, "medium", f.agent.Launches()[0].Effort)
	_, err = f.threads.SetEffort(th.ID, "high")
	require.NoError(t, err)
	_, err = f.threads.Send(context.Background(), th.ID, "continue", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Equal(t, "high", f.agent.Launches()[1].Effort)
	_, err = f.threads.SetEffort(th.ID, "")
	require.NoError(t, err)
	_, err = f.threads.Send(context.Background(), th.ID, "inherit", Source{}, nil)
	require.NoError(t, err)
	f.report(t)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Equal(t, "medium", f.agent.Launches()[2].Effort)
	_, err = f.threads.SetEffort(th.ID, "invalid")
	require.Error(t, err)
}
