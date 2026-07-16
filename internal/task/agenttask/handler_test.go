package agenttask

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/internal/task"
)

type fakeController struct {
	mu       sync.Mutex
	payload  json.RawMessage
	progress string
	runID    string
	events   []task.RunEvent
}

func (c *fakeController) UpdateProgress(_ context.Context, text string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.progress = text
	return nil
}

func (c *fakeController) UpdatePayload(_ context.Context, payload json.RawMessage) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.payload = append(json.RawMessage(nil), payload...)
	return nil
}

func (c *fakeController) IsCancelled(ctx context.Context) bool { return ctx.Err() != nil }

func (c *fakeController) RunID() string {
	if c.runID != "" {
		return c.runID
	}
	return "run-1"
}

func (c *fakeController) AppendRunEvent(_ context.Context, event task.RunEvent) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.events = append(c.events, event)
	return nil
}

func (c *fakeController) decodedPayload(t *testing.T) Payload {
	t.Helper()
	c.mu.Lock()
	defer c.mu.Unlock()
	var payload Payload
	if err := json.Unmarshal(c.payload, &payload); err != nil {
		t.Fatalf("decode checkpointed payload: %v (%s)", err, c.payload)
	}
	return payload
}

type fakeAgent struct {
	available bool
	execute   func(context.Context, string, agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error)
}

func (a *fakeAgent) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
	return a.execute(ctx, prompt, opts)
}
func (a *fakeAgent) IsAvailable() bool                       { return a.available }
func (a *fakeAgent) Type() agentboot.AgentType               { return agentboot.AgentTypeClaude }
func (a *fakeAgent) SetDefaultFormat(agentboot.OutputFormat) {}
func (a *fakeAgent) GetDefaultFormat() agentboot.OutputFormat {
	return agentboot.OutputFormatStreamJSON
}

type responseCall struct {
	id   string
	resp agentboot.ControlResponse
}

type fakeSessionMessage string

func (m fakeSessionMessage) GetSessionID() string { return string(m) }

func controlledHandle(events []agentboot.StreamEvent, result *agentboot.Result, waitErr error, responses chan<- responseCall) agentboot.ExecutionHandle {
	controlHandled := make(chan struct{})
	var controlOnce sync.Once
	handle, controls := agentboot.NewControlledHandle(
		8,
		func(id string, resp agentboot.ControlResponse) error {
			if responses != nil {
				responses <- responseCall{id: id, resp: resp}
			}
			controlOnce.Do(func() { close(controlHandled) })
			return nil
		},
		func() {},
		func() (*agentboot.Result, error) { return result, waitErr },
	)
	go func() {
		ctx := context.Background()
		hasControl := false
		for _, event := range events {
			switch event.(type) {
			case agentboot.AskRequestEvent, agentboot.ApprovalRequestEvent:
				hasControl = true
			}
			controls.Emit(ctx, event)
		}
		if hasControl {
			<-controlHandled
		}
		controls.Close()
	}()
	return handle
}

func mustWorkspace(t *testing.T) string {
	t.Helper()
	workspace, err := CreateWorkspace(t.TempDir(), uuid.NewString())
	if err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
	return workspace
}

