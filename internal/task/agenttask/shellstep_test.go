package agenttask

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

// A heterogeneous pipeline: a shell step (no agent) then an agent step.
func TestHandler_ShellStepRunsWithoutAgent(t *testing.T) {
	workspace := mustWorkspace(t)
	calls := 0
	agent := &fakeAgent{available: true, execute: func(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		calls++
		if !strings.Contains(prompt, "Current step 2 of 2") {
			t.Fatalf("agent should only run step 2, got prompt=%q", prompt)
		}
		return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"reported"}</task_outcome>`}, nil, nil), nil
	}}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)

	payload := Payload{
		Version: 2, Goal: "Ship", Agent: AgentClaude, WorkspacePath: workspace,
		Execution: DefaultExecutionPolicy(AgentClaude),
		Steps: []Step{
			{ID: "s1", Title: "Test", Executor: StepExecutorShell, Command: "echo passing"},
			{ID: "s2", Title: "Report", Instruction: "write the report"},
		},
	}

	// Step 1 (shell) runs with NO agent registered available concern.
	c1 := &fakeController{}
	r1, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, payload)}, c1)
	if err != nil {
		t.Fatal(err)
	}
	if r1.Outcome != task.OutcomeReschedule {
		t.Fatalf("shell step should reschedule to next step, got %+v", r1)
	}
	if calls != 0 {
		t.Fatalf("agent must not run for a shell step, calls=%d", calls)
	}
	cp := c1.decodedPayload(t)
	if cp.CurrentStep != 1 || len(cp.StepOutcomes) != 1 || cp.StepOutcomes[0].Result.ExitCode == nil || *cp.StepOutcomes[0].Result.ExitCode != 0 {
		t.Fatalf("shell outcome not recorded with exit code: %+v", cp)
	}

	// Step 2 (agent) now runs.
	c2 := &fakeController{}
	r2, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, cp)}, c2)
	if err != nil {
		t.Fatal(err)
	}
	if r2.Outcome != task.OutcomeComplete || calls != 1 {
		t.Fatalf("agent step should complete the pipeline: r=%+v calls=%d", r2, calls)
	}
}

func TestHandler_ShellStepFailureStopsPipeline(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	payload := Payload{
		Version: 2, Goal: "Ship", Agent: AgentClaude, WorkspacePath: workspace,
		Execution: DefaultExecutionPolicy(AgentClaude),
		Steps:     []Step{{ID: "s1", Title: "Test", Executor: StepExecutorShell, Command: "exit 7"}},
	}
	_, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, payload)}, &fakeController{})
	if err == nil || !strings.Contains(err.Error(), "step failed") {
		t.Fatalf("failed shell step should fail the task, got %v", err)
	}
}
