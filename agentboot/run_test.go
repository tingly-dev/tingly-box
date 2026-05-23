package agentboot_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// capturePrompter records the requests it receives and returns canned
// responses, so tests can assert RunWithPrompter's dispatch and error handling.
type capturePrompter struct {
	approvals  []agentboot.ApprovalRequestEvent
	asks       []agentboot.AskRequestEvent
	approve    bool
	onApprover error
	onAsker    error
}

func (p *capturePrompter) OnApproval(_ context.Context, req agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	p.approvals = append(p.approvals, req)
	if p.onApprover != nil {
		return agentboot.ApprovalResponse{}, p.onApprover
	}
	return agentboot.ApprovalResponse{Approved: p.approve}, nil
}

func (p *capturePrompter) OnAsk(_ context.Context, req agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	p.asks = append(p.asks, req)
	if p.onAsker != nil {
		return agentboot.AskResponse{}, p.onAsker
	}
	return agentboot.AskResponse{Approved: true, Response: "ok"}, nil
}

// controlledRun wires a NewControlledHandle, emits the given events from a
// producer goroutine, runs RunWithPrompter, and returns the result plus the
// responses routed back through Respond.
//
// The producer only closes the handle once every control request has been
// answered, mirroring the real runner (the agent keeps the stream open while
// it waits for a response). Closing earlier would race Respond against the
// channel close.
func controlledRun(
	t *testing.T,
	events []agentboot.StreamEvent,
	prompter agentboot.Prompter,
	sink agentboot.MessageSink,
	want *agentboot.Result,
) (*agentboot.Result, error, map[string]agentboot.ControlResponse) {
	t.Helper()

	controls := 0
	for _, ev := range events {
		switch ev.(type) {
		case agentboot.ApprovalRequestEvent, agentboot.AskRequestEvent:
			controls++
		}
	}

	responses := map[string]agentboot.ControlResponse{}
	responded := make(chan struct{}, controls+1)
	h, ctrl := agentboot.NewControlledHandle(
		len(events)+1,
		func(reqID string, resp agentboot.ControlResponse) error {
			responses[reqID] = resp
			responded <- struct{}{}
			return nil
		},
		func() {},
		func() (*agentboot.Result, error) { return want, nil },
	)

	go func() {
		ctx := context.Background()
		for _, ev := range events {
			ctrl.Emit(ctx, ev)
		}
		for i := 0; i < controls; i++ {
			<-responded
		}
		ctrl.Close()
	}()

	got, err := agentboot.RunWithPrompter(context.Background(), h, prompter, sink)
	return got, err, responses
}

func TestRunWithPrompter_DispatchesAllEventTypes(t *testing.T) {
	want := &agentboot.Result{Output: "done", ExitCode: 0}
	prompter := &capturePrompter{approve: true}

	var sink []any
	got, err, responses := controlledRun(t,
		[]agentboot.StreamEvent{
			agentboot.MessageEvent{Raw: "hello"},
			agentboot.ApprovalRequestEvent{ID: "ap1", ToolName: "bash"},
			agentboot.AskRequestEvent{ID: "as1", Message: "pick one"},
		},
		prompter,
		func(raw any) { sink = append(sink, raw) },
		want,
	)

	require.NoError(t, err)
	assert.Same(t, want, got, "Wait() result is returned")

	// MessageEvent.Raw reaches the sink.
	require.Len(t, sink, 1)
	assert.Equal(t, "hello", sink[0])

	// Approval/Ask reach the prompter...
	require.Len(t, prompter.approvals, 1)
	assert.Equal(t, "bash", prompter.approvals[0].ToolName)
	require.Len(t, prompter.asks, 1)
	assert.Equal(t, "pick one", prompter.asks[0].Message)

	// ...and the responses are routed back via Respond, keyed by request ID.
	ap, ok := responses["ap1"].(agentboot.ApprovalResponse)
	require.True(t, ok, "approval response routed")
	assert.True(t, ap.Approved)
	as, ok := responses["as1"].(agentboot.AskResponse)
	require.True(t, ok, "ask response routed")
	assert.True(t, as.Approved)
}

// TestRunWithPrompter_ForwardsErrorEventToSink locks in the fix where a
// non-fatal ErrorEvent must reach the sink instead of being log-only.
func TestRunWithPrompter_ForwardsErrorEventToSink(t *testing.T) {
	want := &agentboot.Result{Output: "ok"}
	wantErr := errors.New("decode boom")

	var sink []any
	got, err, _ := controlledRun(t,
		[]agentboot.StreamEvent{
			agentboot.MessageEvent{Raw: "partial"},
			agentboot.ErrorEvent{Err: wantErr},
		},
		&capturePrompter{approve: true},
		func(raw any) { sink = append(sink, raw) },
		want,
	)

	require.NoError(t, err)
	assert.Same(t, want, got)

	require.Len(t, sink, 2)
	assert.Equal(t, "partial", sink[0])
	ev, ok := sink[1].(agentboot.ErrorEvent)
	require.True(t, ok, "ErrorEvent is forwarded to the sink")
	assert.Equal(t, wantErr, ev.Err)
}

// TestRunWithPrompter_PrompterErrorDenies verifies that when the prompter
// returns an error, RunWithPrompter still responds with a safe deny.
func TestRunWithPrompter_PrompterErrorDenies(t *testing.T) {
	prompter := &capturePrompter{onApprover: errors.New("prompter exploded")}

	_, err, responses := controlledRun(t,
		[]agentboot.StreamEvent{
			agentboot.ApprovalRequestEvent{ID: "ap1", ToolName: "rm"},
		},
		prompter,
		nil,
		&agentboot.Result{},
	)

	require.NoError(t, err)
	ap, ok := responses["ap1"].(agentboot.ApprovalResponse)
	require.True(t, ok)
	assert.False(t, ap.Approved, "prompter error must default-deny")
	assert.Equal(t, "prompter exploded", ap.Reason)
}

// TestRunWithPrompter_NilSink ensures message and error events are dropped
// safely (no panic) when no sink is supplied.
func TestRunWithPrompter_NilSink(t *testing.T) {
	want := &agentboot.Result{Output: "done"}
	got, err, _ := controlledRun(t,
		[]agentboot.StreamEvent{
			agentboot.MessageEvent{Raw: "x"},
			agentboot.ErrorEvent{Err: errors.New("e")},
		},
		&capturePrompter{},
		nil,
		want,
	)
	require.NoError(t, err)
	assert.Same(t, want, got)
}
