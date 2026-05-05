package fixture_test

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
)

// TestFixture_AssistantThenResult drives a full execution end-to-end through
// claude.Driver + claude.Transport + agentboot.Runner using a fixture-backed
// process.Factory. It verifies that:
//   - the script's events arrive on handle.Events() in order
//   - the channel closes cleanly after the result step
//   - handle.Wait() returns a non-error Result
func TestFixture_AssistantThenResult(t *testing.T) {
	script := fixture.Script{
		fixture.System("sess-1", "/tmp"),
		fixture.AssistantText("hello from fixture"),
		fixture.Result(true),
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(script))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := agent.Execute(ctx, "test prompt", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var messageCount int
	for ev := range handle.Events() {
		switch ev.(type) {
		case agentboot.MessageEvent:
			messageCount++
		case agentboot.ApprovalRequestEvent, agentboot.AskRequestEvent:
			t.Fatalf("unexpected control event in pure-message script")
		}
	}

	if messageCount == 0 {
		t.Fatalf("expected at least one MessageEvent, got 0")
	}

	res, werr := handle.Wait()
	if werr != nil {
		t.Fatalf("Wait err: %v", werr)
	}
	if res == nil {
		t.Fatalf("Wait result is nil")
	}
	// Result.Events accumulates raw common.Event values from the decoder.
	// We expect at least system + assistant + result.
	if len(res.Events) < 3 {
		t.Fatalf("Result.Events count = %d, want >= 3", len(res.Events))
	}
}

// TestFixture_PermissionRoundTrip verifies that a permission request from
// the fixture surfaces as an ApprovalRequestEvent on the handle, and the
// consumer's Respond unblocks the script to continue.
func TestFixture_PermissionRoundTrip(t *testing.T) {
	script := fixture.Script{
		fixture.System("sess-2", "/tmp"),
		fixture.PermissionRequest("req-A", "Bash", map[string]any{"command": "ls"}),
		fixture.AssistantText("after approve"),
		fixture.Result(true),
	}

	agent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(script))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := agent.Execute(ctx, "test prompt", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	var sawApproval bool
	var afterApproveText string

	for ev := range handle.Events() {
		switch e := ev.(type) {
		case agentboot.ApprovalRequestEvent:
			if e.ID != "req-A" {
				t.Fatalf("approval ID = %q, want req-A", e.ID)
			}
			if e.ToolName != "Bash" {
				t.Fatalf("approval tool = %q, want Bash", e.ToolName)
			}
			sawApproval = true
			if rerr := handle.Respond(e.ID, agentboot.ApprovalResponse{Approved: true}); rerr != nil {
				t.Fatalf("Respond: %v", rerr)
			}
		case agentboot.MessageEvent:
			// Look for the post-approval assistant text in raw events.
			// The MessageEvent.Raw is a *claude.AssistantMessage; we can use Result.Events for inspection.
		}
	}

	if !sawApproval {
		t.Fatalf("never saw approval request")
	}

	res, werr := handle.Wait()
	if werr != nil {
		t.Fatalf("Wait err: %v", werr)
	}

	// Find the post-approval assistant text in the raw events.
	for _, ev := range res.Events {
		if ev.Type == "assistant" {
			if msg, ok := ev.Data["message"].(map[string]any); ok {
				if content, ok := msg["content"].([]any); ok && len(content) > 0 {
					if blk, ok := content[0].(map[string]any); ok {
						if text, ok := blk["text"].(string); ok {
							afterApproveText = text
						}
					}
				}
			}
		}
	}

	if afterApproveText != "after approve" {
		t.Fatalf("post-approval assistant text = %q, want %q", afterApproveText, "after approve")
	}
}

// TestFixture_DenyHaltsScript verifies that a deny response is delivered
// to the fixture; the fixture's internal stdin reader observes the denial
// and the script (which would normally continue) ends.
func TestFixture_DenyHaltsScript(t *testing.T) {
	script := fixture.Script{
		fixture.System("sess-3", "/tmp"),
		fixture.PermissionRequest("req-B", "Bash", map[string]any{"command": "rm -rf /"}),
		// In a real Claude session, the agent would react to deny by halting.
		// Our fixture continues blindly; for this test we simply verify the
		// deny travels through the wire.
		fixture.Result(true),
	}

	var observedStdin [][]byte
	agent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(
		script,
		fixture.WithObserveStdin(func(b []byte) {
			cp := make([]byte, len(b))
			copy(cp, b)
			observedStdin = append(observedStdin, cp)
		}),
	))

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	handle, err := agent.Execute(ctx, "dangerous", agentboot.ExecutionOptions{
		OutputFormat: agentboot.OutputFormatStreamJSON,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for ev := range handle.Events() {
		if e, ok := ev.(agentboot.ApprovalRequestEvent); ok {
			_ = handle.Respond(e.ID, agentboot.ApprovalResponse{Approved: false, Reason: "blocked by test"})
		}
	}

	if _, err := handle.Wait(); err != nil {
		t.Fatalf("Wait err: %v", err)
	}

	// observedStdin should contain at least the user prompt and the deny response.
	if len(observedStdin) < 2 {
		t.Fatalf("observed %d stdin messages, want >= 2 (prompt + response)", len(observedStdin))
	}
	// Find a deny in the observed messages. We search for "behavior":"deny".
	var sawDeny bool
	for _, b := range observedStdin {
		if containsAll(b, []byte(`"behavior"`), []byte(`"deny"`)) {
			sawDeny = true
			break
		}
	}
	if !sawDeny {
		t.Fatalf("did not observe deny response in stdin: %s", observedStdin)
	}
}

func containsAll(haystack []byte, needles ...[]byte) bool {
	for _, n := range needles {
		if !contains(haystack, n) {
			return false
		}
	}
	return true
}

func contains(haystack, needle []byte) bool {
	if len(needle) == 0 {
		return true
	}
outer:
	for i := 0; i+len(needle) <= len(haystack); i++ {
		for j := range needle {
			if haystack[i+j] != needle[j] {
				continue outer
			}
		}
		return true
	}
	return false
}
