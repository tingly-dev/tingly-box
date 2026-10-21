package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/server/module"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/projects/llm"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/swagger"
)

// fakeChannel is a bot's channel that records what it was asked to send.
type fakeChannel struct {
	id, platform string
	err          error
	sent         chan sent
}

type sent struct{ chatID, body string }

func newFakeChannel(id, platform string) *fakeChannel {
	return &fakeChannel{id: id, platform: platform, sent: make(chan sent, 16)}
}

func (c *fakeChannel) ID() string                         { return c.id }
func (c *fakeChannel) Platform() string                   { return c.platform }
func (c *fakeChannel) Capabilities() channel.Capabilities { return channel.Capabilities{} }
func (c *fakeChannel) Send(_ context.Context, t channel.Target, n interaction.Notification) error {
	if c.err != nil {
		return c.err
	}
	c.sent <- sent{t.ChatID, n.Body}
	return nil
}
func (c *fakeChannel) Prompt(context.Context, channel.Target, interaction.Interaction) (interaction.Reply, error) {
	return interaction.Reply{}, errors.New("not used")
}

func (c *fakeChannel) next(t *testing.T) sent {
	t.Helper()
	select {
	case s := <-c.sent:
		return s
	case <-time.After(5 * time.Second):
		t.Fatal("nothing sent through the channel")
		return sent{}
	}
}

func TestBridgeSendsThroughTheBotTheChatWroteThrough(t *testing.T) {
	reg := channel.NewRegistry()
	one, two := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	reg.Register(one)
	reg.Register(two)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, nil)

	// The chat last wrote through bot-2, so replies go back through it.
	b.mu.Lock()
	b.bots[chatKey("telegram", "42")] = "bot-2"
	b.mu.Unlock()
	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "42", "hello"))
	assert.Equal(t, sent{"42", "hello"}, two.next(t))
	assert.Empty(t, one.sent)
}

func TestBridgeFallsBackToThePlatformsBots(t *testing.T) {
	reg := channel.NewRegistry()
	broken, ok := newFakeChannel("bot-1", "telegram"), newFakeChannel("bot-2", "telegram")
	broken.err = errors.New("unknown chat")
	reg.Register(broken)
	reg.Register(ok)
	reg.Register(newFakeChannel("bot-3", "feishu"))
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, nil)

	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "42", "hi"))
	assert.Equal(t, sent{"42", "hi"}, ok.next(t))
	b.mu.RLock()
	assert.Equal(t, "bot-2", b.bots[chatKey("telegram", "42")], "the bot that worked is remembered")
	b.mu.RUnlock()

	err := b.SendChannelText(context.Background(), "slack", "1", "x")
	assert.ErrorIs(t, err, ErrNoChannel)
	assert.ErrorIs(t, NewChatBridge().SendChannelText(context.Background(), "telegram", "1", "x"), ErrNoChannel)
}

func TestBridgeNeverSendsToABlockedChat(t *testing.T) {
	reg := channel.NewRegistry()
	ch := newFakeChannel("bot-1", "telegram")
	reg.Register(ch)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, func(chatID string) bool { return chatID == "blocked" })

	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "blocked", "x"))
	assert.Empty(t, ch.sent)
	require.NoError(t, b.SendChannelText(context.Background(), "telegram", "fine", "x"))
	assert.Equal(t, sent{"fine", "x"}, ch.next(t))
}

// Before Bind, or with the Projects flag off, the bridge takes nothing.
func TestBridgeIsInertWithoutProjects(t *testing.T) {
	b := NewChatBridge()
	assert.False(t, b.Bound("telegram", "1"))
	handled, err := b.RouteInbound(context.Background(), remoteagent.ProjectInbound{Platform: "telegram", ChatID: "1", Text: "hi"})
	assert.False(t, handled)
	assert.NoError(t, err)
}

