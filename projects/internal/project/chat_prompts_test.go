package project

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// promptSink is a fakeSink that can also show buttons, except in the chats
// listed in plain.
type promptSink struct {
	*fakeSink
	plain map[string]bool

	mu       sync.Mutex
	prompts  chan sentPromptMsg
	restated chan sentText
}

type sentPromptMsg struct {
	chatID string
	p      ChannelPrompt
}

func newPromptSink() *promptSink {
	return &promptSink{fakeSink: newFakeSink(), plain: map[string]bool{}, prompts: make(chan sentPromptMsg, 32), restated: make(chan sentText, 32)}
}

func (f *promptSink) SendChannelPrompt(_ context.Context, platform, chatID string, p ChannelPrompt) (string, error) {
	if f.plain[chatID] {
		return "", ErrNoButtons
	}
	f.prompts <- sentPromptMsg{chatID, p}
	return "msg-" + chatID, nil
}

func (f *promptSink) RestateChannelPrompt(_ context.Context, platform, chatID, ref, text string) error {
	if ref != "msg-"+chatID {
		return nil
	}
	f.restated <- sentText{platform, chatID, text}
	return nil
}

func (f *promptSink) nextPrompt(t *testing.T) sentPromptMsg {
	t.Helper()
	select {
	case m := <-f.prompts:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no prompt was sent")
		return sentPromptMsg{}
	}
}

func (f *promptSink) nextRestate(t *testing.T) sentText {
	t.Helper()
	select {
	case m := <-f.restated:
		return m
	case <-time.After(5 * time.Second):
		t.Fatal("no prompt was restated")
		return sentText{}
	}
}

// promptFixture is channelFixtureWith over a sink with buttons.
func promptFixture(t *testing.T, script func(turn *enginetest.Turn) error) (*testFixture, *Service, *promptSink) {
	t.Helper()
	f, svc, _ := channelFixtureWith(t, script)
	sink := newPromptSink()
	relay := NewRelay(f.store, sink)
	svc.Coordinator.OnFinal = relay.Message
	f.threads.OnWait = relay.NeedsUser
	f.threads.OnSettled = relay.Settled
	f.threads.OnReply = relay.ThreadReply
	svc.Relay = relay
	return f, svc, sink
}

func approveScript(turn *enginetest.Turn) error {
	if !strings.Contains(turn.Prompt, "deploy") {
		turn.Say("ok", 1, 1)
		return nil
	}
	resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make deploy"})
	if err != nil {
		return err
	}
	turn.Say(map[bool]string{true: "deployed", false: "not deployed"}[resp.Approved], 1, 1)
	return nil
}

// startDeploy starts a worker that asks to run `make deploy`, with chat "up"
// on the project and chat "own" on the worker.
func startDeploy(t *testing.T, f *testFixture, svc *Service) *db.Thread {
	t.Helper()
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ship", Task: "deploy", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", th.ID, "")
	require.NoError(t, err)
	return th
}

func TestApprovalIsAnsweredFromAChat(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	th := startDeploy(t, f, svc)

	got := map[string]ChannelPrompt{}
	for range 2 {
		m := sink.nextPrompt(t)
		got[m.chatID] = m.p
	}
	require.Contains(t, got, "up", "the project's chat hears its threads' approvals")
	p := got["own"]
	assert.Equal(t, th.ID, p.ThreadID)
	assert.Equal(t, "req-1", p.RequestID)
	assert.Equal(t, "[#1 Ship] Approve: Run `make deploy`", p.Text)
	assert.Equal(t, []string{ChoiceAllow, ChoiceAllowThread, ChoiceDeny}, []string{p.Choices[0].Value, p.Choices[1].Value, p.Choices[2].Value})

	reply, err := svc.AnswerFromChannel("telegram", "up", th.ID, "req-1", ChoiceAllow)
	require.NoError(t, err)
	assert.Empty(t, reply)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	restated := map[string]string{}
	for range 2 {
		m := sink.nextRestate(t)
		restated[m.chatID] = m.text
	}
	assert.Equal(t, "[#1 Ship] Approve: Run `make deploy`\n\nAllowed in a chat.", restated["own"], "every chat sees it handled")
	assert.Contains(t, restated["up"], "Allowed in a chat.")

	events, _ := f.store.ListThreadEvents(th.ID)
	for _, e := range events {
		if e.Kind == db.EvApproval {
			assert.Contains(t, e.Payload, `"state":"allowed"`)
			assert.Contains(t, e.Payload, `"via":"im"`, "the web shows where it was answered")
		}
	}

	reply, err = svc.AnswerFromChannel("telegram", "own", th.ID, "req-1", ChoiceDeny)
	require.NoError(t, err)
	assert.Equal(t, "This was already answered.", reply, "the second answer is too late")
}

func TestApprovalAnsweredOnTheWebIsRestatedInChats(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	th := startDeploy(t, f, svc)
	sink.nextPrompt(t)
	sink.nextPrompt(t)

	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: false}))
	for range 2 {
		assert.True(t, strings.HasSuffix(sink.nextRestate(t).text, "Denied on the web."))
	}
}

