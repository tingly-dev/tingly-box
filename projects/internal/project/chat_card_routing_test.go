package project

import (
	"context"
	"strings"
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

// Words that are not one of a request's answers do not answer it: they are an
// ordinary message to wherever the chat talks, and the request stays open.
func TestWordsWithoutACodeDoNotAnswerAnApprovalOrAQuestion(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "pick") {
			_, err := turn.AskQuestions("q-1", map[string]any{"questions": []any{
				map[string]any{"question": "Which database?", "options": []any{map[string]any{"label": "Postgres"}, map[string]any{"label": "SQLite"}}},
			}})
			return err
		}
		_, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make deploy"})
		return err
	})
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	deploy, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ship", Task: "deploy", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	sink.nextPrompt(t)
	for _, text := range []string{"not on a Friday", "sure, go ahead", "what is blocking this?"} {
		handled, err := svc.PostFromChannel("telegram", "up", text, nil)
		require.NoError(t, err)
		require.True(t, handled)
		waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool {
			for _, e := range m {
				if e.Kind == db.EvUser && e.Content == text {
					return true
				}
			}
			return false
		})
	}
	_, ok := f.threads.PendingRequest(deploy.ID, "req-1")
	assert.True(t, ok, "still waiting for its answer")
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: false}))

	pick, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "DB", Task: "pick", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	// The coordinator's echoes of the words above may arrive before or after
	// the question.
	sink.nextContaining(t, "Which database?")
	_, ok = f.threads.PendingRequest(pick.ID, "q-1")
	require.True(t, ok)
	handled, err := svc.PostFromChannel("telegram", "up", "whichever is cheaper", nil)
	require.NoError(t, err)
	require.True(t, handled)
	waitMessages(t, f.store, f.project.ID, func(m []db.ThreadEvent) bool {
		for _, e := range m {
			if e.Kind == db.EvUser && e.Content == "whichever is cheaper" {
				return true
			}
		}
		return false
	})
	_, ok = f.threads.PendingRequest(pick.ID, "q-1")
	assert.True(t, ok, "a question is answered by an option or by its code")
	handled, err = svc.PostFromChannel("telegram", "up", decisionCode("q-1")+" whichever is cheaper", nil)
	require.NoError(t, err)
	require.True(t, handled)
	waitStatus(t, f.store, pick.ID, db.ThreadIdle)
}

func proposeTwo(t *testing.T, f *testFixture, svc *Service) db.ThreadEvent {
	t.Helper()
	_, err := svc.Coordinator.proposeThreads(context.Background(), f.project, struct{ Threads []Suggestion }{[]Suggestion{
		{Title: "Docs", Task: "write the docs", Repo: "none"}, {Title: "Tests", Task: "add the tests", Repo: "none"},
	}})
	require.NoError(t, err)
	events, err := f.store.ListThreadEvents(f.project.MainThreadID)
	require.NoError(t, err)
	for _, e := range events {
		if e.Kind == db.EvSuggestions {
			return e
		}
	}
	t.Fatal("no suggestions card was posted")
	return db.ThreadEvent{}
}

// The coordinator's proposed threads reach a chat bound to any thread of the
// project, which starts one with its button, and the card is restated.
func TestProposedThreadsCanBeStartedFromAChat(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	worker, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Quiet", Task: "write docs", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", worker.ID, "")
	require.NoError(t, err)
	before := len(threadsOf(t, f))

	card := proposeTwo(t, f, svc)

	m := sink.nextPrompt(t)
	require.Equal(t, "own", m.chatID)
	require.Equal(t, card.ID, m.p.RequestID)
	var values []string
	for _, c := range m.p.Choices {
		values = append(values, c.Value)
	}
	assert.Equal(t, []string{"sall", "s0", "s1", "dall"}, values)

	_, err = svc.AnswerFromChannel("telegram", "own", m.p.ThreadID, m.p.RequestID, "s1")
	require.NoError(t, err)
	assert.Contains(t, sink.nextRestate(t).text, "Started in a chat: #")
	assert.Len(t, threadsOf(t, f), before+1)
	reply, err := svc.AnswerFromChannel("telegram", "own", m.p.ThreadID, m.p.RequestID, "sall")
	require.NoError(t, err)
	assert.Empty(t, reply, "the rest can still be started")
}

func TestProposedThreadsCanBeStartedByReplying(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["dt"] = true
	_, err := svc.BindChannel("dingtalk", "dt", "", "", f.project.ID)
	require.NoError(t, err)
	before := len(threadsOf(t, f))

	proposeTwo(t, f, svc)

	asked := sink.next(t)
	assert.Contains(t, asked.text, "1. Docs: write the docs\n2. Tests: add the tests")
	handled, err := svc.PostFromChannel("dingtalk", "dt", "start 2", nil)
	require.NoError(t, err)
	require.True(t, handled)
	assert.Len(t, threadsOf(t, f), before+1)
	assert.Contains(t, sink.next(t).text, "Started in a chat: #")
}
