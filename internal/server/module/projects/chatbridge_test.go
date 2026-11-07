package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/engine/enginetest"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/bridge"
	"github.com/tingly-dev/tingly-box/remote/channel"
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

// echoHost sends chat text through the bridge, like Host does.
type echoHost struct {
	nopHost
	bridge *bridge.ChatBridge
}

func (h echoHost) SendChannelText(ctx context.Context, platform, chatID, text string) error {
	return h.bridge.SendChannelText(ctx, platform, chatID, text)
}

// lastEvent is the body of the last <event> envelope in a coordinator prompt.
var lastEvent = regexp.MustCompile(`(?s)<event[^>]*>\n(.*?)\n</event>`)

// echoAgents stands in for Claude Code: the coordinator's session answers
// with an echo of the message that woke it. A real model reads the event's
// content rather than repeating its envelope in the answer.
func echoAgents() *agentboot.AgentService {
	return (&enginetest.Agent{Script: func(turn *enginetest.Turn) error {
		if !turn.Opts.StrictMcpConfig {
			return nil // a thread
		}
		turn.Init("echo")
		last := ""
		if m := lastEvent.FindAllStringSubmatch(turn.Prompt, -1); len(m) > 0 {
			last = m[len(m)-1][1]
		}
		turn.Say("You said: "+last, 1, 1)
		return nil
	}}).Service()
}

// The whole path with a real Projects app: bind over HTTP, write from the chat,
// the coordinator answers, and the reply arrives in the chat with the project's
// name; a chat nobody bound is left alone; the flag off turns it all off.
func TestChatToProjectAndBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	bridge := bridge.NewChatBridge()
	app, err := projectsapp.New(projectsapp.Options{Host: echoHost{bridge: bridge}, Agents: echoAgents()})
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
	post := func(chatID, text string) (bool, error) {
		return bridge.Post(projectsapp.ChatRef{Platform: "telegram", BotUUID: "bot-1", ChatID: chatID}, text, nil)
	}

	// Not bound yet: not handled, nothing sent.
	handled, err := post("42", "hello")
	require.NoError(t, err)
	assert.False(t, handled)
	assert.False(t, bridge.Bound("telegram", "42"))

	require.Equal(t, http.StatusOK, call("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "42", "project_id": p.ID}, nil))
	assert.True(t, bridge.Bound("telegram", "42"))
	assert.False(t, bridge.Bound("telegram", "43"), "another chat is not bound")

	handled, err = post("42", "status?")
	require.NoError(t, err)
	require.True(t, handled)
	got := bot.next(t)
	assert.Equal(t, "42", got.chatID)
	assert.Contains(t, got.body, "status?")
	assert.True(t, strings.HasSuffix(got.body, "\n\n[Atlas]"), got.body)

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
	handled, _ = post("42", "again")
	assert.False(t, handled)

	// With Projects switched off a bound chat is also left alone.
	require.Equal(t, http.StatusOK, call("PUT", "/projects/channel-bindings", map[string]any{"platform": "telegram", "chat_id": "42", "project_id": p.ID}, nil))
	on = false
	handled, _ = post("42", "off")
	assert.False(t, handled)
	assert.False(t, bridge.Bound("telegram", "42"))
}
