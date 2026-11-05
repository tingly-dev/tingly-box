package projects

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/remote/channel"
)

// buttonless hides a channel's buttons the way DingTalk, WeChat and WeCom
// do: it still sends and receives text, but is not an actionChannel.
type buttonless struct{ channel.Channel }

type decisionCard struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Payload string `json:"payload"`
}

// chosenOption reads which option of the conversation's decision card was
// picked, or -1 while it is open.
func chosenOption(t *testing.T, h *tinglyProjectsHarness, mainThreadID string) int {
	t.Helper()
	var events struct {
		Events []decisionCard `json:"events"`
	}
	h.call("GET", "/threads/"+mainThreadID+"/events", nil, &events)
	for _, e := range events.Events {
		if e.Kind != "decision" {
			continue
		}
		var d struct {
			Chosen *int `json:"chosen"`
		}
		require.NoError(t, json.Unmarshal([]byte(e.Payload), &d))
		if d.Chosen != nil {
			return *d.Chosen
		}
	}
	return -1
}

func bindCoordinator(t *testing.T, h *tinglyProjectsHarness, name string, chatID string) (projectID, mainThreadID string) {
	t.Helper()
	var project struct {
		ID           string `json:"id"`
		MainThreadID string `json:"main_thread_id"`
	}
	h.call("POST", "/projects", map[string]any{"name": name}, &project)
	h.call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": chatID, "bot_uuid": h.chosenID, "project_id": project.ID}, nil)
	return project.ID, project.MainThreadID
}

// A decision card the coordinator asks through its real MCP tool reaches the
// bound chat as buttons; pressing one is the user's answer to the
// coordinator, and the card is restated as answered.
func TestTinglyDecisionCardReachesChatAsButtons(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("decider").OpenDM(h.chosenID)
	_, main := bindCoordinator(t, h, "Decision project", chat.ChatID)

	chat.SendText("ask decision")
	card := chat.WaitText(10 * time.Second)
	card.AssertContains(t, "Ship it now?")
	card.AssertContains(t, "1. Ship — goes to prod")
	card.AssertContains(t, "2. Wait — one more review (recommended)")
	card.AssertContains(t, "reviewers are still reading")
	ship, ok := card.ButtonByLabel("Ship")
	require.True(t, ok, "a decision must be actionable from IM")
	_, ok = card.ButtonByLabel("Wait (recommended)")
	require.True(t, ok)
	chat.WaitText(10*time.Second).AssertContains(t, "Decision asked")
	require.Equal(t, -1, chosenOption(t, h, main))

	ship.Click()
	chat.WaitRestate(card.MessageID, 10*time.Second).AssertContains(t, "Answered in a chat: Ship")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: Ship")
	require.Equal(t, 0, chosenOption(t, h, main))

	// The same press again is too late and changes nothing.
	ship.Click()
	chat.WaitText(10*time.Second).AssertContains(t, "This was already answered.")
	require.Equal(t, 0, chosenOption(t, h, main))
}

// A chat whose platform has no buttons (DingTalk's case) gets the card as
// numbered text, and answers it by replying with the number.
func TestTinglyDecisionCardReachesButtonlessChatAsNumberedText(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	orig, ok := h.registry.Get(h.chosenID)
	require.True(t, ok)
	h.registry.Register(buttonless{orig})
	chat := h.env.NewUser("dingtalk-like").OpenDM(h.chosenID)
	_, main := bindCoordinator(t, h, "Plain decision project", chat.ChatID)

	chat.SendText("ask decision")
	card := chat.WaitText(10 * time.Second)
	card.AssertContains(t, "Ship it now?")
	card.AssertContains(t, "1. Ship — goes to prod")
	card.AssertContains(t, "2. Wait — one more review (recommended)")
	card.AssertContains(t, "Reply 1 or 2 to choose.")
	require.Empty(t, card.Buttons, "this platform cannot show buttons")

	// Other words are an ordinary message; the card stays open.
	chat.WaitText(10*time.Second).AssertContains(t, "Decision asked")
	chat.SendText("tell me more")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: tell me more")
	require.Equal(t, -1, chosenOption(t, h, main))

	chat.SendText("2")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: Wait")
	require.Equal(t, 1, chosenOption(t, h, main))

	// Answered: "1" is a plain message again.
	chat.SendText("1")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: 1")
	require.Equal(t, 1, chosenOption(t, h, main))
}

// Only a chat of the conversation may answer its card: a chat of another
// project pressing the same button is refused and nothing is chosen.
func TestTinglyDecisionCardIsAnsweredOnlyByItsOwnConversation(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("owner").OpenDM(h.chosenID)
	_, main := bindCoordinator(t, h, "Owner project", chat.ChatID)
	stranger := h.env.NewUser("stranger").OpenDM(h.chosenID)
	bindCoordinator(t, h, "Other project", stranger.ChatID)

	chat.SendText("ask decision")
	card := chat.WaitText(10 * time.Second)
	ship, ok := card.ButtonByLabel("Ship")
	require.True(t, ok)
	chat.WaitText(10*time.Second).AssertContains(t, "Decision asked")

	stranger.SendCallback(card.MessageID, ship.CallbackData)
	stranger.WaitText(10*time.Second).AssertContains(t, "Could not answer")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventRestate)
	require.Equal(t, -1, chosenOption(t, h, main))

	ship.Click()
	chat.WaitText(10*time.Second).AssertContains(t, "You said: Ship")
	require.Equal(t, 0, chosenOption(t, h, main))
}

