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

func TestParseMention(t *testing.T) {
	for in, want := range map[string]struct {
		n    int
		rest string
		ok   bool
	}{
		"@2 add the edge case":   {2, "add the edge case", true},
		"@#12 go\non":            {12, "go\non", true},
		"  @3   spaced  ":        {3, "spaced", true},
		"@2":                     {0, "", false},
		"@opus write the report": {0, "", false},
		"@0 nothing":             {0, "", false},
		"email me@2 later":       {0, "", false},
	} {
		n, rest, ok := parseMention(in)
		assert.Equal(t, want.ok, ok, in)
		assert.Equal(t, want.n, n, in)
		assert.Equal(t, want.rest, rest, in)
	}
}

// boundOf is the thread chat id is bound to.
func boundOf(t *testing.T, f *testFixture, chatID string) string {
	t.Helper()
	b, err := f.store.GetChannelBinding("telegram", chatID)
	require.NoError(t, err)
	return b.ThreadID
}

func TestMentionSendsOneMessageToThatThread(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	handled, err := svc.PostFromChannel("telegram", "1", "@1 add the edge case", nil)
	require.NoError(t, err)
	require.True(t, handled)
	evs := userEvents(t, f.store, th.ID)
	assert.Equal(t, "add the edge case", evs[len(evs)-1].Content)
	assert.Equal(t, db.SourceIM, evs[len(evs)-1].Source)
	assert.Equal(t, f.project.MainThreadID, boundOf(t, f, "1"), "the chat stays where it was")
	assert.Contains(t, sink.next(t).text, "[#1 Worker] Finished: ok", "the reply comes back as the thread's report")

	_, err = svc.PostFromChannel("telegram", "1", "@9 hello", nil)
	assert.ErrorIs(t, err, ErrNoSuchThread)
}

func TestSessionListsAndSwitches(t *testing.T) {
	f, svc, sink := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	_, reply, err := svc.ChannelCommand("telegram", "1", ChatSession, "")
	require.NoError(t, err)
	assert.True(t, strings.HasPrefix(reply, "#0 the project conversation (this chat)\n#1 Worker: idle\n"), reply)

	_, reply, err = svc.ChannelCommand("telegram", "1", ChatSession, "#1")
	require.NoError(t, err)
	assert.Equal(t, "This chat now talks to [#1 Worker]. /session 0 returns to the project conversation of Demo.", reply)
	assert.Equal(t, th.ID, boundOf(t, f, "1"))

	// A plain message now goes to the worker, and its reply to this chat.
	_, err = svc.PostFromChannel("telegram", "1", "keep going", nil)
	require.NoError(t, err)
	assert.Equal(t, "[#1 Worker] ok", sink.next(t).text)
	waitQuiet(t, f, svc, th.ID)

	_, reply, err = svc.ChannelCommand("telegram", "1", ChatSession, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "#1 Worker: idle (this chat)")

	_, reply, err = svc.ChannelCommand("telegram", "1", ChatSession, "0")
	require.NoError(t, err)
	assert.Equal(t, "This chat now talks to the project conversation of Demo.", reply)
	assert.Equal(t, f.project.MainThreadID, boundOf(t, f, "1"))

	_, _, err = svc.ChannelCommand("telegram", "1", ChatSession, "7")
	assert.ErrorIs(t, err, ErrNoSuchThread)
	_, _, err = svc.ChannelCommand("telegram", "1", ChatSession, "Worker")
	assert.Error(t, err)
}

func TestAResolvedThreadHandsItsChatsBackToItsParent(t *testing.T) {
	f, svc, _ := channelFixture(t)
	th := startWorker(t, f)
	waitQuiet(t, f, svc, th.ID)
	_, err := svc.BindChannel("telegram", "1", "", th.ID, "")
	require.NoError(t, err)

	_, err = f.threads.Resolve(th.ID)
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, boundOf(t, f, "1"))

	_, reply, err := svc.ChannelCommand("telegram", "1", ChatSession, "1")
	require.NoError(t, err)
	assert.Contains(t, reply, "is done")
	assert.Equal(t, f.project.MainThreadID, boundOf(t, f, "1"), "a done thread is not focused")
}

func TestStopNamesAThread(t *testing.T) {
	started := make(chan struct{}, 2)
	f, svc, _ := channelFixtureWith(t, longScript(started))
	th, err := f.threads.Start(context.Background(), f.project, StartSpec{Title: "Long", Task: "long", Repo: "none", Origin: "coordinator"})
	require.NoError(t, err)
	<-started
	_, err = svc.BindChannel("telegram", "1", "", "", f.project.ID)
	require.NoError(t, err)

	_, reply, err := svc.ChannelCommand("telegram", "1", ChatStop, "#1")
	require.NoError(t, err)
	assert.Equal(t, "Stopped [#1 Long].", reply)
	waitStatus(t, f.store, th.ID, db.ThreadIdle)

	_, reply, err = svc.ChannelCommand("telegram", "1", ChatStop, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "cannot be stopped", "without a number it is the project conversation")
}

// longScript says ok, except a prompt with "long" runs until stopped,
// signalling started first.
func longScript(started chan<- struct{}) func(turn *enginetest.Turn) error {
	return func(turn *enginetest.Turn) error {
		if strings.Contains(turn.Prompt, "long") {
			started <- struct{}{}
			<-turn.Ctx.Done()
			return turn.Ctx.Err()
		}
		turn.Say("ok", 1, 1)
		return nil
	}
}
