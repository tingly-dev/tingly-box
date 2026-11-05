package project

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

var shipDecision = Decision{
	Question: "Ship it now?",
	Context:  "CI is green",
	Options: []DecisionOption{
		{Label: "Ship", Consequence: "goes to prod"},
		{Label: "Wait", Consequence: "one more review"},
	},
	Recommended: 1,
	Reason:      "reviewers are still reading",
}

// askShip posts shipDecision as the coordinator would and returns its card.
func askShip(t *testing.T, svc *Service, projectID string) db.ThreadEvent {
	t.Helper()
	svc.Coordinator.ask(projectID, shipDecision)
	p, err := svc.Store.GetProject(projectID)
	require.NoError(t, err)
	events, err := svc.Store.ListThreadEvents(p.MainThreadID)
	require.NoError(t, err)
	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == db.EvDecision {
			return events[i]
		}
	}
	t.Fatal("no decision card was posted")
	return db.ThreadEvent{}
}

func chosen(t *testing.T, svc *Service, card db.ThreadEvent) *int {
	t.Helper()
	m, err := svc.Store.GetThreadEvent(card.ID)
	require.NoError(t, err)
	var d Decision
	require.NoError(t, json.Unmarshal([]byte(m.Payload), &d))
	return d.Chosen
}

// A decision card reaches a chat that can show buttons as buttons, one per
// option, with the recommendation and its reason in the text.
func TestDecisionCardReachesAChatWithButtons(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)

	card := askShip(t, svc, f.project.ID)

	m := sink.nextPrompt(t)
	assert.Equal(t, "up", m.chatID)
	assert.Equal(t, f.project.MainThreadID, m.p.ThreadID)
	assert.Equal(t, card.ID, m.p.RequestID)
	assert.Contains(t, m.p.Text, "[Demo] Ship it now?")
	assert.Contains(t, m.p.Text, "CI is green")
	assert.Contains(t, m.p.Text, "1. Ship — goes to prod")
	assert.Contains(t, m.p.Text, "2. Wait — one more review (recommended)")
	assert.Contains(t, m.p.Text, "reviewers are still reading")
	require.Len(t, m.p.Choices, 2)
	assert.Equal(t, ChannelChoice{Value: "o0", Label: "Ship"}, m.p.Choices[0])
	assert.Equal(t, ChannelChoice{Value: "o1", Label: "Wait (recommended)"}, m.p.Choices[1])
}

// A chat without buttons gets the card as text with numbered options and a
// hint to reply with the number.
func TestDecisionCardReachesAPlainChatAsNumberedText(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["dingtalk-chat"] = true
	_, err := svc.BindChannel("dingtalk", "dingtalk-chat", "", "", f.project.ID)
	require.NoError(t, err)

	askShip(t, svc, f.project.ID)

	got := sink.next(t)
	assert.Equal(t, "dingtalk", got.platform)
	assert.Equal(t, "dingtalk-chat", got.chatID)
	assert.Contains(t, got.text, "Ship it now?")
	assert.Contains(t, got.text, "1. Ship — goes to prod")
	assert.Contains(t, got.text, "2. Wait — one more review (recommended)")
	assert.Contains(t, got.text, "Reply 1 or 2")
	assert.True(t, strings.HasSuffix(got.text, "[Demo]"), "it says which project it is from")
	_, _, waiting := svc.Relay.Awaiting("dingtalk", "dingtalk-chat")
	assert.True(t, waiting, "the chat's next message may answer it")
}

// A host that cannot show buttons at all (a bare ChannelSink) still gets it.
func TestDecisionCardReachesAHostWithoutPrompts(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "42", "", "", f.project.ID)
	require.NoError(t, err)

	askShip(t, svc, f.project.ID)

	assert.Contains(t, sink.next(t).text, "Reply 1 or 2")
}

// A button press chooses the option as the user's own message to the
// coordinator, and the card is restated everywhere with the outcome.
func TestPressingADecisionButtonAnswersTheCoordinator(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	card := askShip(t, svc, f.project.ID)
	m := sink.nextPrompt(t)

	reply, err := svc.AnswerFromChannel("telegram", "up", m.p.ThreadID, m.p.RequestID, "o0")
	require.NoError(t, err)
	assert.Empty(t, reply)

	require.NotNil(t, chosen(t, svc, card))
	assert.Equal(t, 0, *chosen(t, svc, card))
	assert.Contains(t, sink.nextRestate(t).text, "Answered in a chat: Ship")
	assert.Equal(t, "Echo: Ship\n\n[Demo]", sink.next(t).text, "the coordinator got the label as the user's message")

	var user *db.ThreadEvent
	events, _ := f.store.ListThreadEvents(f.project.MainThreadID)
	for i := range events {
		if events[i].Kind == db.EvUser {
			user = &events[i]
		}
	}
	require.NotNil(t, user)
	assert.Equal(t, "Ship", user.Content, "the label alone, never the coordinator's wording")
	assert.Equal(t, db.SourceIM, user.Source)
	assert.Equal(t, "up", user.SourceChatID)

	reply, err = svc.AnswerFromChannel("telegram", "up", m.p.ThreadID, m.p.RequestID, "o1")
	require.NoError(t, err)
	assert.Equal(t, "This was already answered.", reply)
	assert.Equal(t, 0, *chosen(t, svc, card), "the second press changes nothing")
}

