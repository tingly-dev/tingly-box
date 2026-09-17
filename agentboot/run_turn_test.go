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