func rawPayload(t *testing.T, payload Payload) json.RawMessage {
	t.Helper()
	payload.ApplyDefaults()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHandler_ClaudeDoneCreatesSession(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		if prompt != "finish the task" {
			t.Fatalf("prompt = %q", prompt)
		}
		if opts.SessionID == "" || opts.Resume {
			t.Fatalf("new session options: id=%q resume=%v", opts.SessionID, opts.Resume)
		}
		if opts.ProjectPath != workspace || !strings.Contains(opts.AppendSystemPrompt, outcomeOpenTag) {
			t.Fatalf("execution options not wired: %+v", opts)
		}
		if opts.PermissionPromptTool != "" || opts.PermissionMode != "acceptEdits" || len(opts.AvailableTools) != 5 || len(opts.AllowedTools) != 5 || opts.AvailableTools[4] != "Edit" {
			t.Fatalf("execution policy not wired: %+v", opts)
		}
		output := `finished
<task_outcome>{"state":"done","summary":"all good","artifacts":["report.md","../secret","/etc/passwd"]}</task_outcome>`
		return controlledHandle(nil, &agentboot.Result{Format: agentboot.OutputFormatText, Output: output}, nil, nil), nil
	}

	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	controller := &fakeController{}
	taskResult, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "finish the task",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if taskResult.Outcome != task.OutcomeComplete {
		t.Fatalf("outcome = %s", taskResult.Outcome)
	}
	var result Result
	if err := json.Unmarshal(taskResult.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.State != "done" || result.Summary != "all good" || result.NativeSessionID == "" {
		t.Fatalf("normalized result = %+v", result)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0] != "report.md" {
		t.Fatalf("unsafe artifacts were not filtered: %v", result.Artifacts)
	}
	checkpoint := controller.decodedPayload(t)
	if checkpoint.SessionID != result.NativeSessionID || checkpoint.WakeCount != 1 {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

func TestHandler_ResumeClearsPendingInputAndReschedules(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		if !strings.HasPrefix(prompt, "watch the build\n\n") || !strings.Contains(prompt, "Additional instruction for this run:\nthe build is ready") || !opts.Resume || opts.SessionID != "session-1" {
			t.Fatalf("resume call: prompt=%q opts=%+v", prompt, opts)
		}
		output := `<task_outcome>{"state":"continue","summary":"still checking","suggested_delay_seconds":1}</task_outcome>`
		return controlledHandle(nil, &agentboot.Result{
			Format:   agentboot.OutputFormatText,
			Output:   output,
			Metadata: map[string]any{"session_id": "session-1"},
		}, nil, nil), nil
	}

	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	fixedNow := time.Date(2026, time.July, 15, 12, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return fixedNow }
	controller := &fakeController{}
	taskResult, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "watch the build",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
			SessionID:     "session-1",
			PendingInput:  "the build is ready",
			FollowUp: FollowUpPolicy{
				Enabled:      true,
				DelaySeconds: 300,
				MaxWakeUps:   5,
			},
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if taskResult.Outcome != task.OutcomeReschedule || taskResult.NextRunAt == nil {
		t.Fatalf("task result = %+v", taskResult)
	}
	if want := fixedNow.Add(time.Minute); !taskResult.NextRunAt.Equal(want) {
		t.Fatalf("minimum delay: want %s, got %s", want, taskResult.NextRunAt)
	}
	checkpoint := controller.decodedPayload(t)
	if checkpoint.PendingInput != "" || checkpoint.WakeCount != 1 {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

func TestNextPrompt_PreservesGoalAsExactPrefix(t *testing.T) {
	goal := "# Updated goal\n\nKeep **this formatting** exactly."
	tests := []struct {
		name      string
		payload   Payload
		resume    bool
		exactGoal bool
		want      []string
	}{
		{name: "first run", payload: Payload{Goal: goal}, exactGoal: true},
		{name: "resume", payload: Payload{Goal: goal}, resume: true, want: []string{"Continue working"}},
		{name: "instruction", payload: Payload{Goal: goal, PendingInput: "focus on tests"}, resume: true, want: []string{"Additional instruction for this run:\nfocus on tests"}},
		{name: "step", payload: Payload{Goal: goal, Steps: []Step{{Title: "Deploy", Instruction: "Deploy the build"}}}, want: []string{"Current step 1 of 1", "Deploy the build"}},
		{name: "step instruction", payload: Payload{Goal: goal, PendingInput: "use staging", Steps: []Step{{Title: "Deploy", Instruction: "Deploy the build"}}}, resume: true, want: []string{"Current step 1 of 1", "User instruction for this step:\nuse staging"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			prompt := nextPrompt(test.payload, test.resume)
			if test.exactGoal {
				if prompt != goal {
					t.Fatalf("first-run prompt changed goal: %q", prompt)
				}
				return
			}
			if !strings.HasPrefix(prompt, goal+"\n\n") {
				t.Fatalf("prompt does not preserve goal as exact prefix: %q", prompt)
			}
			for _, want := range test.want {
				if !strings.Contains(prompt, want) {
					t.Fatalf("prompt %q does not contain %q", prompt, want)
				}
			}
		})
	}
}

func TestHandler_CodexCheckpointsThreadStarted(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, _ string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		if opts.SessionID != "" || opts.Resume {
			t.Fatalf("new Codex thread must not preselect an id: %+v", opts)
		}
		if opts.SandboxMode != "workspace-write" {
			t.Fatalf("Codex sandbox = %q", opts.SandboxMode)
		}
		events := []agentboot.StreamEvent{
			agentboot.MessageEvent{Raw: fakeSessionMessage("thread-1")},
		}
		return controlledHandle(events, &agentboot.Result{
			Format:   agentboot.OutputFormatText,
			Output:   `{"state":"done","summary":"complete"}`,
			Metadata: map[string]any{"session_id": "thread-1"},
		}, nil, nil), nil
	}

	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentCodex: agent}, nil)
	controller := &fakeController{}
	taskResult, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "inspect the repository",
			Agent:         AgentCodex,
			WorkspacePath: workspace,
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if taskResult.Outcome != task.OutcomeComplete {
		t.Fatalf("outcome = %s", taskResult.Outcome)
	}
	checkpoint := controller.decodedPayload(t)
	if checkpoint.SessionID != "thread-1" || checkpoint.WakeCount != 1 {
		t.Fatalf("checkpoint = %+v", checkpoint)
	}
}

func TestHandler_AskStopsRunForNewInput(t *testing.T) {
	workspace := mustWorkspace(t)
	responses := make(chan responseCall, 1)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		events := []agentboot.StreamEvent{agentboot.AskRequestEvent{
			ID:      "ask-1",
			Message: "Which environment?",
		}}
		return controlledHandle(events, &agentboot.Result{
			Output: `<task_outcome>{"state":"done","summary":"deployed to staging"}</task_outcome>`,
		}, nil, responses), nil
	}

	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	controller := &fakeController{}
	taskResult, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "deploy",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if taskResult.Outcome != task.OutcomeNeedsInput || !strings.Contains(string(taskResult.Result), "Which environment?") {
		t.Fatalf("outcome = %s", taskResult.Outcome)
	}
	select {
	case call := <-responses:
		response, ok := call.resp.(agentboot.AskResponse)
		if call.id != "ask-1" || !ok || response.Approved {
			t.Fatalf("response = %+v", call)
		}
	case <-time.After(time.Second):
		t.Fatal("handler did not deliver answer")
	}
}

