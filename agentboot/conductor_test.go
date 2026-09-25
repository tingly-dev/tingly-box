package agentboot_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/process"
)

// writeLine pushes one stream-json event to the fake process's stdout.
func writeLine(t *testing.T, h *process.FakeHandle, v map[string]any) {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	_, err = h.WriteOutput(append(b, '\n'))
	require.NoError(t, err)
}

func initEvent() map[string]any {
	return map[string]any{"type": "system", "subtype": "init", "session_id": "s1"}
}

func assistantText(id, text string) map[string]any {
	return map[string]any{"type": "assistant", "message": map[string]any{
		"id": id, "type": "message", "role": "assistant",
		"content": []any{map[string]any{"type": "text", "text": text}},
	}}
}

// conductorRecorder collects a Conductor's hook calls.
type conductorRecorder struct {
	mu          sync.Mutex
	texts       []string
	unsolicited chan struct{}
	completed   chan error
	terminated  chan string
}

func newConductorRecorder() *conductorRecorder {
	return &conductorRecorder{unsolicited: make(chan struct{}, 4), completed: make(chan error, 4), terminated: make(chan string, 1)}
}

func (r *conductorRecorder) hooks() agentboot.ConductorHooks {
	return agentboot.ConductorHooks{
		Sink: func(raw any) {
			if m, ok := raw.(*claude.AssistantMessage); ok {
				for _, b := range m.Message.Content {
					if b.Type == claude.ContentBlockTypeText {
						r.mu.Lock()
						r.texts = append(r.texts, b.Text)
						r.mu.Unlock()
					}
				}
			}
		},
		OnUnsolicitedTurn: func() (context.Context, agentboot.Prompter) {
			r.unsolicited <- struct{}{}
			return context.Background(), nil
		},
		OnUnsolicitedTurnComplete: func(_ *agentboot.Result, err error) { r.completed <- err },
		OnTerminated:              func(reason string) { r.terminated <- reason },
	}
}

func (r *conductorRecorder) seen() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.texts...)
}

func openConducted(t *testing.T, factory *process.FakeFactory, rec *conductorRecorder) *agentboot.Conductor {
	t.Helper()
	agent := claude.NewAgentWithFactory(claude.Config{}, factory)
	session, err := agent.Open(context.Background(), "turn one", agentboot.ExecutionOptions{})
	require.NoError(t, err)
	c := agentboot.NewConductor(session, rec.hooks())
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = session.Close(ctx)
	})
	return c
}

// A background task finishing after the turn that started it makes Claude
// Code run a turn of its own. Its events must be read as they arrive, not
// left in the buffer for the next Send to mistake for its own turn.
func TestConductor_UnsolicitedTurnIsReadAndClosed(t *testing.T) {
	release := make(chan struct{})
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec) // turn one
			writeLine(t, h, initEvent())
			writeLine(t, h, assistantText("m1", "started a background task"))
			writeResult(t, h, false)

			// The task finishes: the CLI starts a turn by itself.
			writeLine(t, h, map[string]any{"type": "system", "subtype": "task_notification", "task_id": "b1", "status": "completed"})
			writeLine(t, h, initEvent())
			writeLine(t, h, assistantText("m2", "the task finished"))
			<-release
			writeResult(t, h, false)

			_, _ = decodeOne(dec) // turn two, sent by the host
			writeLine(t, h, initEvent())
			writeLine(t, h, assistantText("m3", "second answer"))
			writeResult(t, h, false)

			_, _ = decodeOne(dec) // stdin closes
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	rec := newConductorRecorder()
	c := openConducted(t, factory, rec)

	_, err := c.Await(context.Background(), nil)
	require.NoError(t, err)

	select {
	case <-rec.unsolicited:
	case <-time.After(2 * time.Second):
		t.Fatal("the agent-started turn was never announced")
	}
	// While the agent's own turn runs, a host turn must wait.
	_, err = c.RunTurn(context.Background(), "too early", nil)
	assert.ErrorIs(t, err, agentboot.ErrTurnInFlight)

	close(release)
	select {
	case err := <-rec.completed:
		require.NoError(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("the agent-started turn was never closed")
	}

	_, err = c.RunTurn(context.Background(), "turn two", nil)
	require.NoError(t, err)
	assert.Equal(t, []string{"started a background task", "the task finished", "second answer"}, rec.seen())
}

// Canceling a turn interrupts it rather than closing the session: the
// process, and whatever it runs in the background, stays up for the next turn.
func TestConductor_CancelInterruptsTheTurnAndKeepsTheProcess(t *testing.T) {
	gotInterrupt := make(chan map[string]any, 1)
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec) // turn one: runs until interrupted
			writeLine(t, h, initEvent())
			ctl, _ := decodeOne(dec)
			gotInterrupt <- ctl
			writeLine(t, h, map[string]any{"type": "control_response", "response": map[string]any{"subtype": "success", "request_id": ctl["request_id"]}})
			writeResult(t, h, true) // error_during_execution, as the CLI reports it

			_, _ = decodeOne(dec) // turn two
			writeLine(t, h, initEvent())
			writeResult(t, h, false)

			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	rec := newConductorRecorder()
	c := openConducted(t, factory, rec)

	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(50*time.Millisecond, cancel)
	_, err := c.Await(ctx, nil)
	assert.ErrorIs(t, err, context.Canceled)

	ctl := <-gotInterrupt
	assert.Equal(t, "control_request", ctl["type"])
	assert.Equal(t, map[string]any{"subtype": "interrupt"}, ctl["request"])
	assert.Equal(t, agentboot.SessionStateIdle, c.Session().Status(), "interrupting must not end the process")

	_, err = c.RunTurn(context.Background(), "turn two", nil)
	require.NoError(t, err)
}

func TestConductor_StopTaskSendsTheTaskID(t *testing.T) {
	got := make(chan map[string]any, 1)
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec)
			writeResult(t, h, false)
			ctl, _ := decodeOne(dec)
			got <- ctl
			_, _ = decodeOne(dec)
			h.FinishOutput()
			h.SignalExit(nil)
		}()
	}
	c := openConducted(t, factory, newConductorRecorder())
	_, err := c.Await(context.Background(), nil)
	require.NoError(t, err)

	require.NoError(t, c.StopTask(context.Background(), "bjtpax2er"))
	ctl := <-got
	assert.Equal(t, map[string]any{"subtype": "stop_task", "task_id": "bjtpax2er"}, ctl["request"])
}

func TestConductor_ReportsTheProcessEnding(t *testing.T) {
	factory := process.NewFakeFactory()
	factory.OnStart = func(_ context.Context, _ process.LaunchSpec, h *process.FakeHandle) {
		go func() {
			dec := json.NewDecoder(h.StdinR)
			_, _ = decodeOne(dec)
			h.FinishOutput() // crashes mid-turn
			h.SignalExit(errors.New("boom"))
		}()
	}
	rec := newConductorRecorder()
	c := openConducted(t, factory, rec)

	_, err := c.Await(context.Background(), nil)
	require.Error(t, err)
	select {
	case <-rec.terminated:
	case <-time.After(2 * time.Second):
		t.Fatal("OnTerminated never ran")
	}
	<-c.Done()
}
