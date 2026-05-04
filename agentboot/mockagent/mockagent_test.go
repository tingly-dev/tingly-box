package mock

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Example_mockAgent demonstrates using mock agent with agentboot
func Example_mockAgent() {
	// Create mock agent with custom config
	mockAgent := NewAgent(Config{
		MaxIterations: 3,
		StepDelay:     100 * time.Millisecond, // Fast for testing
		AutoApprove:   true,                   // Auto-approve for demo
	})

	// Execute with context
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := mockAgent.Execute(ctx, "Hello, mock agent!", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Success: %v\n", result.IsSuccess())
	fmt.Printf("Steps: %d events\n", len(result.Events))
	// Output:
	// Success: true
	// Steps: 11 events
}

// Example_mockAgentWithHandler demonstrates mock agent with message handler
func Example_mockAgentWithHandler() {
	// Create mock agent
	mockAgent := NewAgent(Config{
		MaxIterations: 2,
		StepDelay:     50 * time.Millisecond,
		AutoApprove:   false, // Require manual approval
	})

	// Create message handler that auto-approves
	handler := agentboot.NewCompositeHandler().
		SetApprovalHandler(&autoApprovalHandler{}).
		SetAskHandler(&autoAskHandler{})

	// Execute with handler
	ctx := context.Background()
	result, err := mockAgent.Execute(ctx, "Test with handler", agentboot.ExecutionOptions{
		Handler:      handler,
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Completed: %v\n", result.IsSuccess())
	// Output:
	// Completed: true
}

// Example_mockAgentWithAskUserQuestion demonstrates mock agent with AskUserQuestion
func Example_mockAgentWithAskUserQuestion() {
	// Create mock agent that sends AskUserQuestion every 2 steps
	mockAgent := NewAgent(Config{
		MaxIterations:           4,
		StepDelay:               50 * time.Millisecond,
		AutoApprove:             false,
		AskUserQuestionFrequency: 2, // Every 2 steps, send AskUserQuestion
	})

	// Create handler that auto-approves everything
	handler := agentboot.NewCompositeHandler().
		SetApprovalHandler(&autoApprovalHandler{}).
		SetAskHandler(&autoAskHandler{})

	// Execute
	ctx := context.Background()
	result, err := mockAgent.Execute(ctx, "Test with AskUserQuestion", agentboot.ExecutionOptions{
		Handler:      handler,
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Completed: %v\n", result.IsSuccess())
	fmt.Printf("Events: %d\n", len(result.Events))
	// Output:
	// Completed: true
	// Events: 14
}

// Test_ScriptedMockAgent_PlaybackOrder verifies that explicit script playback
// emits init / per-step / result events in declared order.
func Test_ScriptedMockAgent_PlaybackOrder(t *testing.T) {
	ag := NewAgent(Config{
		AutoApprove: true,
		Script: NewScript().
			Assistant("hello").
			Permission("Read", map[string]any{"path": "/tmp/x"}).
			ToolResult("hello\n").
			Assistant("done").
			Success("ok").
			Build(),
	})

	result, err := ag.Execute(context.Background(), "go", agentboot.ExecutionOptions{})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	wantTypes := []string{
		agentboot.EventTypeInit,
		agentboot.EventTypeAssistant,
		agentboot.EventTypePermissionRequest,
		agentboot.EventTypePermissionResult,
		agentboot.EventTypeToolResult,
		agentboot.EventTypeAssistant,
		agentboot.EventTypeResult,
	}
	if len(result.Events) != len(wantTypes) {
		t.Fatalf("expected %d events, got %d (%v)", len(wantTypes), len(result.Events), eventTypes(result.Events))
	}
	for i, want := range wantTypes {
		if result.Events[i].Type != want {
			t.Errorf("event %d: want %q got %q", i, want, result.Events[i].Type)
		}
	}
	if result.Error != "" {
		t.Errorf("expected no Result.Error, got %q", result.Error)
	}
}

// Test_ScriptedMockAgent_DenyHalts verifies that PermissionStep with
// OnDenyTerminate (the default) terminates with permission_denied.
func Test_ScriptedMockAgent_DenyHalts(t *testing.T) {
	ag := NewAgent(Config{
		Script: NewScript().
			Permission("Bash", map[string]any{"cmd": "rm -rf /"}).
			Assistant("never reached").
			Success("never").
			Build(),
	})

	handler := agentboot.NewCompositeHandler().SetApprovalHandler(denyHandler{})
	result, err := ag.Execute(context.Background(), "go", agentboot.ExecutionOptions{Handler: handler})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}

	last := result.Events[len(result.Events)-1]
	if last.Type != agentboot.EventTypeResult {
		t.Fatalf("expected last event %q, got %q", agentboot.EventTypeResult, last.Type)
	}
	if status, _ := last.Data["status"].(string); status != "permission_denied" {
		t.Fatalf("expected status permission_denied, got %q", status)
	}
	for _, e := range result.Events {
		if e.Type == agentboot.EventTypeAssistant {
			if msg, _ := e.Data["message"].(string); msg == "never reached" {
				t.Fatalf("script kept running past denial")
			}
		}
	}
}

// Test_ScriptedMockAgent_ExpectMismatch verifies that ExpectApproved mismatch
// surfaces via Result.Error and a handler.OnError call.
func Test_ScriptedMockAgent_ExpectMismatch(t *testing.T) {
	ag := NewAgent(Config{
		Script: NewScript().
			Permission("Bash", nil, WithExpectApproved(true)).
			Build(),
	})

	rec := &errorRecorder{}
	handler := agentboot.NewCompositeHandler().
		SetStreamer(rec).
		SetApprovalHandler(denyHandler{})

	result, err := ag.Execute(context.Background(), "go", agentboot.ExecutionOptions{Handler: handler})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Error == "" {
		t.Fatalf("expected Result.Error from mismatch, got empty")
	}
	if len(rec.errs) == 0 {
		t.Fatalf("expected handler.OnError to be called for mismatch")
	}
}

// Test_ScriptedMockAgent_ErrorStep verifies that an ErrorStep delivers via
// handler.OnError and is surfaced in Result.Events.
func Test_ScriptedMockAgent_ErrorStep(t *testing.T) {
	ag := NewAgent(Config{
		Script: NewScript().
			Assistant("trying").
			FailWith(errors.New("boom")).
			Build(),
	})

	rec := &errorRecorder{}
	handler := agentboot.NewCompositeHandler().SetStreamer(rec)
	_, err := ag.Execute(context.Background(), "go", agentboot.ExecutionOptions{Handler: handler})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if len(rec.errs) != 1 || rec.errs[0].Error() != "boom" {
		t.Fatalf("expected exactly one OnError(\"boom\"), got %v", rec.errs)
	}
}

// Test_ScriptedMockAgent_AskAnswers verifies that AskStep records expected
// answer mismatches via Result.Error.
func Test_ScriptedMockAgent_AskAnswers(t *testing.T) {
	questions := []AskQuestion{
		{
			Question: "color?",
			Options: []AskOption{
				{Label: "red"},
				{Label: "green"},
			},
		},
	}
	ag := NewAgent(Config{
		Script: NewScript().
			Ask(questions, WithAskExpectAnswers(map[int]int{0: 1})). // expect "green"
			Build(),
	})

	// autoAskHandler always picks Option A → label "red" for the legacy script
	// paths; here it returns "Option A" which doesn't match either label, so
	// we wire a handler that explicitly returns "red".
	handler := agentboot.NewCompositeHandler().SetAskHandler(fixedAskHandler{label: "red"})

	result, err := ag.Execute(context.Background(), "go", agentboot.ExecutionOptions{Handler: handler})
	if err != nil {
		t.Fatalf("Execute returned error: %v", err)
	}
	if result.Error == "" {
		t.Fatalf("expected mismatch on ask answer, got no Result.Error")
	}
}

func eventTypes(evs []agentboot.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Type
	}
	return out
}

type denyHandler struct{}

func (denyHandler) OnApproval(_ context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return agentboot.PermissionResult{Approved: false, Reason: "test denial"}, nil
}

type fixedAskHandler struct{ label string }

func (h fixedAskHandler) OnAsk(_ context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	answers := map[string]interface{}{"0": h.label}
	updated := map[string]interface{}{}
	for k, v := range req.Input {
		updated[k] = v
	}
	updated["answers"] = answers
	return agentboot.AskResult{ID: req.ID, Approved: true, UpdatedInput: updated}, nil
}

type errorRecorder struct {
	errs []error
}

func (r *errorRecorder) OnMessage(interface{}) error { return nil }
func (r *errorRecorder) OnError(err error)           { r.errs = append(r.errs, err) }

// autoApprovalHandler is a simple handler that auto-approves all permissions
type autoApprovalHandler struct{}

func (h *autoApprovalHandler) OnApproval(ctx context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	return agentboot.PermissionResult{Approved: true, UpdatedInput: req.Input}, nil
}

// autoAskHandler is a simple handler that auto-approves all asks
type autoAskHandler struct{}

func (h *autoAskHandler) OnAsk(ctx context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	// For AskUserQuestion, add answers to the input
	if req.ToolName == "AskUserQuestion" && req.Input != nil {
		questions, ok := req.Input["questions"].([]interface{})
		if ok && len(questions) > 0 {
			// Provide default answers (select first option for each question)
			answers := make(map[string]interface{})
			for i := range questions {
				answers[fmt.Sprintf("%d", i)] = "Option A"
			}
			updatedInput := make(map[string]interface{})
			for k, v := range req.Input {
				updatedInput[k] = v
			}
			updatedInput["answers"] = answers
			return agentboot.AskResult{
				ID:           req.ID,
				Approved:     true,
				UpdatedInput: updatedInput,
			}, nil
		}
	}
	return agentboot.AskResult{ID: req.ID, Approved: true}, nil
}