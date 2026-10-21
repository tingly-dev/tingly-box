package project

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// folderChat starts a folder thread with spec and binds chat "fold" to it.
func folderChat(t *testing.T, f *testFixture, svc *Service, spec FolderThreadSpec) *db.Thread {
	t.Helper()
	spec.Task = "go"
	th, err := f.startFolder(t.TempDir(), spec)
	require.NoError(t, err)
	waitQuiet(t, f, svc, th.ID)
	_, err = svc.BindChannel("telegram", "fold", "", th.ID, "")
	require.NoError(t, err)
	return th
}

func boundTo(t *testing.T, f *testFixture) *db.Thread {
	t.Helper()
	b, err := f.store.GetChannelBinding("telegram", "fold")
	require.NoError(t, err)
	th, err := f.store.GetThread(b.ThreadID)
	require.NoError(t, err)
	return th
}

func TestNewWithATaskStartsANewFolderSessionAndMovesTheChat(t *testing.T) {
	f, svc, sink := channelFixture(t)
	old := folderChat(t, f, svc, FolderThreadSpec{Model: "m1", PermissionMode: "plan"})

	handled, reply, err := svc.ChannelCommand("telegram", "fold", ChatNew, "fix the login bug")
	require.NoError(t, err)
	require.True(t, handled)
	cur := boundTo(t, f)
	assert.NotEqual(t, old.ID, cur.ID)
	assert.Equal(t, old.ProjectID, cur.ProjectID, "the same folder")
	assert.Equal(t, "fix the login bug", cur.Task)
	assert.Equal(t, "m1", cur.Model, "settings carry over")
	assert.Equal(t, "plan", cur.PermissionMode)
	assert.Equal(t, 2, cur.Seq)
	assert.True(t, strings.HasPrefix(reply, "Started [#2 fix the login bug]"), reply)
	assert.Equal(t, "[#2 fix the login bug] ok", sink.next(t).text)

	evs := userEvents(t, f.store, cur.ID)
	require.Len(t, evs, 1)
	assert.Equal(t, db.SourceIM, evs[0].Source, "the first message is marked as from the chat")
	_, err = f.store.GetThread(old.ID)
	assert.NoError(t, err, "the old session stays")
}

func TestNewWithoutATaskStartsOnTheNextMessage(t *testing.T) {
	f, svc, sink := channelFixture(t)
	old := folderChat(t, f, svc, FolderThreadSpec{})

	_, reply, err := svc.ChannelCommand("telegram", "fold", ChatNew, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "next message starts a new session")
	assert.Equal(t, old.ID, boundTo(t, f).ID, "nothing starts yet")

	handled, err := svc.PostFromChannel("telegram", "fold", "hello again", nil)
	require.NoError(t, err)
	require.True(t, handled)
	cur := boundTo(t, f)
	assert.NotEqual(t, old.ID, cur.ID)
	assert.Equal(t, "hello again", cur.Task)
	sink.next(t)

	// The one after continues the new session.
	waitQuiet(t, f, svc, cur.ID)
	_, err = svc.PostFromChannel("telegram", "fold", "and more", nil)
	require.NoError(t, err)
	assert.Equal(t, cur.ID, boundTo(t, f).ID)
	sink.next(t)
}

func TestAResolvedFolderSessionGivesWayToANewOne(t *testing.T) {
	f, svc, sink := channelFixture(t)
	old := folderChat(t, f, svc, FolderThreadSpec{})
	_, err := f.threads.Resolve(old.ID)
	require.NoError(t, err)

	_, err = svc.PostFromChannel("telegram", "fold", "next task", nil)
	require.NoError(t, err)
	assert.NotEqual(t, old.ID, boundTo(t, f).ID, "a done session does not refuse the chat")
	sink.next(t)
}

func TestStopAndClearInAFolderChat(t *testing.T) {
	started := make(chan struct{}, 4)
	f, svc, _ := channelFixtureWith(t, func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "long") {
			started <- struct{}{}
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("ok", 1, 1)
		return nil
	})
	th := folderChat(t, f, svc, FolderThreadSpec{})

	_, reply, err := svc.ChannelCommand("telegram", "fold", ChatStop, "")
	require.NoError(t, err)
	assert.Equal(t, "[#1 go] is not running.", reply)

	_, err = svc.PostFromChannel("telegram", "fold", "long job", nil)
	require.NoError(t, err)
	<-started
	_, reply, err = svc.ChannelCommand("telegram", "fold", ChatStop, "")
	require.NoError(t, err)
	assert.Equal(t, "Stopped [#1 go].", reply)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	_, err = svc.PostFromChannel("telegram", "fold", "long again", nil)
	require.NoError(t, err)
	<-started
	_, reply, err = svc.ChannelCommand("telegram", "fold", ChatClear, "")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(reply, "Stopped [#1 go]. Your next message starts a new session"), reply)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)
	b, err := f.store.GetChannelBinding("telegram", "fold")
	require.NoError(t, err)
	assert.True(t, b.NewThread)
}

func TestCommandsInAProjectChatExplainInsteadOfActing(t *testing.T) {
	f, svc, _ := channelFixture(t)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)
	for name, arg := range map[string]string{ChatStop: "", ChatClear: "", ChatNew: "x"} {
		handled, reply, err := svc.ChannelCommand("telegram", "1", name, arg)
		require.NoError(t, err)
		assert.True(t, handled, name)
		assert.Contains(t, reply, "project conversation", name)
	}
	b, err := f.store.GetChannelBinding("telegram", "1")
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, b.ThreadID, "still on the project conversation")
	assert.False(t, b.NewThread)

	// A worker's chat has no folder to start a session in.
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err = svc.BindChannel("telegram", "w", "", th.ID, "")
	require.NoError(t, err)
	_, reply, err := svc.ChannelCommand("telegram", "w", ChatNew, "x")
	require.NoError(t, err)
	assert.Contains(t, reply, "bound to a folder")
}

func TestCommandsInAnUnboundChatAreNotHandled(t *testing.T) {
	_, svc, _ := channelFixture(t)
	handled, _, err := svc.ChannelCommand("telegram", "nobody", ChatStop, "")
	assert.NoError(t, err)
	assert.False(t, handled)
	handled, _, _ = svc.ChannelCommand("telegram", "nobody", "help", "")
	assert.False(t, handled)
}

func TestRebindingClearsAPendingNew(t *testing.T) {
	f, svc, _ := channelFixture(t)
	folderChat(t, f, svc, FolderThreadSpec{})
	_, _, err := svc.ChannelCommand("telegram", "fold", ChatNew, "")
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "fold", "", "", f.project.ID)
	require.NoError(t, err)
	b, err := f.store.GetChannelBinding("telegram", "fold")
	require.NoError(t, err)
	assert.False(t, b.NewThread)
}
