package prompt_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/prompt"
)

// Compile-time check: FakePrompter satisfies agentboot.Prompter.
var _ agentboot.Prompter = (*prompt.FakePrompter)(nil)

func TestFakePrompter_DefaultApproves(t *testing.T) {
	p := prompt.NewFakePrompter()

	got, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{
		ToolName: "Bash",
		Input:    map[string]any{"command": "pwd"},
	})
	require.NoError(t, err)
	assert.True(t, got.Approved, "default behavior is approve")
	assert.Equal(t, map[string]any{"command": "pwd"}, got.UpdatedInput)
}

func TestFakePrompter_ScriptedFIFO(t *testing.T) {
	p := prompt.NewFakePrompter().
		QueueApproval(agentboot.PermissionResult{Approved: false, Reason: "scripted deny 1"}).
		QueueApproval(agentboot.PermissionResult{Approved: true, Reason: "scripted ok 2"})

	r1, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.False(t, r1.Approved)
	assert.Equal(t, "scripted deny 1", r1.Reason)

	r2, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.True(t, r2.Approved)
	assert.Equal(t, "scripted ok 2", r2.Reason)

	// Queue drained — back to default approve.
	r3, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.True(t, r3.Approved)
	assert.Empty(t, r3.Reason)
}

func TestFakePrompter_TimeoutDeny(t *testing.T) {
	// Configure the prompter to "block" longer than the test deadline.
	p := prompt.NewFakePrompter().WithDelay(200 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	got, err := p.OnApproval(ctx, agentboot.PermissionRequest{ToolName: "Bash"})

	require.Error(t, err, "ctx deadline must surface as an error")
	assert.True(t, errors.Is(err, context.DeadlineExceeded))
	assert.False(t, got.Approved, "deadline must default-deny")
}

func TestFakePrompter_CtxCancelDeny(t *testing.T) {
	p := prompt.NewFakePrompter().WithDelay(200 * time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	got, err := p.OnAsk(ctx, agentboot.AskRequest{ID: "x"})

	require.Error(t, err)
	assert.True(t, errors.Is(err, context.Canceled))
	assert.False(t, got.Approved)
}

func TestFakePrompter_AlwaysAllowCaching(t *testing.T) {
	// First approval carries Remember=true; second call for the same
	// tool MUST short-circuit without consuming a queued response.
	p := prompt.NewFakePrompter().
		QueueApproval(agentboot.PermissionResult{Approved: true, Remember: true, Reason: "user said always"})

	r1, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.True(t, r1.Approved)
	assert.Equal(t, "user said always", r1.Reason)

	// No more script entries queued. If the cache works, this still
	// approves; if it didn't, it would fall through to default-approve
	// (also true) — so we additionally queue a deny that should NOT be
	// consumed.
	p.QueueApproval(agentboot.PermissionResult{Approved: false, Reason: "should not be reached"})

	r2, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.True(t, r2.Approved, "AlwaysAllow must short-circuit second call")
	assert.Contains(t, r2.Reason, "Always Allow")

	// The queued deny should still be there, ready for a different
	// tool.
	r3, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Edit"})
	require.NoError(t, err)
	assert.False(t, r3.Approved, "queued deny must apply to a tool not in the AlwaysAllow cache")
	assert.Equal(t, "should not be reached", r3.Reason)
}

func TestFakePrompter_AddAlwaysAllow(t *testing.T) {
	// Direct cache seeding — no script needed.
	p := prompt.NewFakePrompter()
	p.AddAlwaysAllow("Bash")

	got, err := p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
	require.NoError(t, err)
	assert.True(t, got.Approved)
	assert.Contains(t, got.Reason, "Always Allow")
}

func TestFakePrompter_ConcurrentSafe(t *testing.T) {
	p := prompt.NewFakePrompter()
	for i := 0; i < 10; i++ {
		p.QueueApproval(agentboot.PermissionResult{Approved: true, Reason: "ok"})
	}
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			_, _ = p.OnApproval(context.Background(), agentboot.PermissionRequest{ToolName: "Bash"})
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	assert.Equal(t, 10, p.ApprovalCalls())
}

func TestFakePrompter_OnAskScripted(t *testing.T) {
	p := prompt.NewFakePrompter().
		QueueAsk(agentboot.AskResult{ID: "q1", Approved: true, Selection: map[string]any{"option": "apple"}})

	got, err := p.OnAsk(context.Background(), agentboot.AskRequest{ID: "q1", Type: "question"})
	require.NoError(t, err)
	assert.True(t, got.Approved)
	assert.Equal(t, map[string]any{"option": "apple"}, got.Selection)
	assert.Equal(t, 1, p.AskCalls())
}
