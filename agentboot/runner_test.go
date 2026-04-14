package agentboot_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	mock "github.com/tingly-dev/tingly-box/agentboot/mockagent"
)

// --- helpers ----------------------------------------------------------------

// testHandler captures everything a MessageHandler receives.
type testHandler struct {
	mu          sync.Mutex
	messages    []interface{}
	errors      []error
	completions []*agentboot.CompletionResult
	approvals   []agentboot.PermissionRequest
	asks        []agentboot.AskRequest
}

func (h *testHandler) OnMessage(msg interface{}) error {
	h.mu.Lock()
	h.messages = append(h.messages, msg)
	h.mu.Unlock()
	return nil
}

func (h *testHandler) OnError(err error) {
	h.mu.Lock()
	h.errors = append(h.errors, err)
	h.mu.Unlock()
}

func (h *testHandler) OnComplete(r *agentboot.CompletionResult) {
	h.mu.Lock()
	h.completions = append(h.completions, r)
	h.mu.Unlock()
}

func (h *testHandler) OnApproval(_ context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	h.mu.Lock()
	h.approvals = append(h.approvals, req)
	h.mu.Unlock()
	return agentboot.PermissionResult{Approved: true, UpdatedInput: req.Input}, nil
}

func (h *testHandler) OnAsk(_ context.Context, req agentboot.AskRequest) (agentboot.AskResult, error) {
	h.mu.Lock()
	h.asks = append(h.asks, req)
	h.mu.Unlock()
	return agentboot.AskResult{ID: req.ID, Approved: true}, nil
}

func (h *testHandler) messageCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.messages)
}

func (h *testHandler) completed() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.completions) > 0
}

// newMockAgent returns a mock.Agent configured for fast, deterministic tests.
func newMockAgent(iters int, autoApprove bool) *mock.Agent {
	return mock.NewAgent(mock.Config{
		MaxIterations: iters,
		StepDelay:     0,
		AutoApprove:   autoApprove,
	})
}

// --- AgentBoot registry tests -----------------------------------------------

func TestAgentBoot_RegisterAndGet(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	agent := newMockAgent(1, true)
	ab.RegisterAgent(agentboot.AgentTypeMockAgent, agent)

	got, err := ab.GetAgent(agentboot.AgentTypeMockAgent)
	require.NoError(t, err)
	assert.Equal(t, agentboot.AgentTypeMockAgent, got.Type())
}

func TestAgentBoot_GetUnregistered(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	_, err = ab.GetAgent("nonexistent")
	assert.Error(t, err)
}

func TestAgentBoot_ResumeSession(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	opts := ab.ResumeSession("sess-abc")
	assert.Equal(t, "sess-abc", opts.SessionID)
	assert.True(t, opts.Resume)
}

// --- MockAgent Execute (collect path) ---------------------------------------

func TestMockAgent_Execute_CollectsResult(t *testing.T) {
	agent := newMockAgent(2, true)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := agent.Execute(ctx, "hello", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsSuccess())
	assert.NotEmpty(t, result.Events, "should have at least one event")
}

