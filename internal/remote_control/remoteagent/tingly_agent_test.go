package remoteagent_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/remote_control/remoteagent"

	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// agentBoot wires a TestEnv + bot harness backed by a fixture-driven Claude
// agent and routes the chat to "claude" so that messages flow through the
// real ClaudeCodeExecutor → claude.Driver → claude.Transport → Runner
// pipeline. The fixture script defines the wire-format events the
// substituted "claude binary" emits.
func agentBoot(t *testing.T, script fixture.Script) (*testenv.TestEnv, *remoteagent.TestHarness, *testenv.Chat) {
	t.Helper()

	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	rp := false
	setting := bot.BotSetting{
		UUID:           uuid,
		Name:           "tingly-test",
		Platform:       "tingly",
		AuthType:       "none",
		Auth:           map[string]string{},
		Enabled:        true,
		RequirePairing: &rp,
	}
	harness := remoteagent.BootForTest(t, env.Manager(), setting, remoteagent.TestBootOptions{
		FixtureScript: script,
	})
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(harness.Setting.UUID)
	harness.SetCurrentAgent(chat.ChatID, "claude")

	return env, harness, chat
}

// drainProcessingPreface reads the leading "⏳ CC: Processing..." reply that
// ClaudeCodeExecutor sends before invoking the agent.
func drainProcessingPreface(t *testing.T, chat *testenv.Chat) {
	t.Helper()
	evt := chat.WaitText(3 * time.Second)
	if !strings.Contains(evt.Text, "CC: Processing") {
		t.Fatalf("expected 'CC: Processing...' preface, got %q", evt.Text)
	}
}

// waitTextContaining scans up to maxScan outbound text messages for the
// first containing substr. Fails the test if not found in time.
func waitTextContaining(t *testing.T, chat *testenv.Chat, substr string, maxScan int, perWait time.Duration) *testenv.OutEvent {
	t.Helper()
	for i := 0; i < maxScan; i++ {
		evt := chat.WaitText(perWait)
		if strings.Contains(evt.Text, substr) {
			return evt
		}
	}
	t.Fatalf("did not see text containing %q within %d messages", substr, maxScan)
	return nil
}

// lastClaudeSession returns the status of the most recent claude session for
// chatID. The runner sets the terminal status inside Wait() before the bot
// executor sends its final chat message, so by the time the test has seen
// that message this read is guaranteed race-free.
func lastClaudeSession(t *testing.T, harness *remoteagent.TestHarness, chatID string) session.Status {
	t.Helper()
	all := harness.SessionMgr.ListByChat(chatID)
	var sessID string
	for _, s := range all {
		if s.Agent == "claude" {
			sessID = s.ID
		}
	}
	if sessID == "" {
		t.Fatalf("no claude session for chat %s; have %d sessions", chatID, len(all))
		return ""
	}
	st, ok := harness.SessionMgr.GetStatus(sessID)
	if !ok {
		t.Fatalf("session %s not found after terminal chat message", sessID)
		return ""
	}
	return st
}

// Test_AgentE2E_AssistantText drives the bot through a fixture script that
// emits a single assistant text and a success result.
func Test_AgentE2E_AssistantText(t *testing.T) {
	_, harness, chat := agentBoot(t, fixture.Script{
		fixture.AssistantText("hello from fixture"),
		fixture.Result(true),
	})

	chat.SendText("hi")
	drainProcessingPreface(t, chat)

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "hello from fixture", Name: "assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_PermissionApprove verifies the full permission round-trip.
func Test_AgentE2E_PermissionApprove(t *testing.T) {
	_, harness, chat := agentBoot(t, fixture.Script{
		fixture.PermissionRequest("req-approve", "Bash", map[string]any{"command": "pwd"}),
		fixture.AssistantText("after approve"),
		fixture.Result(true),
	})

	chat.SendText("run pwd")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID, "permission prompt should carry a request id")
	prompt.Approve()

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Allow for tool", Name: "approve-ack"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "after approve", Name: "post-approve-assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_PermissionDeny verifies that clicking Deny halts execution
// and the session ends in failed (the script's Result(false) terminator
// signals the simulated agent stopped on the denied permission).
func Test_AgentE2E_PermissionDeny(t *testing.T) {
	_, harness, chat := agentBoot(t, fixture.Script{
		fixture.PermissionRequest("req-deny", "Bash", map[string]any{"command": "rm -rf /"}),
		fixture.Result(false),
	})

	chat.SendText("dangerous")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny()

	// Wait for both the immediate denial ack AND the failure message that
	// the executor sends after handle.Wait() returns. The runner calls
	// store.SetFailed inside Wait(), before SendTextWithReply, so by the
	// time the failure message arrives the session status is guaranteed set.
	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Deny for tool", Name: "deny-ack"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Execution failed", Name: "failure-msg"},
	)

	require.Equal(t, session.StatusFailed, lastClaudeSession(t, harness, chat.ChatID),
		"deny + Result(false) should mark session as failed")
}

// Test_AgentE2E_AskQuestion drives the bot through a control_request /
// AskUserQuestion event and verifies the option keyboard works end-to-end.
func Test_AgentE2E_AskQuestion(t *testing.T) {
	askInput := map[string]any{
		"questions": []any{
			map[string]any{
				"question": "pick a fruit",
				"options": []any{
					map[string]any{"label": "apple"},
					map[string]any{"label": "banana"},
					map[string]any{"label": "cherry"},
				},
			},
		},
	}

	_, harness, chat := agentBoot(t, fixture.Script{
		fixture.AskQuestionStep("req-ask", "tool-1", askInput),
		fixture.AssistantText("got it"),
		fixture.Result(true),
	})

	chat.SendText("ask me")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitAskQuestionPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID)
	require.Contains(t, summarizeButtonLabels(prompt.Event), "apple")
	require.Contains(t, summarizeButtonLabels(prompt.Event), "banana")
	prompt.SelectOption(0, 1) // banana

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "got it", Name: "post-ask-assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID))
}

// summarizeButtonLabels returns a flat string of all button labels in the
// event's keyboard for use in failure messages and Contains assertions.
func summarizeButtonLabels(e *testenv.OutEvent) string {
	var labels []string
	for _, row := range e.Buttons {
		for _, b := range row {
			labels = append(labels, b.Label)
		}
	}
	return strings.Join(labels, "|")
}
