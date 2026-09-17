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

// TestE2E_ClaudePersistentSession drives a real, unmocked `claude` CLI
// process through two turns on the same PersistentSession — the same real
// process (Runner.Open, claude.Agent.Open) that the empirical experiment in
// .design/claude-code.md §3.1 verified by hand with a throwaway script, now
// as a permanent, repeatable check against the actual production code path.
//
// It asserts what the fake-process-backed
// remote/control/remoteagent/persistent_session_e2e_test.go cannot: that
// the real CLI actually answers a second turn on the same stdin without
// exiting, and that it genuinely remembers the first turn's content (not
// just that our own protocol plumbing replays two scripted turns).
//
// Run with: go test -tags e2e ./agentboot/...   (requires the `claude` CLI
// on PATH and valid credentials; skips otherwise. Makes two real, minimal
// model calls — see .design/harness-remote.md §6.)
func TestE2E_ClaudePersistentSession(t *testing.T) {
	config := agentboot.DefaultConfig()
	agent := claude.NewAgent(config)
	if !agent.IsAvailable() {
		t.Skip("claude CLI not available; skipping e2e")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	session, err := agent.Open(ctx, "Reply with exactly the single word: ALPHA", agentboot.ExecutionOptions{
		ProjectPath: t.TempDir(),
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer closeCancel()
		_ = session.Close(closeCtx)
	})

	first, err := agentboot.RunTurnWithPrompter(ctx, session, e2eAutoApprove{}, nil)
	require.NoError(t, err)
	require.NotNil(t, first)
	assert.True(t, first.IsSuccess(), "turn 1: exit=%d err=%q", first.ExitCode, first.Error)
	assert.Contains(t, strings.ToUpper(first.TextOutput()), "ALPHA", "turn 1: expected ALPHA in reply")
	assert.Equal(t, agentboot.SessionStateIdle, session.Status())

	// Same process, same stdin — no new claude invocation. If this second
	// Send silently degraded to a fresh process (the context-lifetime bug
	// .design/claude-code.md §5.2 documents), the model would have no
	// memory of turn 1 and would not be able to answer this correctly.
	require.NoError(t, session.Send(ctx, "What was the single word I just asked you to reply with? Reply with just that word."))

	second, err := agentboot.RunTurnWithPrompter(ctx, session, e2eAutoApprove{}, nil)
	require.NoError(t, err)
	require.NotNil(t, second)
	assert.True(t, second.IsSuccess(), "turn 2: exit=%d err=%q", second.ExitCode, second.Error)
	assert.Contains(t, strings.ToUpper(second.TextOutput()), "ALPHA",
		"turn 2: model did not recall turn 1's word — persistent context was not actually preserved")

	sid1 := first.GetSessionID()
	sid2 := second.GetSessionID()
	if sid1 != "" && sid2 != "" {
		assert.Equal(t, sid1, sid2, "both turns should report the same underlying claude session_id")
	}
}