func TestMockAgent_Execute_HasInitAndResultEvents(t *testing.T) {
	agent := newMockAgent(1, true)

	ctx := context.Background()
	result, err := agent.Execute(ctx, "test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})

	require.NoError(t, err)
	require.NotNil(t, result)

	types := make(map[string]bool)
	for _, e := range result.Events {
		types[e.Type] = true
	}
	assert.True(t, types["init"], "should have init event")
	assert.True(t, types["result"], "should have result event")
}

// --- MockAgent Execute (handler path) ---------------------------------------

// TestMockAgent_Execute_StreamsToHandler verifies that the handler receives
// messages and OnComplete is called. MockAgent returns a non-nil Result even
// in the handler path (it fills both paths internally).
func TestMockAgent_Execute_StreamsToHandler(t *testing.T) {
	agent := newMockAgent(3, true)
	h := &testHandler{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "stream test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})

	require.NoError(t, err)
	assert.True(t, h.completed(), "OnComplete should be called")
	assert.Greater(t, h.messageCount(), 0, "should receive messages")
}

func TestMockAgent_Execute_HandlerReceivesApprovals(t *testing.T) {
	// AutoApprove=false means mock agent calls OnApproval.
	agent := newMockAgent(2, false)
	h := &testHandler{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "approval test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})

	require.NoError(t, err)
	h.mu.Lock()
	approvalCount := len(h.approvals)
	h.mu.Unlock()
	assert.Greater(t, approvalCount, 0, "should have received permission requests")
}

func TestMockAgent_Execute_DenyApproval(t *testing.T) {
	denied := 0
	var mu sync.Mutex

	h := agentboot.NewCompositeHandler().SetApprovalHandler(&denyApprovalHandler{
		fn: func(req agentboot.PermissionRequest) {
			mu.Lock()
			denied++
			mu.Unlock()
		},
	})

	agent := newMockAgent(2, false)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, _ = agent.Execute(ctx, "deny test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})

	mu.Lock()
	n := denied
	mu.Unlock()
	assert.Greater(t, n, 0, "should have denied at least one permission")
}

// --- MockAgent with AskUserQuestion -----------------------------------------

func TestMockAgent_Execute_AskUserQuestion(t *testing.T) {
	agent := mock.NewAgent(mock.Config{
		MaxIterations: 4,
		StepDelay:     50 * time.Millisecond,
		AutoApprove:   false,
	})
	// Set frequency after construction to work around Merge not propagating it.
	agent.SetAskUserQuestionFrequency(2)

	h := &testHandler{}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "ask test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})
	require.NoError(t, err)

	h.mu.Lock()
	askCount := len(h.asks)
	h.mu.Unlock()
	assert.Greater(t, askCount, 0, "should have received ask requests")
}

// --- CompositeHandler wiring ------------------------------------------------

func TestCompositeHandler_DefaultAutoApproves(t *testing.T) {
	h := agentboot.NewCompositeHandler()

	result, err := h.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "bash"})
	require.NoError(t, err)
	assert.True(t, result.Approved)

	askResult, err := h.OnAsk(context.Background(), agentboot.AskRequest{ID: "x"})
	require.NoError(t, err)
	assert.True(t, askResult.Approved)
}

func TestCompositeHandler_WithCompletionFunc(t *testing.T) {
	called := false
	h := agentboot.NewCompositeHandler().
		WithCompletionFunc(func(r *agentboot.CompletionResult) { called = true })

	h.OnComplete(&agentboot.CompletionResult{Success: true})
	assert.True(t, called)
}

// --- Context cancellation ---------------------------------------------------

func TestMockAgent_Execute_ContextCancel(t *testing.T) {
	agent := newMockAgent(100, true) // many iterations

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately.
	cancel()

	result, err := agent.Execute(ctx, "cancel test", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})

	// Either context error or a cancelled result — should not hang.
	_ = err
	_ = result
}

// --- AgentBoot.SetDefaultAgent ----------------------------------------------

func TestAgentBoot_SetDefaultAgent(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	agent := newMockAgent(1, true)
	ab.RegisterAgent(agentboot.AgentTypeMockAgent, agent)

	err = ab.SetDefaultAgent(agentboot.AgentTypeMockAgent)
	require.NoError(t, err)

	got, err := ab.GetDefaultAgent()
	require.NoError(t, err)
	assert.Equal(t, agentboot.AgentTypeMockAgent, got.Type())
}

func TestAgentBoot_SetDefaultAgent_Unregistered(t *testing.T) {
	ab, err := agentboot.New(agentboot.Config{})
	require.NoError(t, err)

	err = ab.SetDefaultAgent("ghost")
	assert.Error(t, err)
}

// --- helpers ----------------------------------------------------------------

// denyApprovalHandler implements ApprovalHandler, denying every request.
type denyApprovalHandler struct {
	fn func(agentboot.PermissionRequest)
}

func (d *denyApprovalHandler) OnApproval(_ context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	if d.fn != nil {
		d.fn(req)
	}
	return agentboot.PermissionResult{Approved: false, Reason: "denied by test"}, nil
}
