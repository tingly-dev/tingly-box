//go:build e2e
// +build e2e

package claude_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// newAgent returns a Claude agent for e2e testing.
func newAgent() *claude.Agent {
	return claude.NewAgentWithConfig(claude.DefaultConfig())
}

// --- Availability guard -----------------------------------------------------

func TestClaudeAgent_IsAvailable(t *testing.T) {
	agent := newAgent()
	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available, skipping e2e tests")
	}
	assert.Equal(t, agentboot.AgentTypeClaude, agent.Type())
}

// --- Stream-JSON execution (collect path) -----------------------------------

func TestClaudeAgent_Execute_StreamJSON(t *testing.T) {
	agent := newAgent()
	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := agent.Execute(ctx, "Reply with exactly: hello", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsSuccess())
	assert.NotEmpty(t, result.Events, "should have events")
}

// --- Init and result events -------------------------------------------------

func TestClaudeAgent_Execute_HasInitAndResultEvents(t *testing.T) {
	agent := newAgent()
	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := agent.Execute(ctx, "Say hi", agentboot.ExecutionOptions{
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

// --- Handler streaming path -------------------------------------------------

func TestClaudeAgent_Execute_StreamsToHandler(t *testing.T) {
	agent := newAgent()
	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available")
	}

	msgCh := make(chan interface{}, 100)
	doneCh := make(chan *agentboot.CompletionResult, 1)

	h := agentboot.NewCompositeHandler().
		WithMessageFunc(func(msg interface{}) error {
			msgCh <- msg
			return nil
		}).
		WithCompletionFunc(func(r *agentboot.CompletionResult) {
			doneCh <- r
		})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "Reply with: streaming works", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})

	require.NoError(t, err)

	select {
	case result := <-doneCh:
		assert.True(t, result.Success, "should complete successfully")
	case <-time.After(10 * time.Second):
		t.Fatal("OnComplete not called within timeout")
	}

	assert.Greater(t, len(msgCh), 0, "should have received messages")
}

// --- Permission/approval via stdio control protocol -------------------------

func TestClaudeAgent_Execute_PermissionApproval(t *testing.T) {
	cfg := claude.DefaultConfig()
	cfg.PermissionMode = claude.PermissionModeDefault
	agent := claude.NewAgentWithConfig(cfg)

	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available")
	}

	approvals := 0
	h := agentboot.NewCompositeHandler().
		SetApprovalHandler(&countingApprovalHandler{
			count: &approvals,
		})

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "Run the bash command: echo hello", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
		Handler:      h,
	})

	// May succeed or error depending on what Claude decides to do.
	_ = err
	// We can't guarantee a permission was triggered, but the test verifies the
	// plumbing doesn't panic or deadlock.
}

// --- Timeout ----------------------------------------------------------------

func TestClaudeAgent_Execute_Timeout(t *testing.T) {
	agent := newAgent()
	if !agent.IsAvailable() {
		t.Skip("Claude CLI not available")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	result, err := agent.Execute(ctx, "Tell me a very long story", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})

	// Should not hang — either an error or a cancelled result.
	_ = result
	_ = err
}

// --- CLI unavailable --------------------------------------------------------

func TestClaudeAgent_Execute_CLIUnavailable(t *testing.T) {
	cfg := claude.DefaultConfig()
	agent := claude.NewAgentWithConfig(cfg)
	agent.SetCLIPath("/nonexistent/path/to/claude")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	_, err := agent.Execute(ctx, "hello", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	assert.Error(t, err)
}

// --- DefaultFormat ----------------------------------------------------------

func TestClaudeAgent_DefaultFormat(t *testing.T) {
	agent := newAgent()

	agent.SetDefaultFormat(agentboot.OutputFormatStreamJSON)
	assert.Equal(t, agentboot.OutputFormatStreamJSON, agent.GetDefaultFormat())

	agent.SetDefaultFormat(agentboot.OutputFormatText)
	assert.Equal(t, agentboot.OutputFormatText, agent.GetDefaultFormat())
}

// --- helpers ----------------------------------------------------------------

type countingApprovalHandler struct {
	count *int
}

func (h *countingApprovalHandler) OnApproval(_ context.Context, req agentboot.PermissionRequest) (agentboot.PermissionResult, error) {
	*h.count++
	return agentboot.PermissionResult{Approved: true, UpdatedInput: req.Input}, nil
}
