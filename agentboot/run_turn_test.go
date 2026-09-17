package agentboot_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func TestRunTurnWithPrompter_CompletesAndReturnsResult(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			if _, err := decodeOne(dec); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}
			writeResult(t, h, false)
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	session, err := agent.Open(ctx, "hello", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	prompter := &capturePrompter{approve: true}
	var sunk []any
	result, rerr := agentboot.RunTurnWithPrompter(ctx, session, prompter, func(v any) { sunk = append(sunk, v) })
	require.NoError(t, rerr)
	require.NotNil(t, result)
	assert.True(t, result.IsSuccess())
	assert.Equal(t, agentboot.SessionStateIdle, session.Status())

	require.NoError(t, session.Close(ctx))
}

func TestRunTurnWithPrompter_SessionTerminatedMidTurnReturnsError(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			if _, err := decodeOne(dec); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}
			// Crash before ever emitting a result for this turn.
			h.FinishOutput()
			h.SignalExit(errors.New("boom"))
		}()
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	session, err := agent.Open(ctx, "hello", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	prompter := &capturePrompter{approve: true}
	result, rerr := agentboot.RunTurnWithPrompter(ctx, session, prompter, nil)
	assert.Nil(t, result)
	assert.Error(t, rerr)
	assert.Equal(t, agentboot.SessionStateTerminated, session.Status())
}

// TestRunTurnWithPrompter_CtxCancelClosesSession asserts the fix for a real
// bug: a persistent session's process is deliberately detached from any one
// caller's ctx (Runner.Open), so nothing previously stopped a runaway or
// user-canceled ("/stop") turn — the underlying process just kept running
// forever while the caller's ctx.Done() was silently ignored. Canceling ctx
// must now end the whole session (there's no way to interrupt just the
// in-flight turn) and RunTurnWithPrompter must return promptly.
func TestRunTurnWithPrompter_CtxCancelClosesSession(t *testing.T) {
	factory := process.NewFakeFactory()
	started := make(chan struct{})
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			if _, err := decodeOne(dec); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}
			close(started)
			// Hang: never reply to this turn — only stdin closing (from
			// Close()) should ever make this process exit.
			var next map[string]any
			_ = dec.Decode(&next)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	openCtx, openCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer openCancel()

	session, err := agent.Open(openCtx, "hello", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("fake process never observed the first turn's message")
	}

	turnCtx, turnCancel := context.WithCancel(context.Background())
	defer turnCancel()
	go func() {
		time.Sleep(20 * time.Millisecond)
		turnCancel()
	}()

	prompter := &capturePrompter{approve: true}
	result, rerr := agentboot.RunTurnWithPrompter(turnCtx, session, prompter, nil)
	assert.Nil(t, result)
	assert.ErrorIs(t, rerr, context.Canceled)

	require.Eventually(t, func() bool {
		return session.Status() == agentboot.SessionStateTerminated
	}, time.Second, 5*time.Millisecond, "ctx cancellation should close the whole session, not just abandon the turn")
}