func TestOnlyChatsOfTheThreadOrItsAncestorsMayAnswer(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	th := startDeploy(t, f, svc)
	sink.nextPrompt(t)
	sink.nextPrompt(t)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err := svc.BindChannel("telegram", "stranger", "", "", other.ID)
	require.NoError(t, err)

	_, err = svc.AnswerFromChannel("telegram", "stranger", th.ID, "req-1", ChoiceAllow)
	assert.ErrorIs(t, err, ErrNotThisChat)
	_, err = svc.AnswerFromChannel("telegram", "nobody", th.ID, "req-1", ChoiceAllow)
	assert.ErrorIs(t, err, ErrNotThisChat)
	_, err = svc.AnswerFromChannel("telegram", "own", th.ID, "req-1", "o0")
	assert.Error(t, err, "an approval has no options")
	_, ok := f.threads.PendingRequest(th.ID, "req-1")
	assert.True(t, ok, "still waiting")
	require.NoError(t, f.threads.Respond(th.ID, "req-1", Answer{Allow: true}))
}

func TestAQuestionIsAnsweredByReplying(t *testing.T) {
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error {
		if !strings.Contains(turn.Prompt, "pick") {
			turn.Say("ok", 1, 1)
			return nil
		}
		resp, err := turn.AskQuestions("q-1", map[string]any{"questions": []any{
			map[string]any{"question": "Which database?", "options": []any{
				map[string]any{"label": "Postgres", "description": "for production"},
				map[string]any{"label": "SQLite"},
			}},
		}})
		if err != nil {
			return err
		}
		answers, _ := resp.UpdatedInput["answers"].(map[string]string)
		turn.Say("chose "+answers["Which database?"], 1, 1)
		return nil
	})
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "DB", Task: "pick", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "own", "", th.ID, "")
	require.NoError(t, err)

	asked := sink.next(t)
	assert.Equal(t, "[#1 DB] Which database?\n  1. Postgres: for production\n  2. SQLite\nReply with a number or your own answer.", asked.text,
		"a question has no buttons, even where the chat could show them")

	handled, err := svc.PostFromChannel("telegram", "own", "2")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, "[#1 DB] chose SQLite", sink.next(t).text)
	_, _, waiting := svc.Relay.Awaiting("telegram", "own")
	assert.False(t, waiting, "answered: the next message is a message again")
}

func TestAChatWithoutButtonsAnswersAnApprovalByReplying(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["own"] = true
	startDeploy(t, f, svc)
	assert.Equal(t, "up", sink.nextPrompt(t).chatID)
	asked := sink.next(t)
	assert.Equal(t, "own", asked.chatID)
	assert.Equal(t, "[#1 Ship] Approve: Run `make deploy`\nReply y to allow, always to allow it for this thread, n to deny, or say what to do instead.", asked.text)

	handled, err := svc.PostFromChannel("telegram", "own", "y")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, "up", sink.nextRestate(t).chatID, "only prompts with buttons are restated")
	assert.Equal(t, "[#1 Ship] deployed", sink.next(t).text)
}

func TestOtherWordsDenyAnApprovalAndSayWhy(t *testing.T) {
	var reason string
	f, svc, sink := promptFixture(t, func(turn *enginetest.Turn) error {
		if !strings.Contains(turn.Prompt, "deploy") {
			turn.Say("ok", 1, 1)
			return nil
		}
		resp, err := turn.Approve("req-1", "Bash", map[string]any{"command": "make deploy"})
		if err != nil {
			return err
		}
		reason = resp.Reason
		turn.Say("stopped", 1, 1)
		return nil
	})
	startDeploy(t, f, svc)
	sink.nextPrompt(t)
	sink.nextPrompt(t)
	handled, err := svc.PostFromChannel("telegram", "own", "not on a Friday")
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, "[#1 Ship] stopped", sink.next(t).text)
	assert.Contains(t, reason, "not on a Friday")
}

func TestRepliesReadNumbersLinesAndWords(t *testing.T) {
	qs := askItems(map[string]any{"questions": []any{
		map[string]any{"question": "DB?", "options": []any{map[string]any{"label": "Postgres"}, map[string]any{"label": "SQLite"}}},
		map[string]any{"question": "Extras?", "multiSelect": true, "options": []any{map[string]any{"label": "Cache"}, map[string]any{"label": "Queue"}, map[string]any{"label": "Search"}}},
	}}, "")
	ans := askReply(qs, "2\n1, 3")
	assert.Equal(t, map[string]string{"DB?": "SQLite", "Extras?": "Cache, Search"}, ans.Answers)
	ans = askReply(qs, "whatever is cheapest")
	assert.Equal(t, "whatever is cheapest", ans.Answer, "not one line per question: it answers the first")
	assert.Equal(t, "1 3", pickOptions(qs[0], "1 3"), "several numbers for a single choice are words")
	assert.Equal(t, "9", pickOptions(qs[0], "9"))

	assert.True(t, approvalReply("Y").Allow)
	assert.Equal(t, "thread", approvalReply("always").Scope)
	assert.False(t, approvalReply("n").Allow)
	deny := approvalReply("use staging")
	assert.False(t, deny.Allow)
	assert.Equal(t, "use staging", deny.Answer)
}

func TestAStoppedTurnRestatesItsPrompts(t *testing.T) {
	f, svc, sink := promptFixture(t, approveScript)
	th := startDeploy(t, f, svc)
	sink.nextPrompt(t)
	sink.nextPrompt(t)
	f.threads.Stop(th.ID)
	for range 2 {
		assert.True(t, strings.HasSuffix(sink.nextRestate(t).text, "No longer waiting: the turn ended."))
	}
}
