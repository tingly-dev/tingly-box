package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
	"github.com/tingly-dev/tingly-box/remote/agent"
)

func proposeTestCard(t *testing.T, f *testFixture, svc *Service, sink *promptSink) *db.ThreadEvent {
	t.Helper()
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	card, err := svc.Coordinator.ProposeCard(context.Background(), f.project, ProposalCard{Suggestions: []Suggestion{
		{Title: "A", Task: "do a"}, {Title: "B", Task: "do b"},
	}})
	require.NoError(t, err)
	prompt := sink.nextPrompt(t)
	require.Contains(t, prompt.p.Text, "Proposed threads")
	require.Contains(t, choiceLabels(prompt.p.Choices), "Dismiss all")
	return card
}

func choiceLabels(choices []agent.Choice) []string {
	out := make([]string, len(choices))
	for i, c := range choices {
		out[i] = c.Label
	}
	return out
}

func TestDismissedOnWebSettlesTheChatCopy(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	card := proposeTestCard(t, f, svc, sink)

	_, err := svc.Coordinator.DismissSuggestions(f.project.MainThreadID, card.ID, nil, Source{})
	require.NoError(t, err)

	restated := sink.nextRestate(t)
	assert.Contains(t, restated.text, "Dismissed on the web: A, B")
	assert.Empty(t, svc.Relay.awaitingAll("telegram", "up"), "the settled chat no longer answers the card")
}

func TestDismissAllFromChatSettlesTheWebCard(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	card := proposeTestCard(t, f, svc, sink)

	_, err := svc.AnswerFromChannel("telegram", "up", f.project.MainThreadID, card.ID, choiceDismissAll)
	require.NoError(t, err)

	pl := payloadOf(t, f.store, card.ID)
	for i, s := range pl.Suggestions {
		assert.True(t, s.Dismissed, "proposal %d is dismissed from the chat", i)
	}
	restated := sink.nextRestate(t)
	assert.Contains(t, restated.text, "Dismissed in a chat: A, B")
}

func TestDismissReplyFromChat(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	card := proposeTestCard(t, f, svc, sink)

	handled, err := svc.PostFromChannel("telegram", "up", "dismiss 2", nil)
	require.NoError(t, err)
	require.True(t, handled, "a dismiss reply answers the card instead of reaching the coordinator")
	pl := payloadOf(t, f.store, card.ID)
	assert.False(t, pl.Suggestions[0].Dismissed)
	assert.True(t, pl.Suggestions[1].Dismissed)
}

func TestSupersededCardSettlesTheChatCopy(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error { turn.Say("ok", 1, 1); return nil })
	proposeTestCard(t, f, svc, sink)

	svc.Coordinator.SupersedeSuggestions(f.project.MainThreadID, "", Source{})

	restated := sink.nextRestate(t)
	assert.Contains(t, restated.text, "Expired on the web: A, B")
	assert.Empty(t, svc.Relay.awaitingAll("telegram", "up"))
}
