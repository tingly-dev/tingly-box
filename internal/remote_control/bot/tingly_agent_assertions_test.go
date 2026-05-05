//go:build pivot_to_fixture_pending

package bot_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	mockagent "github.com/tingly-dev/tingly-box/agentboot/mockagent"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// Test_AgentE2E_AssistantTextSentExactlyOnce asserts the assistant text
// emitted by the script reaches the chat exactly once, not twice.
//
// Background: MockAgentExecutor previously sent the response a second time
// via SendTextWithActionKeyboard *after* streamingMessageHandler had already
// streamed it. This test caught that regression.
func Test_AgentE2E_AssistantTextSentExactlyOnce(t *testing.T) {
	const phrase = "hello from mock once"

	_, _, chat := agentBoot(t, mockagent.NewScript().
		Assistant(phrase).
		Success("done").
		Build())

	chat.SendText("hi")
	drainProcessingPreface(t, chat)

	// Wait for the script to fully play out: streaming text + the
	// CompletionCallback's "Task done" card.
	waitTextContaining(t, chat, "Task done", 6, 3*time.Second)

	chat.AssertTextOccurrences(phrase, 1)
}

// Test_AgentE2E_MockTaskDoneCard asserts the mock executor delivers the
// CompletionCallback "Task done" footer + action keyboard at the end of a
// successful run, matching the Claude Code path.
//
// Background: MockAgentExecutor never registered SetCompletionCallback, so
// no "Task done" card was ever sent for mock-routed chats.
func Test_AgentE2E_MockTaskDoneCard(t *testing.T) {
	_, harness, chat := agentBoot(t, mockagent.NewScript().
		Assistant("processing").
		Success("ok").
		Build())

	chat.SendText("go")
	drainProcessingPreface(t, chat)

	// Find the "Task done" message within a small window.
	var doneEvt *testenv.OutEvent
	for i := 0; i < 6 && doneEvt == nil; i++ {
		evt := chat.WaitText(3 * time.Second)
		if strings.Contains(evt.Text, "Task done") {
			doneEvt = evt
		}
	}
	require.NotNil(t, doneEvt, "expected a Task done card after script success")

	// Session must be marked completed by the CompletionCallback.
	// lastMockSession polls until terminal (race-free via GetStatus).
	require.Equal(t, session.StatusCompleted, lastMockSession(t, harness, chat.ChatID),
		"CompletionCallback should mark mock session as completed")
}

// Test_AgentE2E_DenyDoesNotSendEmptyMessage asserts that when a permission
// is denied and the script halts (no further assistant text), the bot does
// NOT send an empty message via the action-keyboard path.
//
// Background: ChunkText("") returned [""] and SendTextWithActionKeyboard
// happily sent it. Tests that scanned for "Deny" / "Denied" never noticed
// the empty bubble that landed in chat.
func Test_AgentE2E_DenyDoesNotSendEmptyMessage(t *testing.T) {
	_, _, chat := agentBoot(t, mockagent.NewScript().
		Permission("Bash", map[string]any{"command": "rm -rf /"},
			mockagent.WithExpectApproved(false)).
		Build())

	chat.SendText("dangerous")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitApprovalPrompt(3 * time.Second)
	prompt.Deny()

	// Wait long enough for: callback ack → IMPrompter edits prompt to
	// "❌ Denied" → MockAgentExecutor's post-script SendTextWithActionKeyboard
	// (which currently sends an empty bubble — what this test catches).
	time.Sleep(800 * time.Millisecond)

	// Verify no empty Send events ever appeared for this chat.
	for _, e := range chat.AllEvents() {
		if e.Kind == tingly.EventSend && strings.TrimSpace(e.Text) == "" && len(e.Media) == 0 {
			t.Fatalf("found empty outbound text event: %s", brief(e))
		}
	}

	// And at least one denial signal must have arrived (either as a Send
	// containing "Denied"/"❌" or as an Edit on the prompt message).
	sawDenialConfirm := false
	for _, e := range chat.AllEvents() {
		if strings.Contains(e.Text, "Denied") || strings.Contains(e.Text, "❌") {
			sawDenialConfirm = true
			break
		}
	}
	require.True(t, sawDenialConfirm, "expected denial confirmation in chat events")
}

// brief copies testenv.brief for in-test debugging — testenv keeps it
// unexported, so we build a tiny mirror here.
func brief(e testenv.OutEvent) string {
	text := e.Text
	if len(text) > 60 {
		text = text[:60] + "..."
	}
	return string(e.Kind) + " " + text
}
