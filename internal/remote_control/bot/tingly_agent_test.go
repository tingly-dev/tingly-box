//go:build pivot_to_fixture_pending

package bot_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	mockagent "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// agentBoot returns a bootHelper variant that registers a custom mock agent
// for the chat. The returned chat is already routed to the mock agent.
func agentBoot(t *testing.T, script []mockagent.Step) (*testenv.TestEnv, *bot.TestHarness, *testenv.Chat) {
	t.Helper()

	env, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	customMock := mockagent.NewAgent(mockagent.Config{Script: script})
	harness.AgentBoot.RegisterAgent(agentboot.AgentTypeMockAgent, customMock)
	harness.SetCurrentAgent(chat.ChatID, "mock")

	return env, harness, chat
}

// drainProcessingPreface reads the leading "🧪 Mock: Processing..." reply that
// MockAgentExecutor sends before invoking the agent. Tests use this to focus
// assertions on script-driven events.
func drainProcessingPreface(t *testing.T, chat *testenv.Chat) {
	t.Helper()
	evt := chat.WaitText(3 * time.Second)
	if !strings.Contains(evt.Text, "Mock: Processing") {
		t.Fatalf("expected 'Mock: Processing...' preface, got %q", evt.Text)
	}
}

// waitTextContaining scans up to maxScan outbound text messages for the first
// containing substr. Fails the test if not found in time.
//
// Prefer chat.Expect / chat.ExpectInOrderLoose for new tests — this helper is
// kept around as an escape hatch where the surrounding event ordering is too
// flaky to lock down.
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

// lastMockSession finds the most recent mock-agent session for chatID and
// polls up to 3 s for it to reach a terminal state. Status is read via
// Manager.GetStatus (which holds the manager lock) so there is no data race
// with the executor goroutine that writes session.Status concurrently.
func lastMockSession(t *testing.T, harness *bot.TestHarness, chatID string) session.Status {
	t.Helper()
	all := harness.SessionMgr.ListByChat(chatID)
	var sessID string
	for _, s := range all {
		if s.Agent == "mock" {
			sessID = s.ID // .ID is immutable after creation — safe to read
		}
	}
	if sessID == "" {
		t.Fatalf("no mock session for chat %s; have %d sessions", chatID, len(all))
		return ""
	}

	// Poll until terminal. Reading via GetStatus (holds RLock) is race-free.
	var last session.Status
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := harness.SessionMgr.GetStatus(sessID); ok {
			switch st {
			case session.StatusCompleted, session.StatusFailed,
				session.StatusClosed, session.StatusExpired:
				return st
			default:
				last = st
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("session %s for chat %s never reached terminal state (last: %s)", sessID, chatID, last)
	return ""
}

// Test_AgentE2E_AssistantText drives the bot through a script that emits a
// single assistant text and a success result. The assertions verify the
// strict sequence: assistant text, then the CompletionCallback's "Task done"
// footer, and finally that the session is marked completed.
func Test_AgentE2E_AssistantText(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Assistant("hello from mock").
		Success("done").
		Build())

	chat.SendText("hi")
	drainProcessingPreface(t, chat)

	// Loose ordering: the platform interleaves typing-indicator (EventReact)
	// events around outbound sends. We only care about the relative order
	// of the script-driven sends, not the ambient noise.
	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "hello from mock", Name: "assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastMockSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_PermissionApprove verifies the full permission round-trip:
// the bot surfaces a permission prompt, the user approves, the prompt is
// edited to mark approval, the agent emits its post-approval text, and the
// session ends in completed.
func Test_AgentE2E_PermissionApprove(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", map[string]any{"command": "pwd"},
			mockagent.WithExpectApproved(true),
			mockagent.WithDenyHalts(false)).
		Assistant("after approve").
		Success("ok").
		Build())

	chat.SendText("run pwd")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID, "permission prompt should carry a request id")
	prompt.Approve()

	// After approve, the IMPrompter posts an "✅ Allow for tool: ..." ack
	// (on Telegram it edits the prompt; on the tingly platform it posts a
	// new send, which is what we observe here). Then the agent emits the
	// "after approve" assistant text and the CompletionCallback's
	// Task done card.
	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Allow for tool", Name: "approve-ack"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "after approve", Name: "post-approve-assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastMockSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_PermissionDeny verifies that clicking Deny halts the script,
