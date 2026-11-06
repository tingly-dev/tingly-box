package project

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// projectChatOnSecondThread starts a worker that asks to run `make deploy`
// (#1) and a quiet one (#2), with chat "up" bound to the project and then
// moved to #2 with /session 2, the way a chat that follows another thread
// is. #1's prompt was sent while the chat was still on the conversation.
func projectChatOnSecondThread(t *testing.T) (*testFixture, *Service, *promptSink, *db.Thread) {
	t.Helper()
	f, svc, sink := promptFixture(t, approveScript)
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	deploy, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ship", Task: "deploy", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	sink.nextPrompt(t)
	_, err = f.threads.Start(context.Background(), f.project, StartSpec{Title: "Docs", Task: "write docs", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	handled, _, err := svc.ChannelCommand("telegram", "up", "session", "2")
	require.NoError(t, err)
	require.True(t, handled)
	b, err := f.store.GetChannelBinding("telegram", "up")
	require.NoError(t, err)
	require.NotEqual(t, deploy.ID, b.ThreadID, "the chat now follows another thread")
	return f, svc, sink, deploy
}

// A chat bound to a project answers the approvals of every thread of that
// project, not only the thread it happens to follow.
func TestProjectChatApprovesAnotherThreadsRequest(t *testing.T) {
	f, svc, sink, deploy := projectChatOnSecondThread(t)

	reply, err := svc.AnswerFromChannel("telegram", "up", deploy.ID, "req-1", ChoiceAllow)
	require.NoError(t, err)
	assert.Empty(t, reply)
	waitStatus(t, f.store, deploy.ID, db.ThreadIdle)
	assert.Contains(t, sink.nextRestate(t).text, "Allowed in a chat.")

	reply, err = svc.AnswerFromChannel("telegram", "up", deploy.ID, "req-1", ChoiceDeny)
	require.NoError(t, err)
	assert.Equal(t, "This was already answered.", reply)
}

func TestProjectChatDeniesAnotherThreadsRequest(t *testing.T) {
	f, svc, sink, deploy := projectChatOnSecondThread(t)

	_, err := svc.AnswerFromChannel("telegram", "up", deploy.ID, "req-1", ChoiceDeny)
	require.NoError(t, err)
	waitStatus(t, f.store, deploy.ID, db.ThreadIdle)
	assert.Contains(t, sink.nextRestate(t).text, "Denied in a chat.")
}

func TestProjectChatAnswersAnotherThreadsRequestByText(t *testing.T) {
	f, svc, sink, deploy := projectChatOnSecondThread(t)

	handled, err := svc.PostFromChannel("telegram", "up", "deny", nil)
	require.NoError(t, err)
	assert.True(t, handled)
	waitStatus(t, f.store, deploy.ID, db.ThreadIdle)
	assert.Contains(t, sink.nextRestate(t).text, "Denied in a chat.")
}

// A chat explicitly bound to one session of a folder (TargetThreadID) keeps
// to that session's request; only a chat bound to the project as a whole
// answers for its other threads.
func TestSessionBoundChatAnswersOnlyItsOwnLineage(t *testing.T) {
	f, svc, sink, deploy := projectChatOnSecondThread(t)
	b, err := f.store.GetChannelBinding("telegram", "up")
	require.NoError(t, err)
	require.NoError(t, f.store.UpsertChannelBinding(&db.ChannelBinding{
		Platform: "telegram", ChatID: "pinned", ThreadID: b.ThreadID, TargetProjectID: f.project.ID, TargetThreadID: b.ThreadID,
	}))

	_, err = svc.AnswerFromChannel("telegram", "pinned", deploy.ID, "req-1", ChoiceAllow)
	assert.ErrorIs(t, err, ErrNotThisChat)
	_, ok := f.threads.PendingRequest(deploy.ID, "req-1")
	assert.True(t, ok, "still waiting")
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: true}))
	_ = sink
}

// A chat of another project is refused by button and by text, even one that
// was asked the request before it moved there.
func TestChatOfAnotherProjectCannotAnswer(t *testing.T) {
	f, svc, sink, deploy := projectChatOnSecondThread(t)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err := svc.BindChannel("telegram", "stranger", "", "", other.ID)
	require.NoError(t, err)

	_, err = svc.AnswerFromChannel("telegram", "stranger", deploy.ID, "req-1", ChoiceAllow)
	assert.ErrorIs(t, err, ErrNotThisChat)

	svc.Relay.await(db.ChannelBinding{Platform: "telegram", ChatID: "stranger"}, pendingRequest{threadID: deploy.ID, requestID: "req-1"})
	handled, err := svc.PostFromChannel("telegram", "stranger", "allow", nil)
	require.NoError(t, err)
	assert.True(t, handled)
	_, ok := f.threads.PendingRequest(deploy.ID, "req-1")
	assert.True(t, ok, "a stale request of the old project is not answered by a chat now in another")
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: true}))
	_ = sink
}
