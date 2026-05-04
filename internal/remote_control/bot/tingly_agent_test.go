package bot_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	mockagent "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
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

// Test_AgentE2E_AssistantText drives the bot through a script that emits a
// single assistant text and a success result. Verifies the assistant text
// reaches chat at least once.
func Test_AgentE2E_AssistantText(t *testing.T) {
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Assistant("hello from mock").
		Success("done").
		Build())

	chat.SendText("hi")
	drainProcessingPreface(t, chat)

	waitTextContaining(t, chat, "hello from mock", 4, 3*time.Second)
}

// Test_AgentE2E_PermissionApprove verifies the full permission round-trip:
// the bot surfaces a permission prompt, the user approves, and the agent
// continues to emit further events.
func Test_AgentE2E_PermissionApprove(t *testing.T) {
	approved := true
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", map[string]any{"command": "pwd"},
			mockagent.WithExpectApproved(approved),
			mockagent.WithDenyHalts(false)).
		Assistant("after approve").
		Success("ok").
		Build())

	chat.SendText("run pwd")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID, "permission prompt should carry a request id")
	prompt.Approve()

	waitTextContaining(t, chat, "after approve", 4, 3*time.Second)
}

// Test_AgentE2E_PermissionDeny verifies that clicking Deny halts the script
// and the bot surfaces a denial confirmation.
func Test_AgentE2E_PermissionDeny(t *testing.T) {
	denied := false
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", map[string]any{"command": "rm -rf /"},
			mockagent.WithExpectApproved(denied)).
		Assistant("never reached").
		Build())

	chat.SendText("dangerous")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny()

	// The denial confirmation lands within a few messages: either the
	// "❌ Deny for tool: ..." callback ack or the prompter's edited prompt
	// carrying "Denied".
	for i := 0; i < 4; i++ {
		evt := chat.WaitText(3 * time.Second)
		if strings.Contains(evt.Text, "Deny") || strings.Contains(evt.Text, "❌") || strings.Contains(evt.Text, "Denied") {
			return
		}
	}
	t.Fatalf("expected a denial-related reply after clicking Deny")
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
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Ask(questions, mockagent.WithAskExpectAnswers(map[int]int{0: 1})).
		Assistant("got it").
		Success("ok").
		Build())

	chat.SendText("ask me")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitAskQuestionPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID)
	prompt.SelectOption(0, 1) // banana

	waitTextContaining(t, chat, "got it", 6, 3*time.Second)
}

// Test_AgentE2E_ScriptError verifies that an ErrorStep reaches the chat as an
// "[ERROR] ..." message via the streaming handler's OnError path.
func Test_AgentE2E_ScriptError(t *testing.T) {
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Assistant("trying").
		Error(errors.New("boom")).
		Success("done").
		Build())

	chat.SendText("trigger error")
	drainProcessingPreface(t, chat)

	// We expect "trying" then an "[ERROR] boom" line — not necessarily in a
	// fixed order, since OnMessage and OnError can interleave on the bot
	// goroutine. Scan up to four messages for both.
	sawTrying, sawErr := false, false
	for i := 0; i < 6 && (!sawTrying || !sawErr); i++ {
		evt := chat.WaitText(3 * time.Second)
		if strings.Contains(evt.Text, "trying") {
			sawTrying = true
		}
		if strings.Contains(evt.Text, "boom") {
			sawErr = true
		}
	}
	if !sawTrying {
		t.Fatalf("did not see 'trying' assistant text")
	}
	if !sawErr {
		t.Fatalf("did not see 'boom' error text")
	}
}

// Test_AgentE2E_PermissionExpectMismatch verifies that scripts with
// ExpectApproved mismatches surface via the streaming handler's OnError path
// (rendered as "[ERROR] mockagent: ...").
func Test_AgentE2E_PermissionExpectMismatch(t *testing.T) {
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", nil,
			mockagent.WithExpectApproved(true),
			mockagent.WithDenyHalts(true)).
		Build())

	chat.SendText("trigger")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny() // user denies, but the script expected approval → mismatch

	// Scan up to six replies for the "expected approved=true" mismatch.
	for i := 0; i < 6; i++ {
		evt := chat.WaitText(3 * time.Second)
		if strings.Contains(evt.Text, "expected approved=true") {
			return
		}
	}
	t.Fatalf("did not see expectation-mismatch error in chat")
}
