package peerconsumer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/peer"
)

// fakeAgentState is an in-memory CurrentAgent map.
type fakeAgentState struct {
	agents map[string]string
}

func newFakeAgentState() *fakeAgentState { return &fakeAgentState{agents: map[string]string{}} }

func (f *fakeAgentState) SetCurrentAgent(chatID, platform, agentType string) error {
	f.agents[chatID] = agentType
	return nil
}

func (f *fakeAgentState) GetCurrentAgent(chatID string) (string, error) {
	return f.agents[chatID], nil
}

const (
	testBot  = "bot-1"
	testChat = "chat-1"
)

type fixture struct {
	consumer *Consumer
	store    *peer.MemStore
	inbox    *peer.Inbox
	sends    *peer.RecentSends
	state    *fakeAgentState
	mgr      *imbot.Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := peer.NewMemStore()
	inbox := peer.NewInbox(store)
	sends := peer.NewRecentSends(16)
	return &fixture{
		consumer: New(store, inbox, sends),
		store:    store,
		inbox:    inbox,
		sends:    sends,
		state:    newFakeAgentState(),
		mgr:      imbot.NewManager(), // no bots: outbound send/react are no-ops
	}
}

func (f *fixture) addPeer(t *testing.T, name string, exclusive bool) peer.Peer {
	t.Helper()
	p := peer.Peer{
		Name: name, BotUUID: testBot, ChatID: testChat, Enabled: true, Exclusive: exclusive,
	}
	if err := f.store.Create(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func textMsg(text string) imbot.Message {
	return imbot.Message{
		ID:        "m-in",
		Recipient: imbot.Recipient{ID: testChat},
		Sender:    imbot.Sender{ID: "user-1"},
		Content:   imbot.NewTextContent(text),
	}
}

func (f *fixture) handle(msg imbot.Message) bool {
	return f.consumer.handle(msg, imbot.Platform("telegram"), testBot, f.mgr, f.state)
}

func (f *fixture) drain(t *testing.T, peerUUID string) []peer.Update {
	t.Helper()
	updates, err := f.inbox.Poll(context.Background(), peerUUID, 0, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	// Confirm what we read so the next drain sees only new updates.
	if len(updates) > 0 {
		if err := f.store.AckUpdates(peerUUID, updates[len(updates)-1].ID); err != nil {
			t.Fatal(err)
		}
	}
	return updates
}

func TestUnboundChatNeverClaimed(t *testing.T) {
	f := newFixture(t)
	// Peer bound to ANOTHER chat.
	p := peer.Peer{Name: "report", BotUUID: testBot, ChatID: "other-chat", Enabled: true, Exclusive: true}
	if err := f.store.Create(&p); err != nil {
		t.Fatal(err)
	}
	if f.handle(textMsg("@report hello")) {
		t.Fatal("claimed a message in an unbound chat")
	}
	if f.handle(textMsg("plain text")) {
		t.Fatal("claimed plain text in an unbound chat")
	}
}

func TestDisabledPeerNeverClaimed(t *testing.T) {
	f := newFixture(t)
	p := f.addPeer(t, "report", true)
	p.Enabled = false
	if err := f.store.Save(&p); err != nil {
		t.Fatal(err)
	}
	if f.handle(textMsg("plain text")) {
		t.Fatal("claimed for a disabled peer")
	}
}

func TestPassRules(t *testing.T) {
	f := newFixture(t)
	f.addPeer(t, "report", false)

	for _, text := range []string{
		"@cc fix the bug", "@tb", "/cc", "/tb", "cc", "tb", // agent handoffs
		"/help", "/stop", "/bind 1234", // slash commands
		"plain text with no addressing", // non-exclusive, not sticky
	} {
		if f.handle(textMsg(text)) {
			t.Errorf("claimed %q, want pass", text)
		}
	}

	// Callbacks pass even in a bound chat.
	cb := textMsg("x")
	cb.Metadata = map[string]interface{}{"is_callback": true}
	if f.handle(cb) {
		t.Error("claimed a callback")
	}
}

func TestMentionHandoffSticky(t *testing.T) {
	f := newFixture(t)
	p := f.addPeer(t, "report", false)

	// Bare mention: sticky set, nothing enqueued.
	if !f.handle(textMsg("@report")) {
		t.Fatal("bare mention not claimed")
	}
	if agent := f.state.agents[testChat]; agent != "peer:"+p.UUID {
		t.Fatalf("CurrentAgent = %q", agent)
	}
	if updates := f.drain(t, p.UUID); len(updates) != 0 {
		t.Fatalf("bare mention enqueued %+v", updates)
	}

	// Plain text now follows the sticky peer.
	if !f.handle(textMsg("status please")) {
		t.Fatal("sticky message not claimed")
	}
	updates := f.drain(t, p.UUID)
	if len(updates) != 1 || updates[0].Text != "status please" || updates[0].SenderID != "user-1" {
		t.Fatalf("sticky updates = %+v", updates)
	}
	if updates[0].Type != peer.UpdateTypeMessage {
		t.Fatalf("update type = %q", updates[0].Type)
	}

	// Mention with trailing text enqueues the trailing part only.
	if !f.handle(textMsg("@report run job 7")) {
		t.Fatal("mention-with-text not claimed")
	}
	updates = f.drain(t, p.UUID)
	if len(updates) != 1 || updates[0].Text != "run job 7" {
		t.Fatalf("mention trailing updates = %+v", updates)
	}

	// Case-insensitive mention.
	if !f.handle(textMsg("@Report ping")) {
		t.Fatal("case-insensitive mention not claimed")
	}
}

func TestStickySelfHealWhenTargetGone(t *testing.T) {
	f := newFixture(t)
	f.addPeer(t, "keeper", false) // keeps the chat bound so rules run
	f.state.agents[testChat] = "peer:deleted-uuid"

	if f.handle(textMsg("hello")) {
		t.Fatal("claimed for a dead sticky target")
	}
	if agent := f.state.agents[testChat]; agent != "" {
		t.Fatalf("CurrentAgent not reset: %q", agent)
	}
}

func TestReplyToRouting(t *testing.T) {
	f := newFixture(t)
	p := f.addPeer(t, "report", false)
	f.sends.Track(testChat, "out-42", p.UUID)

	msg := textMsg("looks wrong, rerun?")
	msg.ThreadContext = &imbot.ThreadContext{ParentMessageID: "out-42"}
	if !f.handle(msg) {
		t.Fatal("reply-to not claimed")
	}
	updates := f.drain(t, p.UUID)
	if len(updates) != 1 || updates[0].Text != "looks wrong, rerun?" {
		t.Fatalf("reply-to updates = %+v", updates)
	}
	// Sticky state untouched by reply-to.
	if agent := f.state.agents[testChat]; agent != "" {
		t.Fatalf("reply-to changed sticky state: %q", agent)
	}

	// Reply to an untracked message: not claimed (non-exclusive chat).
	other := textMsg("random reply")
	other.ThreadContext = &imbot.ThreadContext{ParentMessageID: "unknown"}
	if f.handle(other) {
		t.Fatal("claimed reply to unknown message")
	}
}

func TestExclusiveBinding(t *testing.T) {
	f := newFixture(t)
	p := f.addPeer(t, "report", true)

	if !f.handle(textMsg("anything at all")) {
		t.Fatal("exclusive chat message not claimed")
	}
	updates := f.drain(t, p.UUID)
	if len(updates) != 1 || updates[0].Text != "anything at all" {
		t.Fatalf("exclusive updates = %+v", updates)
	}

	// Commands and agent handoffs still pass even in an exclusive chat.
	for _, text := range []string{"/help", "@cc do it", "@tb"} {
		if f.handle(textMsg(text)) {
			t.Errorf("exclusive chat claimed %q, want pass", text)
		}
	}
}

func TestPeersCommandClaimed(t *testing.T) {
	f := newFixture(t)
	f.addPeer(t, "report", false)
	if !f.handle(textMsg("/peers")) {
		t.Fatal("/peers not claimed in a bound chat")
	}
	// And passes in an unbound chat.
	f2 := newFixture(t)
	if f2.handle(textMsg("/peers")) {
		t.Fatal("/peers claimed in an unbound chat")
	}
}

func TestMounted(t *testing.T) {
	f := newFixture(t)
	setting := bot.BotSetting{UUID: testBot}
	if f.consumer.Mounted(setting) {
		t.Fatal("mounted with no peers")
	}
	p := f.addPeer(t, "report", false)
	if !f.consumer.Mounted(setting) {
		t.Fatal("not mounted with an enabled peer")
	}
	p.Enabled = false
	if err := f.store.Save(&p); err != nil {
		t.Fatal(err)
	}
	if f.consumer.Mounted(setting) {
		t.Fatal("mounted with only a disabled peer")
	}
}

func TestOfflineNoticeFiresViaConsumerEnqueue(t *testing.T) {
	f := newFixture(t)
	p := f.addPeer(t, "report", true)
	noticed := make(chan peer.Update, 1)
	f.inbox.SetOfflineNotifier(func(_ peer.Peer, u peer.Update) {
		noticed <- u
	})
	if !f.handle(textMsg("are you there")) {
		t.Fatal("not claimed")
	}
	select {
	case u := <-noticed:
		if u.PeerUUID != p.UUID || !strings.Contains(u.Text, "are you there") {
			t.Fatalf("notice update = %+v", u)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("offline notice not fired")
	}
}