func TestHandler_ApprovalRequiresNativeHandoff(t *testing.T) {
	workspace := mustWorkspace(t)
	responses := make(chan responseCall, 1)
	agent := &fakeAgent{available: true, execute: func(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		return controlledHandle([]agentboot.StreamEvent{agentboot.ApprovalRequestEvent{
			ID: "approval-1", ToolName: "Bash", Input: map[string]any{"command": "go test ./..."},
		}}, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"tests passed"}</task_outcome>`}, nil, responses), nil
	}}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	controller := &fakeController{}

	result, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, Payload{
		Goal: "test", Agent: AgentClaude, WorkspacePath: workspace,
	})}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeHandoff || !strings.Contains(string(result.Result), "Bash") {
		t.Fatalf("outcome = %s", result.Outcome)
	}
	call := <-responses
	response, ok := call.resp.(agentboot.ApprovalResponse)
	if !ok || response.Approved || call.id != "approval-1" {
		t.Fatalf("response = %+v", call)
	}
	foundHandoff := false
	for _, event := range controller.events {
		foundHandoff = foundHandoff || event.Kind == "handoff_required"
	}
	if !foundHandoff {
		t.Fatalf("events = %+v", controller.events)
	}
}

func TestHandler_MissingEnvelopePausesForInput(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		return controlledHandle(nil, &agentboot.Result{Format: agentboot.OutputFormatText, Output: "plain final answer"}, nil, nil), nil
	}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	result, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "answer",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
		}),
	}, &fakeController{})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeNeedsInput || !strings.Contains(string(result.Result), "plain final answer") || !strings.Contains(string(result.Result), "outcome_unreported") {
		t.Fatalf("fallback result = %+v", result)
	}
}

func TestHandler_UnavailableAgentFails(t *testing.T) {
	workspace := mustWorkspace(t)
	handler := NewHandler(map[AgentKind]agentboot.Agent{
		AgentClaude: &fakeAgent{available: false},
	}, nil)
	_, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "answer",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
		}),
	}, &fakeController{})
	if err == nil || !strings.Contains(err.Error(), "not available") {
		t.Fatalf("want unavailable error, got %v", err)
	}
}

func TestHandler_SequentialStepsAdvanceOneRunAtATime(t *testing.T) {
	workspace := mustWorkspace(t)
	calls := 0
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		calls++
		switch calls {
		case 1:
			if !strings.Contains(prompt, "Current step 1 of 2") || !strings.Contains(prompt, "Inspect the build") || strings.Contains(prompt, "Publish artifacts") {
				t.Fatalf("first step prompt = %q", prompt)
			}
			if opts.Resume {
				t.Fatal("first step unexpectedly resumed")
			}
			return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"build inspected"}</task_outcome>`}, nil, nil), nil
		case 2:
			if !strings.Contains(prompt, "Current step 2 of 2") || !strings.Contains(prompt, "Publish artifacts") {
				t.Fatalf("second step prompt = %q", prompt)
			}
			if opts.Resume {
				t.Fatal("second step must start a fresh session, not resume the first step's")
			}
			if !strings.Contains(prompt, "Completed steps so far") || !strings.Contains(prompt, "build inspected") {
				t.Fatalf("second step prompt lacks prior-step context: %q", prompt)
			}
			return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"artifacts published"}</task_outcome>`}, nil, nil), nil
		default:
			t.Fatalf("unexpected execution %d", calls)
			return nil, nil
		}
	}

	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	fixedNow := time.Date(2026, time.July, 15, 15, 0, 0, 0, time.UTC)
	handler.now = func() time.Time { return fixedNow }
	payload := Payload{
		Goal: "Ship the release", Agent: AgentClaude, WorkspacePath: workspace,
		Steps: []Step{
			{ID: "step-1", Title: "Inspect", Instruction: "Inspect the build"},
			{ID: "step-2", Title: "Publish", Instruction: "Publish artifacts"},
		},
	}

	firstController := &fakeController{}
	first, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, payload)}, firstController)
	if err != nil {
		t.Fatal(err)
	}
	if first.Outcome != task.OutcomeReschedule || first.NextRunAt == nil || !first.NextRunAt.Equal(fixedNow) {
		t.Fatalf("first result = %+v", first)
	}
	checkpoint := firstController.decodedPayload(t)
	if checkpoint.CurrentStep != 1 || len(checkpoint.StepOutcomes) != 1 || checkpoint.StepOutcomes[0].Result.Summary != "build inspected" {
		t.Fatalf("first checkpoint = %+v", checkpoint)
	}

	secondController := &fakeController{}
	second, err := handler.Run(context.Background(), &task.Task{ID: uuid.NewString(), Type: TaskType, Payload: rawPayload(t, checkpoint)}, secondController)
	if err != nil {
		t.Fatal(err)
	}
	if second.Outcome != task.OutcomeComplete {
		t.Fatalf("second result = %+v", second)
	}
	checkpoint = secondController.decodedPayload(t)
	if checkpoint.CurrentStep != 2 || len(checkpoint.StepOutcomes) != 2 || checkpoint.StepOutcomes[1].Result.Summary != "artifacts published" {
		t.Fatalf("final checkpoint = %+v", checkpoint)
	}
}

func TestHandler_SequentialContinueWithoutFollowUpPausesCurrentStep(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true, execute: func(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"continue","summary":"more work remains"}</task_outcome>`}, nil, nil), nil
	}}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	controller := &fakeController{}
	result, err := handler.Run(context.Background(), &task.Task{
		ID: uuid.NewString(), Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal: "Ship", Agent: AgentClaude, WorkspacePath: workspace,
			Steps: []Step{{ID: "step-1", Title: "Inspect", Instruction: "Inspect the build"}},
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeNeedsInput || !strings.Contains(string(result.Result), "follow_up_disabled") {
		t.Fatalf("result = %+v", result)
	}
	checkpoint := controller.decodedPayload(t)
	if checkpoint.CurrentStep != 0 || len(checkpoint.StepOutcomes) != 0 {
		t.Fatalf("checkpoint advanced unexpectedly: %+v", checkpoint)
	}
}

func TestHandler_CompletedSequenceRestartsFromFirstStep(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true, execute: func(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		if !strings.Contains(prompt, "Current step 1 of 1") || opts.Resume {
			t.Fatalf("restart call must use a fresh session: prompt=%q opts=%+v", prompt, opts)
		}
		return controlledHandle(nil, &agentboot.Result{Output: `<task_outcome>{"state":"done","summary":"new result"}</task_outcome>`}, nil, nil), nil
	}}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	controller := &fakeController{}
	step := Step{ID: "step-1", Title: "Inspect", Instruction: "Inspect the build"}
	result, err := handler.Run(context.Background(), &task.Task{
		ID: uuid.NewString(), Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal: "Ship", Agent: AgentClaude, WorkspacePath: workspace, SessionID: "session-1",
			Steps: []Step{step}, CurrentStep: 1,
			StepOutcomes: []StepOutcome{{StepID: step.ID, Result: Result{State: "done", Summary: "old result"}}},
		}),
	}, controller)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != task.OutcomeComplete {
		t.Fatalf("result = %+v", result)
	}
	checkpoint := controller.decodedPayload(t)
	if checkpoint.CurrentStep != 1 || len(checkpoint.StepOutcomes) != 1 || checkpoint.StepOutcomes[0].Result.Summary != "new result" {
		t.Fatalf("restart checkpoint = %+v", checkpoint)
	}
}