// echoHost answers the coordinator with an echo and sends chat text through
// the bridge, like Host does.
type echoHost struct {
	nopHost
	bridge *ChatBridge
}

func (echoHost) ClientFor(context.Context, string) (llm.Client, string, error) {
	return echoClient{}, "echo", nil
}

func (h echoHost) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	return h.bridge.SendChannelText(ctx, platform, chatID, text)
}

type echoClient struct{}

func (echoClient) Stream(_ context.Context, req llm.Request, onDelta func(string)) (llm.Result, error) {
	last := ""
	for _, m := range req.Messages {
		if m.Role == "user" {
			last = m.Content
		}
	}
	text := "You said: " + last
	onDelta(text)
	return llm.Result{Text: text}, nil
}
func (echoClient) ListModels(context.Context) ([]llm.Model, error) { return nil, nil }

// The whole path with a real Projects app: bind over HTTP, write from the chat,
// the coordinator answers, and the reply arrives in the chat with the project's
// name; a chat nobody bound is left alone; the flag off turns it all off.
func TestChatToProjectAndBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bridge := NewChatBridge()
	app, err := projectsapp.New(projectsapp.Options{Host: echoHost{bridge: bridge}})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })

	reg := channel.NewRegistry()
	bot := newFakeChannel("bot-1", "telegram")
	reg.Register(bot)
	on := true
	bridge.Bind(app, reg, func() bool { return on }, nil, nil)

	engine := gin.New()
	module.Mount(&module.Routes{Manager: swagger.NewRouteManager(engine)}, NewHandler(app, func() bool { return true }, nil))
	call := func(method, path string, body any, out any) int {
		var r *bytes.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		} else {
			r = bytes.NewReader(nil)
		}
		req := httptest.NewRequest(method, "/api/v1"+path, r)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		engine.ServeHTTP(w, req)
		if out != nil {
			require.NoError(t, json.Unmarshal(w.Body.Bytes(), out), w.Body.String())
		}
		return w.Code
	}

	var p struct {
		ID           string `json:"id"`
		MainThreadID string `json:"main_thread_id"`
	}
	require.Equal(t, http.StatusCreated, call("POST", "/projects", map[string]any{"name": "Atlas"}, &p))
	ctx := context.Background()
	in := func(chatID, text string) remoteagent.ProjectInbound {
		return remoteagent.ProjectInbound{Platform: "telegram", BotUUID: "bot-1", ChatID: chatID, Text: text}
	}

	// Not bound yet: not handled, nothing sent.
	handled, err := bridge.RouteInbound(ctx, in("42", "hello"))
	require.NoError(t, err)
	assert.False(t, handled)
	assert.False(t, bridge.Bound("telegram", "42"))

	require.Equal(t, http.StatusOK, call("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "42", "project_id": p.ID}, nil))
	assert.True(t, bridge.Bound("telegram", "42"))
	assert.False(t, bridge.Bound("telegram", "43"), "another chat is not bound")

	handled, err = bridge.RouteInbound(ctx, in("42", "status?"))
	require.NoError(t, err)
	require.True(t, handled)
	assert.Equal(t, sent{"42", "[Atlas] You said: status?"}, bot.next(t))

	// The conversation the web shows has both sides, the first marked as IM.
	var msgs struct {
		Events []struct {
			Kind         string `json:"kind"`
			Source       string `json:"source"`
			SourceChatID string `json:"source_chat_id"`
		} `json:"events"`
	}
	require.Eventually(t, func() bool {
		call("GET", "/threads/"+p.MainThreadID+"/events", nil, &msgs)
		return len(msgs.Events) == 2
	}, 5*time.Second, 20*time.Millisecond)
	assert.Equal(t, "user", msgs.Events[0].Kind)
	assert.Equal(t, "im", msgs.Events[0].Source)
	assert.Equal(t, "42", msgs.Events[0].SourceChatID)
	assert.Equal(t, "assistant", msgs.Events[1].Kind)

	// Unbinding returns the chat to the usual routing.
	var list struct {
		Bindings []struct {
			ID string `json:"id"`
		} `json:"bindings"`
	}
	call("GET", "/projects/channel-bindings", nil, &list)
	require.Len(t, list.Bindings, 1)
	require.Equal(t, http.StatusOK, call("DELETE", "/projects/channel-bindings/"+list.Bindings[0].ID, nil, nil))
	handled, _ = bridge.RouteInbound(ctx, in("42", "again"))
	assert.False(t, handled)

	// With Projects switched off a bound chat is also left alone.
	require.Equal(t, http.StatusOK, call("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "42", "project_id": p.ID}, nil))
	on = false
	handled, _ = bridge.RouteInbound(ctx, in("42", "off"))
	assert.False(t, handled)
	assert.False(t, bridge.Bound("telegram", "42"))
}

// actionFake is a channel with buttons, like imchannel.Channel.
type actionFake struct {
	*fakeChannel
	noButtons bool
	actions   chan []imchannel.Button
	restated  chan sent
}

func (c *actionFake) SendActions(_ context.Context, chatID, text string, buttons []imchannel.Button) (string, error) {
	if c.noButtons {
		return "", imchannel.ErrNoButtons
	}
	c.actions <- buttons
	return "m-" + chatID, nil
}

func (c *actionFake) Restate(_ context.Context, chatID, messageID, text string) error {
	c.restated <- sent{chatID + "/" + messageID, text}
	return nil
}

func TestBridgeSendsPromptsWithButtonsAndRestatesThroughTheSameBot(t *testing.T) {
	reg := channel.NewRegistry()
	plain := newFakeChannel("bot-1", "telegram")
	rich := &actionFake{fakeChannel: newFakeChannel("bot-2", "telegram"), actions: make(chan []imchannel.Button, 4), restated: make(chan sent, 4)}
	reg.Register(plain)
	reg.Register(rich)
	b := NewChatBridge()
	b.Bind(nil, reg, nil, nil, func(chatID string) bool { return chatID == "blocked" })
	b.mu.Lock()
	b.bots[chatKey("telegram", "42")] = "bot-2"
	b.mu.Unlock()

	p := projectsapp.ChannelPrompt{ThreadID: "t1", RequestID: "r1", Text: "Approve: ls", Choices: []projectsapp.ChannelChoice{{Value: "allow", Label: "Allow"}, {Value: "deny", Label: "Deny"}}}
	ref, err := b.SendChannelPrompt(context.Background(), "telegram", "42", p)
	require.NoError(t, err)
	got := <-rich.actions
	require.Len(t, got, 2)
	assert.Equal(t, "Allow", got[0].Label)
	assert.Equal(t, []string{remoteagent.ProjectAnswerPayload, "t1", "r1", "allow"}, got[0].Payload)

	require.NoError(t, b.RestateChannelPrompt(context.Background(), "telegram", "42", ref, "Approve: ls\n\nAllowed."))
	assert.Equal(t, sent{"42/m-42", "Approve: ls\n\nAllowed."}, <-rich.restated)

	// A bot without buttons is not a prompter: the caller falls back to text.
	b.mu.Lock()
	b.bots[chatKey("telegram", "7")] = "bot-1"
	b.mu.Unlock()
	_, err = b.SendChannelPrompt(context.Background(), "telegram", "7", p)
	assert.ErrorIs(t, err, projectsapp.ErrNoButtons)
	rich.noButtons = true
	_, err = b.SendChannelPrompt(context.Background(), "telegram", "42", p)
	assert.ErrorIs(t, err, projectsapp.ErrNoButtons, "nor is a platform without them")

	ref, err = b.SendChannelPrompt(context.Background(), "telegram", "blocked", p)
	require.NoError(t, err)
	assert.Empty(t, ref, "a blocked chat gets nothing")
}
