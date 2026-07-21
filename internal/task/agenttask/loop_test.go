package agenttask

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

// drivePipeline runs handler.Run repeatedly (as the manager would on each
// reschedule) until a terminal outcome, feeding the checkpointed payload
// forward. Returns the final outcome and last payload.
func drivePipeline(t *testing.T, handler *Handler, start Payload, maxRuns int) (task.OutcomeKind, Payload) {
	t.Helper()
	payload := start
	for i := 0; i < maxRuns; i++ {
		ctl := &fakeController{}
		res, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, payload)}, ctl)
		if err != nil {
			t.Fatalf("run %d error: %v", i, err)
		}
		if len(ctl.payload) > 0 {
			payload = ctl.decodedPayload(t)
		}
		if res.Outcome != task.OutcomeReschedule {
			return res.Outcome, payload
		}
	}
	t.Fatalf("pipeline did not terminate within %d runs", maxRuns)
	return "", payload
}

// The canonical structured loop: test(shell) → fix(agent, when test.failed),
// repeat until test.succeeded. The loop condition is a deterministic shell
// exit code, not agent judgment.
func TestHandler_TestFixRepeatLoop(t *testing.T) {
	workspace := mustWorkspace(t)
	marker := filepath.Join(workspace, "fixed")
	fixCalls := 0
	agent := &fakeAgent{available: true, execute: func(_ context.Context, prompt string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		fixCalls++
		if !strings.Contains(prompt, "Fix the failure") {
			t.Fatalf("unexpected agent prompt: %q", prompt)
		}
		// The agent "fixes" the problem so the next test iteration passes.
		if err := os.WriteFile(marker, []byte("ok"), 0o644); err != nil {
			t.Fatal(err)
		}
		return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"fixed"}</task_outcome>`}, nil, nil), nil
	}}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)

	payload := Payload{
		Version: 2, Goal: "Green tests", Agent: AgentClaude, WorkspacePath: workspace,
		Execution: DefaultExecutionPolicy(AgentClaude),
		Steps: []Step{
			{ID: "test", Title: "Test", Executor: StepExecutorShell, Command: "test -f fixed"},
			{ID: "fix", Title: "Fix", Instruction: "Fix the failure", When: "steps.test.failed"},
		},
		Repeat: &RepeatPolicy{Until: "steps.test.succeeded", Max: 3},
	}

	outcome, final := drivePipeline(t, handler, payload, 12)
	if outcome != task.OutcomeComplete {
		t.Fatalf("loop should converge to complete, got %s", outcome)
	}
	if fixCalls != 1 {
		t.Fatalf("fix should run exactly once (iteration 0 only), got %d", fixCalls)
	}
	if final.Repeat.Iteration != 1 {
		t.Fatalf("should converge on iteration 1, got %d", final.Repeat.Iteration)
	}
}

// A loop that never converges pauses for a human at the iteration cap.
func TestHandler_RepeatExhaustedPausesForInput(t *testing.T) {
	workspace := mustWorkspace(t)
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: &fakeAgent{available: true}}, nil)
	payload := Payload{
		Version: 2, Goal: "Never green", Agent: AgentClaude, WorkspacePath: workspace,
		Execution: DefaultExecutionPolicy(AgentClaude),
		Steps:     []Step{{ID: "test", Title: "Test", Executor: StepExecutorShell, Command: "exit 1"}},
		Repeat:    &RepeatPolicy{Until: "steps.test.succeeded", Max: 2},
	}
	outcome, final := drivePipeline(t, handler, payload, 8)
	if outcome != task.OutcomeNeedsInput {
		t.Fatalf("exhausted loop should pause for input, got %s", outcome)
	}
	if final.Repeat.Iteration != 1 {
		t.Fatalf("should stop at max-1 iteration index, got %d", final.Repeat.Iteration)
	}
}

func TestEvalCondition(t *testing.T) {
	code0, code1 := 0, 1
	outcomes := map[string]StepOutcome{
		"a": {StepID: "a", Result: Result{State: "done", ExitCode: &code0}},
		"b": {StepID: "b", Result: Result{State: "failed", ExitCode: &code1}},
		"c": {StepID: "c", Result: Result{State: "skipped"}},
	}
	cases := []struct {
		expr string
		want bool
	}{
		{"", true},
		{"steps.a.succeeded", true},
		{"steps.a.failed", false},
		{"steps.b.failed", true},
		{"steps.b.succeeded", false},
		{"steps.c.skipped", true},
		{"steps.a.succeeded && steps.b.failed", true},
		{"steps.a.succeeded && steps.a.failed", false},
		{"steps.missing.succeeded", false},
	}
	for _, tc := range cases {
		got, err := evalCondition(tc.expr, outcomes)
		if err != nil {
			t.Fatalf("%q: unexpected error %v", tc.expr, err)
		}
		if got != tc.want {
			t.Fatalf("%q: got %v want %v", tc.expr, got, tc.want)
		}
	}
	if _, err := evalCondition("steps.a", outcomes); err == nil {
		t.Fatal("malformed term should error (fail closed)")
	}
	if _, err := evalCondition("steps.a.bogus", outcomes); err == nil {
		t.Fatal("unknown predicate should error")
	}
}
