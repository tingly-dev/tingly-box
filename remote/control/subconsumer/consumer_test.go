package subconsumer

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/subscription"
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
	store    *subscription.MemStore
	mailbox  *subscription.Mailbox
	sends    *subscription.RecentSends
	state    *fakeAgentState
	mgr      *imbot.Manager
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	store := subscription.NewMemStore()
	mailbox := subscription.NewMailbox(store)
	sends := subscription.NewRecentSends(16)
	return &fixture{
		consumer: New(store, mailbox, sends),
		store:    store,
		mailbox:  mailbox,
		sends:    sends,
		state:    newFakeAgentState(),
		mgr:      imbot.NewManager(), // no bots: outbound send/react are no-ops
	}
}

func (f *fixture) addSub(t *testing.T, name string, exclusive bool) subscription.Subscription {
	t.Helper()
	sub := subscription.Subscription{
		Name: name, BotUUID: testBot, ChatID: testChat, Enabled: true, Exclusive: exclusive,
	}
	if err := f.store.Create(&sub); err != nil {
		t.Fatal(err)
	}
	return sub
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

func (f *fixture) drain(t *testing.T, subUUID string) []subscription.Event {
	t.Helper()
	events, err := f.mailbox.Poll(context.Background(), subUUID, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	return events
}

func TestUnboundChatNeverClaimed(t *testing.T) {
	f := newFixture(t)
	// Subscription bound to ANOTHER chat.
	sub := subscription.Subscription{Name: "report", BotUUID: testBot, ChatID: "other-chat", Enabled: true, Exclusive: true}
	if err := f.store.Create(&sub); err != nil {
		t.Fatal(err)
	}
	if f.handle(textMsg("@report hello")) {
		t.Fatal("claimed a message in an unbound chat")
	}
	if f.handle(textMsg("plain text")) {
		t.Fatal("claimed plain text in an unbound chat")
	}
}

func TestDisabledSubscriptionNeverClaimed(t *testing.T) {
	f := newFixture(t)
	sub := f.addSub(t, "report", true)
	sub.Enabled = false
	if err := f.store.Update(&sub); err != nil {
		t.Fatal(err)
	}
	if f.handle(textMsg("plain text")) {
		t.Fatal("claimed for a disabled subscription")
	}
}

func TestPassRules(t *testing.T) {
	f := newFixture(t)
	f.addSub(t, "report", false)

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
	sub := f.addSub(t, "report", false)

	// Bare mention: sticky set, nothing enqueued.
	if !f.handle(textMsg("@report")) {
		t.Fatal("bare mention not claimed")
	}
	if agent := f.state.agents[testChat]; agent != "sub:"+sub.UUID {
		t.Fatalf("CurrentAgent = %q", agent)
	}
	if events := f.drain(t, sub.UUID); len(events) != 0 {
		t.Fatalf("bare mention enqueued %+v", events)
	}

	// Plain text now follows the sticky peer.
	if !f.handle(textMsg("status please")) {
		t.Fatal("sticky message not claimed")
	}
	events := f.drain(t, sub.UUID)
	if len(events) != 1 || events[0].Text != "status please" || events[0].SenderID != "user-1" {
		t.Fatalf("sticky events = %+v", events)
	}

	// Mention with trailing text enqueues the trailing part only.
	if !f.handle(textMsg("@report run job 7")) {
		t.Fatal("mention-with-text not claimed")
	}
	events = f.drain(t, sub.UUID)
	if len(events) != 2 || events[1].Text != "run job 7" {
		t.Fatalf("mention trailing events = %+v", events)
	}

	// Case-insensitive mention.
	if !f.handle(textMsg("@Report ping")) {
		t.Fatal("case-insensitive mention not claimed")
	}
}

func TestStickySelfHealWhenTargetGone(t *testing.T) {
	f := newFixture(t)
	f.addSub(t, "keeper", false) // keeps the chat bound so rules run
	f.state.agents[testChat] = "sub:deleted-uuid"

	if f.handle(textMsg("hello")) {
		t.Fatal("claimed for a dead sticky target")
	}
	if agent := f.state.agents[testChat]; agent != "" {
		t.Fatalf("CurrentAgent not reset: %q", agent)
	}
}

func TestReplyToRouting(t *testing.T) {
	f := newFixture(t)
	sub := f.addSub(t, "report", false)
	f.sends.Track(testChat, "out-42", sub.UUID)

	msg := textMsg("looks wrong, rerun?")
	msg.ThreadContext = &imbot.ThreadContext{ParentMessageID: "out-42"}
	if !f.handle(msg) {
		t.Fatal("reply-to not claimed")
	}
	events := f.drain(t, sub.UUID)
	if len(events) != 1 || events[0].Text != "looks wrong, rerun?" {
		t.Fatalf("reply-to events = %+v", events)
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
	sub := f.addSub(t, "report", true)

	if !f.handle(textMsg("anything at all")) {
		t.Fatal("exclusive chat message not claimed")
	}
	events := f.drain(t, sub.UUID)
	if len(events) != 1 || events[0].Text != "anything at all" {
		t.Fatalf("exclusive events = %+v", events)
	}

	// Commands and agent handoffs still pass even in an exclusive chat.
	for _, text := range []string{"/help", "@cc do it", "@tb"} {
		if f.handle(textMsg(text)) {
			t.Errorf("exclusive chat claimed %q, want pass", text)
		}
	}
}

func TestSubsCommandClaimed(t *testing.T) {
	f := newFixture(t)
	f.addSub(t, "report", false)
	if !f.handle(textMsg("/subs")) {
		t.Fatal("/subs not claimed in a bound chat")
	}
	// And passes in an unbound chat.
	f2 := newFixture(t)
	if f2.handle(textMsg("/subs")) {
		t.Fatal("/subs claimed in an unbound chat")
	}
}

func TestMounted(t *testing.T) {
	f := newFixture(t)
	setting := bot.BotSetting{UUID: testBot}
	if f.consumer.Mounted(setting) {
		t.Fatal("mounted with no subscriptions")
	}
	sub := f.addSub(t, "report", false)
	if !f.consumer.Mounted(setting) {
		t.Fatal("not mounted with an enabled subscription")
	}
	sub.Enabled = false
	if err := f.store.Update(&sub); err != nil {
		t.Fatal(err)
	}
	if f.consumer.Mounted(setting) {
		t.Fatal("mounted with only a disabled subscription")
	}
}

func TestOfflineNoticeFiresViaConsumerEnqueue(t *testing.T) {
	f := newFixture(t)
	sub := f.addSub(t, "report", true)
	noticed := make(chan subscription.Event, 1)
	f.mailbox.SetOfflineNotifier(func(_ subscription.Subscription, ev subscription.Event) {
		noticed <- ev
	})
	if !f.handle(textMsg("are you there")) {
		t.Fatal("not claimed")
	}
	select {
	case ev := <-noticed:
		if ev.SubscriptionUUID != sub.UUID || !strings.Contains(ev.Text, "are you there") {
			t.Fatalf("notice event = %+v", ev)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("offline notice not fired")
	}
}