func TestOnlyAChatOfTheConversationMayAnswerADecision(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err = svc.BindChannel("telegram", "stranger", "", "", other.ID)
	require.NoError(t, err)
	card := askShip(t, svc, f.project.ID)
	m := sink.nextPrompt(t)

	_, err = svc.AnswerFromChannel("telegram", "stranger", m.p.ThreadID, m.p.RequestID, "o0")
	assert.ErrorIs(t, err, ErrNotThisChat)
	_, err = svc.AnswerFromChannel("telegram", "nobody", m.p.ThreadID, m.p.RequestID, "o0")
	assert.ErrorIs(t, err, ErrNotThisChat)
	_, err = svc.AnswerFromChannel("telegram", "up", m.p.ThreadID, m.p.RequestID, "allow")
	assert.Error(t, err, "a decision has no allow button")
	_, err = svc.AnswerFromChannel("telegram", "up", m.p.ThreadID, m.p.RequestID, "o9")
	assert.Error(t, err, "an option that is not on the card")
	assert.Nil(t, chosen(t, svc, card))
}

// Replying with an option's number or its label chooses it; any other
// message is an ordinary one and leaves the card open.
func TestRepliesChooseADecisionOptionInAPlainChat(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["dt"] = true
	_, err := svc.BindChannel("dingtalk", "dt", "", "", f.project.ID)
	require.NoError(t, err)
	card := askShip(t, svc, f.project.ID)
	sink.next(t)

	handled, err := svc.PostFromChannel("dingtalk", "dt", "what do you think?", nil)
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, "Echo: what do you think?\n\n[Demo]", sink.next(t).text, "an ordinary message")
	assert.Nil(t, chosen(t, svc, card), "the card is still open")
	_, _, waiting := svc.Relay.Awaiting("dingtalk", "dt")
	assert.True(t, waiting)

	handled, err = svc.PostFromChannel("dingtalk", "dt", " wait. ", nil)
	require.NoError(t, err)
	require.True(t, handled)
	require.NotNil(t, chosen(t, svc, card))
	assert.Equal(t, 1, *chosen(t, svc, card), "the label, in any case")
	assert.Equal(t, "Echo: Wait\n\n[Demo]", sink.next(t).text)
	_, _, waiting = svc.Relay.Awaiting("dingtalk", "dt")
	assert.False(t, waiting, "answered: the next message is a message again")

	card2 := askShip(t, svc, f.project.ID)
	sink.next(t)
	_, err = svc.PostFromChannel("dingtalk", "dt", "1", nil)
	require.NoError(t, err)
	require.NotNil(t, chosen(t, svc, card2))
	assert.Equal(t, 0, *chosen(t, svc, card2), "the number")
	assert.Equal(t, "Echo: Ship\n\n[Demo]", sink.next(t).text)
}

// A card answered on the web is restated in the chats, and a later "1" in
// a chat is no longer an answer.
func TestADecisionAnsweredOnTheWebIsRestatedInChats(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "plain", "", "", f.project.ID)
	require.NoError(t, err)
	sink.plain["plain"] = true
	card := askShip(t, svc, f.project.ID)
	sink.nextPrompt(t)
	sink.next(t)

	_, err = svc.Coordinator.Choose(t.Context(), f.project.MainThreadID, card.ID, 1, Source{})
	require.NoError(t, err)

	assert.Contains(t, sink.nextRestate(t).text, "Answered on the web: Wait")
	_, _, waiting := svc.Relay.Awaiting("telegram", "plain")
	assert.False(t, waiting)
	_, _, waiting = svc.Relay.Awaiting("telegram", "up")
	assert.False(t, waiting)
}

// A card the chat left open does not take the answer to a later approval.
func TestAnOpenDecisionDoesNotBlockALaterApprovalInTheSameChat(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["up"] = true
	card := askShip(t, svc, f.project.ID)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	// The chat was bound after the card, so ask again now that it listens.
	card = askShip(t, svc, f.project.ID)
	sink.next(t)
	th := startDeploy(t, f, svc)
	assert.Contains(t, sink.next(t).text, "Approve: Run `make deploy`")

	handled, err := svc.PostFromChannel("telegram", "up", "y", nil)
	require.NoError(t, err)
	require.True(t, handled)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	assert.Nil(t, chosen(t, svc, card), "the card is still open")
}
