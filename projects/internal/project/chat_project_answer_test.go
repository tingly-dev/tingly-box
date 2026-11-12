package project

import (
	"context"
	"strings"
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

// A project chat must receive requests raised after it follows a sibling thread.
func TestProjectChatReceivesNewApprovalAfterSwitch(t *testing.T) {
	for _, plain := range []bool{false, true} {
		t.Run(map[bool]string{false: "buttons", true: "text"}[plain], func(t *testing.T) {
			projectChatReceivesNewApprovalAfterSwitch(t, plain)
		})
	}
}

func projectChatReceivesNewApprovalAfterSwitch(t *testing.T, plain bool) {
	t.Helper()
	f, svc, sink := promptFixture(t, approveScript)
	sink.plain["up"] = plain
	_, err := svc.BindChannel("telegram", "up", "", "", f.project.ID)
	require.NoError(t, err)
	_, err = f.threads.Start(context.Background(), f.project, StartSpec{Title: "Docs", Task: "write docs", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	handled, _, err := svc.ChannelCommand("telegram", "up", "session", "1")
	require.NoError(t, err)
	require.True(t, handled)
	bound, err := f.store.GetChannelBinding("telegram", "up")
	require.NoError(t, err)
	require.NoError(t, f.store.UpsertChannelBinding(&db.ChannelBinding{
		Platform: "telegram", ChatID: "pinned", ThreadID: bound.ThreadID, TargetProjectID: f.project.ID, TargetThreadID: bound.ThreadID,
	}))
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	_, err = svc.BindChannel("telegram", "stranger", "", "", other.ID)
	require.NoError(t, err)
	deploy, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Ship", Task: "deploy", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	if plain {
		var asked sentText
		for range 5 {
			asked = sink.next(t)
			if strings.Contains(asked.text, "make deploy") {
				break
			}
		}
		require.Contains(t, asked.text, "make deploy")
		require.Equal(t, "up", asked.chatID)
		handled, err := svc.PostFromChannel("telegram", "up", "y", nil)
		require.NoError(t, err)
		require.True(t, handled)
	} else {
		prompt := sink.nextPrompt(t)
		require.Equal(t, "up", prompt.chatID)
		require.Equal(t, deploy.ID, prompt.p.ThreadID)
		_, err = svc.AnswerFromChannel("telegram", "up", deploy.ID, "req-1", ChoiceAllow)
		require.NoError(t, err)
		assert.Contains(t, sink.nextRestate(t).text, "Allowed in a chat.")
	}
	assert.Empty(t, svc.Relay.awaitingAll("telegram", "pinned"))
	assert.Empty(t, svc.Relay.awaitingAll("telegram", "stranger"))
	waitStatus(t, f.store, deploy.ID, db.ThreadIdle)
}

// A chat selecting a new folder has left its old thread even before it sends
// the first message to start the new session.
func TestPendingFolderRebindCannotAnswerOldThread(t *testing.T) {
	f, svc, _, deploy := projectChatOnSecondThread(t)
	_, err := svc.BindChannel("telegram", "up", "", deploy.ID, "")
	require.NoError(t, err)
	require.NoError(t, f.store.SetChannelBindingFolder("telegram", "up", "", t.TempDir(), ""))
	_, err = svc.AnswerFromChannel("telegram", "up", deploy.ID, "req-1", ChoiceAllow)
	require.ErrorIs(t, err, ErrNotThisChat)
	_, waiting := f.threads.PendingRequest(deploy.ID, "req-1")
	require.True(t, waiting)
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: false}))
}

func TestRebindingDuringTextAuthorizationDoesNotAnswer(t *testing.T) {
	f, svc, _, deploy := projectChatOnSecondThread(t)
	other := mkProject(t, f.store, &db.Project{Name: "Other"})
	called := false
	handled, err := svc.PostFromChannel("telegram", "up", "allow", nil, func() bool {
		called = true
		_, err := svc.BindChannel("telegram", "up", "", "", other.ID)
		require.NoError(t, err)
		return true
	})
	require.NoError(t, err)
	require.True(t, called)
	require.True(t, handled)
	_, waiting := f.threads.PendingRequest(deploy.ID, "req-1")
	require.True(t, waiting)
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: false}))
}

func TestFolderSessionsCannotAnswerSiblingApprovals(t *testing.T) {
	f, svc, _, deploy := projectChatOnSecondThread(t)
	folder, err := f.store.OpenFolder(t.TempDir(), "")
	require.NoError(t, err)
	first, err := f.threads.StartFolderThread(FolderThreadSpec{ProjectID: folder.ID, Title: "Folder one", Task: "write docs"})
	require.NoError(t, err)
	second, err := f.threads.StartFolderThread(FolderThreadSpec{ProjectID: folder.ID, Title: "Folder two", Task: "write docs"})
	require.NoError(t, err)
	b := &db.ChannelBinding{Platform: "telegram", ChatID: "folder", ThreadID: first.ID, TargetProjectID: folder.ID}
	require.NoError(t, svc.mayAnswer(b, first.ID))
	require.ErrorIs(t, svc.mayAnswer(b, second.ID), ErrNotThisChat)
	require.NoError(t, f.threads.Respond(deploy.ID, "req-1", Answer{Allow: false}))
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