// the prompt is edited to mark denial, no further assistant text appears,
// and the session is marked failed (the script terminates with permission_denied).
func Test_AgentE2E_PermissionDeny(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", map[string]any{"command": "rm -rf /"},
			mockagent.WithExpectApproved(false)).
		Assistant("never reached").
		Build())

	chat.SendText("dangerous")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny()

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Deny for tool", Name: "deny-ack"},
	)

	// Give the executor a beat to finish its (now empty) post-script work.
	time.Sleep(500 * time.Millisecond)

	// The "never reached" assistant must never appear — script halted on deny.
	for _, e := range chat.AllEvents() {
		if strings.Contains(e.Text, "never reached") {
			t.Fatalf("script kept running past denial: saw %q", e.Text)
		}
	}

	// Session should be failed (Result with status=permission_denied → SetFailed).
	require.Equal(t, session.StatusFailed, lastMockSession(t, harness, chat.ChatID),
		"deny should mark session as failed")
}

// Test_AgentE2E_AskQuestion drives the bot through an AskUserQuestion script
// step and verifies the option keyboard works end-to-end.
func Test_AgentE2E_AskQuestion(t *testing.T) {
	questions := []mockagent.AskQuestion{
		{
			Question: "pick a fruit",
			Options: []mockagent.AskOption{
				{Label: "apple"},
				{Label: "banana"},
				{Label: "cherry"},
			},
		},
	}
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Ask(questions, mockagent.WithAskExpectAnswers(map[int]int{0: 1})).
		Assistant("got it").
		Success("ok").
		Build())

	chat.SendText("ask me")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitAskQuestionPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID)
	// The keyboard must contain the option labels — guards against the
	// IMPrompter type-assertion regression that would have rendered the
	// default Approve/Deny keyboard instead.
	require.Contains(t, summarizeButtonLabels(prompt.Event), "apple")
	require.Contains(t, summarizeButtonLabels(prompt.Event), "banana")
	prompt.SelectOption(0, 1) // banana

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "got it", Name: "post-ask-assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastMockSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_ScriptError verifies that an ErrorStep reaches the chat as an
// "[ERROR] ..." message via the streaming handler's OnError path. Both the
// preceding assistant text and the error must appear; their order on the bot
// goroutine is non-deterministic, so we only assert that both appeared.
func Test_AgentE2E_ScriptError(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Assistant("trying").
		Error(errors.New("boom")).
		Success("done").
		Build())

	chat.SendText("trigger error")
	drainProcessingPreface(t, chat)

	// Wait for the script to fully play out, then scan the recorded events.
	waitTextContaining(t, chat, "Task done", 8, 3*time.Second)

	sawTrying, sawErr := false, false
	for _, e := range chat.AllEvents() {
		if e.Kind == tingly.EventSend && strings.Contains(e.Text, "trying") {
			sawTrying = true
		}
		if e.Kind == tingly.EventSend && strings.Contains(e.Text, "boom") {
			sawErr = true
		}
	}
	require.True(t, sawTrying, "did not see 'trying' assistant text")
	require.True(t, sawErr, "did not see 'boom' error text")

	// The script ends with a success Result, so the session is completed
	// even though OnError fired for the (non-fatal) ErrorStep.
	require.Equal(t, session.StatusCompleted, lastMockSession(t, harness, chat.ChatID))
}

// Test_AgentE2E_PermissionExpectMismatch verifies that scripts with
// ExpectApproved mismatches surface via the streaming handler's OnError path
// (rendered as "[ERROR] mockagent: ...").
func Test_AgentE2E_PermissionExpectMismatch(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", nil,
			mockagent.WithExpectApproved(true),
			mockagent.WithDenyHalts(true)).
		Build())

	chat.SendText("trigger")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny() // user denies, but the script expected approval → mismatch

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "expected approved=true", Name: "mismatch-error"},
	)

	// Mismatch + OnDenyTerminate halts the script with permission_denied →
	// session ends in failed.
	require.Equal(t, session.StatusFailed, lastMockSession(t, harness, chat.ChatID))
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
