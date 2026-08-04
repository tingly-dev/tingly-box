package remoteagent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// Test_AgentE2E_AskQuestion_TextReply mirrors Test_AgentE2E_AskQuestion but
// answers with a plain-text "2" (1-based option index) instead of a keyboard
// callback — the only reply path on platforms without inline keyboards
// (weixin, wecom, whatsapp, dingtalk).
func Test_AgentE2E_AskQuestion_TextReply(t *testing.T) {
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
		fixture.AskQuestionStep("req-ask-text", "tool-1", askInput),
		fixture.AssistantText("got it"),
		fixture.Result(true),
	})

	chat.SendText("ask me")
	drainProcessingPreface(t, chat)

	prompt := chat.WaitAskQuestionPrompt(3 * time.Second)
	require.NotEmpty(t, prompt.RequestID)

	// Reply with "2" as text — 1-based index selecting "banana".
	chat.SendText("2")

	chat.ExpectInOrderLoose(3*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Selected", Name: "text-selection-ack"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "got it", Name: "post-ask-assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID))
}
