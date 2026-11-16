package agentboot_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

func controlAnswer(t *testing.T, h *process.FakeHandle, ctl map[string]any, err string) {
	t.Helper()
	resp := map[string]any{"subtype": "success", "request_id": ctl["request_id"]}
	if err != "" {
		resp = map[string]any{"subtype": "error", "request_id": ctl["request_id"], "error": err}
	}
	writeLine(t, h, map[string]any{"type": "control_response", "response": resp})
}

// A message sent while a turn runs joins that turn: it is written to the
// process as a user message, the turn still ends with one result, and the
// session stays Running throughout.
func TestConductor_InjectJoinsTheRunningTurn(t *testing.T) {
	got := make(chan map[string]any, 1)
	finish := make(chan struct{})
	firstRead := make(chan struct{})
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec) // turn one
			close(firstRead)
			writeLine(t, h, initEvent())
			msg, _ := decodeOne(dec) // the injected message
			got <- msg
			<-finish
			writeLine(t, h, assistantText("m1", "done, with the extra"))
			writeResult(t, h, false)
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	c := openConducted(t, factory, newConductorRecorder())

	type outcome struct {
		res *agentboot.Result
		err error
	}
	turn := make(chan outcome, 1)
	go func() {
		res, err := c.Await(context.Background(), nil)
		turn <- outcome{res, err}
	}()

	<-firstRead
	require.NoError(t, c.Inject(context.Background(), "also do the extra thing"))
	msg := <-got
	assert.Equal(t, "user", msg["type"])
	assert.Equal(t, map[string]any{"role": "user", "content": "also do the extra thing"}, msg["message"])
	assert.Equal(t, agentboot.SessionStateRunning, c.Session().Status(), "injecting must not change the turn")

	// A second Send is still refused: injection is not a second turn.
	assert.ErrorIs(t, c.Session().Send(context.Background(), "new turn"), agentboot.ErrTurnInFlight)

	close(finish)
	select {
	case o := <-turn:
		require.NoError(t, o.err)
	case <-time.After(2 * time.Second):
		t.Fatal("the turn never completed")
	}
	assert.Equal(t, agentboot.SessionStateIdle, c.Session().Status())
}

func TestConductor_InjectNeedsARunningTurn(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec)
			writeResult(t, h, false)
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	c := openConducted(t, factory, newConductorRecorder())
	_, err := c.Await(context.Background(), nil)
	require.NoError(t, err)

	assert.ErrorIs(t, c.Inject(context.Background(), "late"), agentboot.ErrNoTurnInFlight)

	require.NoError(t, c.Session().Close(context.Background()))
	assert.ErrorIs(t, c.Inject(context.Background(), "later"), agentboot.ErrSessionClosed)
}

// SetModel and SetPermissionMode write the matching control request and
// return once the agent answers it.
func TestConductor_LiveControlWaitsForTheAnswer(t *testing.T) {
	got := make(chan map[string]any, 4)
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec)
			writeResult(t, h, false)
			for i := 0; i < 3; i++ {
				ctl, _ := decodeOne(dec)
				got <- ctl
				if i == 2 {
					controlAnswer(t, h, ctl, "Unsupported control request subtype: set_model")
				} else {
					controlAnswer(t, h, ctl, "")
				}
			}
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	c := openConducted(t, factory, newConductorRecorder())
	_, err := c.Await(context.Background(), nil)
	require.NoError(t, err)

	require.NoError(t, c.SetModel(context.Background(), "claude-haiku-5-5"))
	assert.Equal(t, map[string]any{"subtype": "set_model", "model": "claude-haiku-5-5"}, (<-got)["request"])

	require.NoError(t, c.SetPermissionMode(context.Background(), "acceptEdits"))
	assert.Equal(t, map[string]any{"subtype": "set_permission_mode", "mode": "acceptEdits"}, (<-got)["request"])

	// An empty model resets to the default, and a refusal comes back as one.
	err = c.SetModel(context.Background(), "")
	var rejected *agentboot.ControlRejectedError
	require.ErrorAs(t, err, &rejected)
	assert.Contains(t, rejected.Message, "Unsupported")
	assert.Equal(t, map[string]any{"subtype": "set_model", "model": "default"}, (<-got)["request"])
}

// A process that never answers must not hang the caller.
func TestConductor_LiveControlHonoursContext(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec)
			writeResult(t, h, false)
			_, _ = decodeOne(dec) // the request, never answered
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	c := openConducted(t, factory, newConductorRecorder())
	_, err := c.Await(context.Background(), nil)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	assert.ErrorIs(t, c.SetModel(ctx, "x"), context.DeadlineExceeded)
}
