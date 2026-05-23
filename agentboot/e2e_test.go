//go:build e2e
// +build e2e

package agentboot_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
)

// e2eAutoApprove approves everything so the run is non-interactive.
type e2eAutoApprove struct{}

func (e2eAutoApprove) OnApproval(context.Context, agentboot.ApprovalRequestEvent) (agentboot.ApprovalResponse, error) {
	return agentboot.ApprovalResponse{Approved: true}, nil
}

func (e2eAutoApprove) OnAsk(context.Context, agentboot.AskRequestEvent) (agentboot.AskResponse, error) {
	return agentboot.AskResponse{Approved: true}, nil
}

// TestE2E_ClaudeRun drives the full pipeline — AgentService → Execute → Runner
// → process → protocol → transport → ExecutionHandle → RunWithPrompter →
// Result — against the real Claude CLI.
//
// Run with: go test -tags e2e ./...   (requires the `claude` CLI on PATH and
// valid credentials; skips otherwise).
func TestE2E_ClaudeRun(t *testing.T) {
	agent := claude.NewAgent(agentboot.DefaultConfig())
	if !agent.IsAvailable() {
		t.Skip("claude CLI not available; skipping e2e")
	}

	svc, err := agentboot.NewAgentService(agentboot.DefaultConfig())
	require.NoError(t, err)
	svc.RegisterAgent(agentboot.AgentTypeClaude, agent)

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	result, err := svc.Run(ctx, agentboot.RunRequest{
		ProjectPath: t.TempDir(),
		Prompt:      "Reply with exactly one word: pong",
		Opts: agentboot.ExecutionOptions{
			OutputFormat: agentboot.OutputFormatStreamJSON,
		},
	}, e2eAutoApprove{}, nil)

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.IsSuccess(),
		"expected success; exit=%d err=%q", result.ExitCode, result.Error)
	assert.NotEmpty(t, strings.TrimSpace(result.TextOutput()), "expected non-empty output")
}