func TestHandler_StartFailureDoesNotCheckpointNewSession(t *testing.T) {
	workspace := mustWorkspace(t)
	agent := &fakeAgent{available: true}
	agent.execute = func(_ context.Context, _ string, _ agentboot.ExecutionOptions) (agentboot.ExecutionHandle, error) {
		return nil, errors.New("spawn failed")
	}
	controller := &fakeController{}
	handler := NewHandler(map[AgentKind]agentboot.Agent{AgentClaude: agent}, nil)
	_, err := handler.Run(context.Background(), &task.Task{
		ID:   uuid.NewString(),
		Type: TaskType,
		Payload: rawPayload(t, Payload{
			Goal:          "answer",
			Agent:         AgentClaude,
			WorkspacePath: workspace,
		}),
	}, controller)
	if err == nil || !strings.Contains(err.Error(), "spawn failed") {
		t.Fatalf("want spawn error, got %v", err)
	}
	controller.mu.Lock()
	defer controller.mu.Unlock()
	if len(controller.payload) != 0 {
		t.Fatalf("new session was checkpointed before process start: %s", controller.payload)
	}
}

func TestParseOutcome_InvalidTagPausesForInput(t *testing.T) {
	result := parseOutcome(`prefix <task_outcome>{bad}</task_outcome>`)
	if result.State != "needs_input" || result.ExitReason != "outcome_unreported" || !strings.Contains(result.Summary, "{bad}") {
		t.Fatalf("result = %+v", result)
	}
}
