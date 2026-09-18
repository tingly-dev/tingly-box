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

// writeResult pushes a scripted stream-json "result" event to h's stdout,
// mirroring the shape a real claude process ends a turn with.
func writeResult(t *testing.T, h *process.FakeHandle, isError bool) {
	t.Helper()
	subtype := claude.ResultSubtypeSuccess
	if isError {
		subtype = "error_during_execution"
	}
	result, err := json.Marshal(map[string]any{
		"type":     claude.SDKResultMessage,
		"subtype":  subtype,
		"is_error": isError,
	})
	require.NoError(t, err)
	_, err = h.WriteOutput(append(result, '\n'))
	require.NoError(t, err)
}

// decodeOne reads exactly one JSON value from the handle's stdin pipe,
// mirroring how the real claude CLI reads one stream-json message at a time.
func decodeOne(dec *json.Decoder) (map[string]any, error) {
	var v map[string]any
	err := dec.Decode(&v)
	return v, err
}

func TestRunner_Open_TwoTurnsOverOneProcess(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)

			// Turn 1 (Open's prompt, delivered via InitialInput).
			if _, err := decodeOne(dec); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}
			writeResult(t, h, false)

			// Turn 2 (via Send, on the same stdin/process).
			if _, err := decodeOne(dec); err != nil {
				h.FinishOutput()
				h.SignalExit(err)
				return
			}
			writeResult(t, h, false)

			// Wait for Close to end stdin, then exit cleanly.
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	session, err := agent.Open(ctx, "turn one", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	// A turn is already in flight from Open; a concurrent Send must be
	// rejected until it completes.
	assert.ErrorIs(t, session.Send(ctx, "too early"), agentboot.ErrTurnInFlight)

	first := requireTurnComplete(t, session)
	assert.True(t, first.IsSuccess())
	assert.Equal(t, agentboot.SessionStateIdle, session.Status())

	// Send only guarantees the turn was submitted, not that Status() still
	// reads Running by the time this goroutine checks it — with the fake
	// process replying near-instantly, the turn can already be back to
	// Idle. Status() is checked once more below via the happens-before
	// edge a channel receive gives us (requireTurnComplete's read of
	// TurnCompleteEvent happens after completeTurn's state write).
	require.NoError(t, session.Send(ctx, "turn two"))

	second := requireTurnComplete(t, session)
	assert.True(t, second.IsSuccess())
	assert.Equal(t, agentboot.SessionStateIdle, session.Status())

	require.NoError(t, session.Close(ctx))
	assert.Equal(t, agentboot.SessionStateTerminated, session.Status())

	// Close's final SessionStateEvent{Terminated} may still be sitting in
	// the buffered channel; drain until it actually closes.
	sawTerminated := false
	for ev := range session.Events() {
		if se, ok := ev.(agentboot.SessionStateEvent); ok && se.State == agentboot.SessionStateTerminated {
			sawTerminated = true
		}
	}
	assert.True(t, sawTerminated)
}

func TestRunner_Open_UnexpectedExitTerminatesSession(t *testing.T) {
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
			// Crash without ever observing stdin close.
			h.FinishOutput()
			h.SignalExit(errors.New("boom"))
		}()
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	session, err := agent.Open(ctx, "turn one", agentboot.ExecutionOptions{})
	require.NoError(t, err)

	first := requireTurnComplete(t, session)
	assert.True(t, first.IsSuccess())

	// The process is gone; the session must observe that on its own and
	// terminate without a Close call.
	var stateEvent *agentboot.SessionStateEvent
	for ev := range session.Events() {
		if se, ok := ev.(agentboot.SessionStateEvent); ok {
			se := se
			stateEvent = &se
		}
	}
	require.NotNil(t, stateEvent)
	assert.Equal(t, agentboot.SessionStateTerminated, stateEvent.State)
	assert.Equal(t, agentboot.SessionStateTerminated, session.Status())

	assert.ErrorIs(t, session.Send(ctx, "too late"), agentboot.ErrSessionClosed)
}

// requireTurnComplete drains Events() until TurnCompleteEvent, failing the
// test if the channel closes first.
func requireTurnComplete(t *testing.T, session agentboot.PersistentSession) *agentboot.Result {
	t.Helper()
	for ev := range session.Events() {
		if tc, ok := ev.(agentboot.TurnCompleteEvent); ok {
			return tc.Result
		}
	}
	t.Fatal("session Events() closed before a TurnCompleteEvent arrived")
	return nil
}
