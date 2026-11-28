package project

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

func TestAMutedChatHearsNothingAndStaysBound(t *testing.T) {
	f, svc, sink := channelFixture(t)
	muted, err := svc.BindChannel("telegram", "quiet", "Quiet", "", f.project.ID)
	require.NoError(t, err)
	_, err = svc.BindChannel("telegram", "loud", "Loud", "", f.project.ID)
	require.NoError(t, err)

	b, err := svc.MuteChannel(muted.ID, true, nil)
	require.NoError(t, err)
	assert.True(t, b.Muted)
	assert.Nil(t, b.MutedUntil)

	// Replies, reports and requests reach only the chat that is not muted.
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "hi"})
	assert.Equal(t, "loud", sink.next(t).chatID)
	worker := &db.Thread{ProjectID: f.project.ID, ParentID: f.project.MainThreadID, Title: "Fix login"}
	require.NoError(t, f.store.CreateThread(worker))
	svc.Relay.NeedsUser(*worker, approvalEvent("r1"))
	assert.Equal(t, "loud", sink.next(t).chatID)
	sink.none(t)
	_, _, asked := awaitingOf(svc.Relay, "telegram", "quiet")
	assert.False(t, asked, "a muted chat is not waited on for an answer it never saw")

	// The binding is untouched.
	still, err := f.store.GetChannelBinding("telegram", "quiet")
	require.NoError(t, err)
	assert.Equal(t, f.project.MainThreadID, still.ThreadID)
	assert.Equal(t, "Quiet", still.ChatTitle)

	// Unmuted, it hears again.
	b, err = svc.MuteChannel(muted.ID, false, nil)
	require.NoError(t, err)
	assert.False(t, b.Muted)
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "back"})
	first, second := sink.next(t), sink.next(t)
	assert.ElementsMatch(t, []string{"quiet", "loud"}, []string{first.chatID, second.chatID})

	// One note per change, none for a repeat.
	_, err = svc.MuteChannel(muted.ID, false, nil)
	require.NoError(t, err)
	notes := systemNotes(t, f.store, f.project.MainThreadID)
	require.Len(t, notes, 2)
	assert.Equal(t, "IM Quiet muted until unmuted: nothing is sent to it; its messages still arrive.", notes[0])
	assert.Equal(t, "IM Quiet unmuted: it hears replies and updates again.", notes[1])
}

func TestAMutedChatStillTalksAndIsToldOnceWhereRepliesGo(t *testing.T) {
	f, svc, sink := channelFixture(t)
	b, err := svc.BindChannel("telegram", "42", "Me", "", f.project.ID)
	require.NoError(t, err)
	until := time.Now().Add(2 * time.Hour)
	_, err = svc.MuteChannel(b.ID, true, &until)
	require.NoError(t, err)

	handled, err := svc.PostFromChannel("telegram", "42", "first", nil)
	require.NoError(t, err)
	require.True(t, handled)
	notice := sink.next(t)
	assert.True(t, strings.HasPrefix(notice.text, "This chat is muted until "), notice.text)
	assert.Contains(t, notice.text, "Bind page")
	waitQuiet(t, f, svc, f.project.MainThreadID)

	_, err = svc.PostFromChannel("telegram", "42", "second", nil)
	require.NoError(t, err)
	waitQuiet(t, f, svc, f.project.MainThreadID)
	sink.none(t)

	// Both messages reached the coordinator, which answered both on the web.
	var users, replies int
	for _, e := range convo(t, f.store, f.project.ID) {
		switch e.Kind {
		case db.EvUser:
			users++
		case db.EvAssistant:
			replies++
		}
	}
	assert.Equal(t, 2, users)
	assert.Equal(t, 2, replies)

	// /status says so; a new mute tells the chat again.
	_, reply, err := svc.ChannelCommand("telegram", "42", ChatStatus, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "Muted: until ")
	_, err = svc.MuteChannel(b.ID, true, nil)
	require.NoError(t, err)
	_, err = svc.PostFromChannel("telegram", "42", "third", nil)
	require.NoError(t, err)
	assert.Contains(t, sink.next(t).text, "muted until unmuted")
}

func TestAMuteThatRunsOutLetsTheChatHearAgain(t *testing.T) {
	f, svc, sink := channelFixture(t)
	b, err := svc.BindChannel("telegram", "42", "", "", f.project.ID)
	require.NoError(t, err)

	past := time.Now().Add(-time.Second)
	_, err = svc.MuteChannel(b.ID, true, &past)
	assert.ErrorIs(t, err, ErrMutePast)
	_, err = svc.MuteChannel("nope", true, nil)
	assert.ErrorIs(t, err, db.ErrNotFound)

	soon := time.Now().Add(300 * time.Millisecond)
	_, err = svc.MuteChannel(b.ID, true, &soon)
	require.NoError(t, err)
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "while muted"})
	sink.none(t)

	time.Sleep(time.Until(soon) + 20*time.Millisecond)
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "after"})
	assert.Equal(t, "after\n\n[Demo]", sink.next(t).text)
	got, err := f.store.GetChannelBinding("telegram", "42")
	require.NoError(t, err)
	assert.False(t, got.Muted, "a mute that ran out reads as unmuted")
}

func TestAChatMutesAndUnmutesItself(t *testing.T) {
	f, svc, sink := channelFixture(t)
	_, err := svc.BindChannel("telegram", "42", "Me", "", f.project.ID)
	require.NoError(t, err)

	_, reply, err := svc.ChannelCommand("telegram", "42", ChatMute, "soon")
	require.NoError(t, err)
	assert.Equal(t, muteUsage, reply)

	before := time.Now()
	handled, reply, err := svc.ChannelCommand("telegram", "42", ChatMute, "8h")
	require.NoError(t, err)
	require.True(t, handled)
	assert.True(t, strings.HasPrefix(reply, "Muted until "), reply)
	assert.Contains(t, reply, "/unmute")
	b, err := f.store.GetChannelBinding("telegram", "42")
	require.NoError(t, err)
	require.True(t, b.Muted)
	require.NotNil(t, b.MutedUntil)
	assert.WithinDuration(t, before.Add(8*time.Hour), *b.MutedUntil, 5*time.Second)

	// It said so already: writing next is not answered with the notice.
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "quiet"})
	_, err = svc.PostFromChannel("telegram", "42", "hello", nil)
	require.NoError(t, err)
	waitQuiet(t, f, svc, f.project.MainThreadID)
	sink.none(t)

	_, reply, err = svc.ChannelCommand("telegram", "42", ChatMute, "")
	require.NoError(t, err)
	assert.Contains(t, reply, "Muted until unmuted")
	_, reply, err = svc.ChannelCommand("telegram", "42", ChatMute, "tomorrow")
	require.NoError(t, err)
	b, err = f.store.GetChannelBinding("telegram", "42")
	require.NoError(t, err)
	require.NotNil(t, b.MutedUntil)
	assert.Equal(t, 9, b.MutedUntil.Local().Hour(), reply)

	_, reply, err = svc.ChannelCommand("telegram", "42", ChatUnmute, "")
	require.NoError(t, err)
	assert.Equal(t, "Unmuted: replies and updates come here again.", reply)
	_, reply, err = svc.ChannelCommand("telegram", "42", ChatUnmute, "")
	require.NoError(t, err)
	assert.Equal(t, "This chat is not muted.", reply)
	svc.Relay.Message(db.ThreadEvent{ThreadID: f.project.MainThreadID, Kind: db.EvAssistant, Content: "loud"})
	assert.Equal(t, "loud\n\n[Demo]", sink.next(t).text)
}
