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

func openLive(t *testing.T, ctx context.Context, prompt string, opts agentboot.ExecutionOptions) agentboot.PersistentSession {
	t.Helper()
	agent := claude.NewAgent(agentboot.DefaultConfig())
	if !agent.IsAvailable() {
		t.Skip("claude CLI not available; skipping e2e")
	}
	opts.ProjectPath = t.TempDir()
	session, err := agent.Open(ctx, prompt, opts)
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = session.Close(closeCtx)
	})
	return session
}

// A message injected while Claude is in the middle of a tool call is seen
// by the model in the same turn: one turn, one result, and the answer obeys
// the injected instruction. (.design/agentboot-live-control.md §2)
//
// Run with: go test -tags e2e -run TestE2E_Live ./agentboot/
func TestE2E_LiveInjectIntoRunningTurn(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	session := openLive(t, ctx,
		"Use the Bash tool to run `sleep 8`, then run `echo first-done`, then say DONE-A.",
		agentboot.ExecutionOptions{PermissionMode: "bypassPermissions"})

	go func() {
		time.Sleep(4 * time.Second)
		assert.NoError(t, session.Inject(ctx, "INJECTED: also end your final answer with the word BANANA."))
	}()

	res, err := agentboot.RunTurnWithPrompter(ctx, session, e2eAutoApprove{}, nil)
	require.NoError(t, err)
	assert.True(t, res.IsSuccess(), "err=%q", res.Error)
	out := strings.ToUpper(res.TextOutput())
	assert.Contains(t, out, "DONE-A")
	assert.Contains(t, out, "BANANA", "the injected message was not seen by the running turn")
	assert.Equal(t, agentboot.SessionStateIdle, session.Status())
}

// Model and permission mode change on the live process; the process is the
// same one afterwards (it still remembers turn one).
func TestE2E_LiveSetModelAndPermissionMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Second)
	defer cancel()
	session := openLive(t, ctx, "Remember the word ALPHA. Reply OK.", agentboot.ExecutionOptions{})
	_, err := agentboot.RunTurnWithPrompter(ctx, session, e2eAutoApprove{}, nil)
	require.NoError(t, err)

	require.NoError(t, session.SetModel(ctx, "claude-haiku-5-5"))
	require.NoError(t, session.SetPermissionMode(ctx, "acceptEdits"))

	var rejected *agentboot.ControlRejectedError
	err = session.SetPermissionMode(ctx, "not-a-mode")
	if err != nil {
		assert.ErrorAs(t, err, &rejected)
	}

	require.NoError(t, session.Send(ctx, "What word did I ask you to remember? Reply with just that word."))
	res, err := agentboot.RunTurnWithPrompter(ctx, session, e2eAutoApprove{}, nil)
	require.NoError(t, err)
	assert.Contains(t, strings.ToUpper(res.TextOutput()), "ALPHA")
}
