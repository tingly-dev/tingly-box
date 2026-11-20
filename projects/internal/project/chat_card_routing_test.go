package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// Binding a chat to a project, or to any thread of it, is the same as binding
// it to the whole project: every card of the project (the coordinator's
// decision cards, any thread's approvals and questions) reaches it, and it may
// answer them all.

// A chat bound to one worker thread hears the coordinator's decision cards and
// answers them.
func TestThreadBoundChatReceivesAndAnswersDecisionCards(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	worker, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Docs", Task: "write docs", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", worker.ID, "")
	require.NoError(t, err)

	card := askShip(t, svc, f.project.ID)

	m := sink.nextPrompt(t)
	require.Equal(t, "own", m.chatID)
	require.Equal(t, card.ID, m.p.RequestID)
	_, err = svc.AnswerFromChannel("telegram", "own", m.p.ThreadID, m.p.RequestID, "o0")
	require.NoError(t, err)
	require.NotNil(t, chosen(t, svc, card))
}

// A project chat that follows another thread (/session 2) still hears the
// coordinator's decision cards.
func TestProjectChatFollowingAThreadReceivesDecisionCards(t *testing.T) {
	f, svc, sink, _ := projectChatOnSecondThread(t)

	card := askShip(t, svc, f.project.ID)

	m := sink.nextPrompt(t)
	require.Equal(t, "up", m.chatID)
	require.Equal(t, card.ID, m.p.RequestID)
	_, err := svc.AnswerFromChannel("telegram", "up", m.p.ThreadID, m.p.RequestID, "o0")
	require.NoError(t, err)
	require.NotNil(t, chosen(t, svc, card))
}

// A chat bound to one worker thread hears and answers the approvals of its
// sibling threads.
func TestThreadBoundChatAnswersASiblingsApproval(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	docs, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Docs", Task: "write docs", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", docs.ID, "")
	require.NoError(t, err)
	deploy, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ship", Task: "deploy", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)

	m := sink.nextPrompt(t)
	require.Equal(t, "own", m.chatID)
	require.Equal(t, deploy.ID, m.p.ThreadID)
	_, err = svc.AnswerFromChannel("telegram", "own", deploy.ID, m.p.RequestID, ChoiceAllow)
	require.NoError(t, err)
	waitStatus(t, f.store, deploy.ID, db.ThreadIdle)
}

// A reply that is not an answer to anything is an ordinary message, not eaten,
// even when several requests are open and the chat could not tell which one a
// "y" would approve.
func TestReplyThatAnswersNothingIsAnOrdinaryMessage(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error {
		_, err := turn.Approve(turn.Prompt, "Bash", map[string]any{"command": turn.Prompt})
		return err
	})
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	first, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "One", Task: "first", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	sink.nextPrompt(t)
	second, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Two", Task: "second", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	sink.nextPrompt(t)

	handled, err := svc.PostFromChannel("telegram", "up", "how is it going?", nil)
	require.NoError(t, err)
	require.True(t, handled)

	msgs := waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool {
		for _, e := range m {
			if e.Kind == db.EvUser && e.Content == "how is it going?" {
				return true
			}
		}
		return false
	})
	assert.NotEmpty(t, msgs)
	_, ok := f.threads.PendingRequest(first.ID, "first")
	assert.True(t, ok, "nothing was approved or denied by it")
	_, ok = f.threads.PendingRequest(second.ID, "second")
	assert.True(t, ok)
	require.NoError(t, f.threads.Respond(first.ID, "first", Answer{Allow: false}))
	require.NoError(t, f.threads.Respond(second.ID, "second", Answer{Allow: false}))
}
