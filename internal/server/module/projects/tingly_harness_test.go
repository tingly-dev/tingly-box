package projects

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	hostdb "github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	projectsapp "github.com/tingly-dev/tingly-box/projects"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/control/adapter"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
	"github.com/tingly-dev/tingly-box/swagger"
)

// This mirrors the TB host's transport facts using its live channel registry.
type harnessTransportFacts struct{ registry *channel.Registry }

func (h harnessTransportFacts) TransportFacts(id string, _ access.CapabilityName, _ access.ActionName) (access.TransportStatus, bool) {
	if _, ok := h.registry.Get(id); ok {
		return access.TransportOnline, true
	}
	return access.TransportOffline, false
}

// tinglyProjectsHarness runs the production bot manager, access store, channel
// registry, Projects HTTP handlers, session engine and the Tingly transport.
// A scripted Claude CLI speaks stream-json; no external IM or model is needed.
type tinglyProjectsHarness struct {
	env               *testenv.TestEnv
	bridge            *ChatBridge
	stores            *hostdb.StoreManager
	otherID, chosenID string
	workdir           string
	enabled           *atomic.Bool
	call              func(string, string, any, any)
}

func newTinglyProjectsHarness(t *testing.T) *tinglyProjectsHarness {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the scripted Claude CLI uses a POSIX executable")
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is required for the scripted Claude CLI")
	}
	gin.SetMode(gin.TestMode)
	cli := filepath.Join(t.TempDir(), "claude")
	script := "#!" + python + `
import json, sys
if '--version' in sys.argv:
    print('2.1.218 (Claude Code)')
    sys.exit(0)
for line in sys.stdin:
    msg = json.loads(line)
    if msg.get('type') != 'user': continue
    text = msg.get('message', {}).get('content', '')
    if text == 'ask question':
        q = {'question':'Pick a color', 'header':'Color', 'options':[{'label':'Red','description':''},{'label':'Blue','description':''}], 'multiSelect':False}
        print(json.dumps({'type':'control_request','request_id':'question-1','request':{'subtype':'can_use_tool','tool_name':'AskUserQuestion','tool_use_id':'tool-1','input':{'questions':[q]}}}), flush=True)
        answer = json.loads(sys.stdin.readline())
        text = 'answered Blue' if 'Blue' in json.dumps(answer) else 'answer missing'
    if text in ('request approval', 'request denial'):
        print(json.dumps({'type':'control_request','request_id':'permission-1','request':{'subtype':'can_use_tool','tool_name':'Bash','input':{'command':'echo harness'}}}), flush=True)
        response = json.loads(sys.stdin.readline())
        text += ' approved' if response.get('response', {}).get('response', {}).get('behavior') == 'allow' else ' rejected'
    out = 'Harness reply: ' + str(text)
    print(json.dumps({'type':'assistant','message':{'id':'msg','type':'message','role':'assistant','model':'harness','content':[{'type':'text','text':out}],'usage':{'input_tokens':1,'output_tokens':1}}}), flush=True)
    print(json.dumps({'type':'result','subtype':'success','is_error':False,'result':out}), flush=True)
`
	require.NoError(t, os.WriteFile(cli, []byte(script), 0700))
	t.Setenv("CLAUDE_CLI_PATH", cli)
	env := testenv.NewTestEnv(t)
	_, otherID := env.AddTinglyBot()
	_, chosenID := env.AddTinglyBot()
	bridge := NewChatBridge()
	// Exercise TB's real Projects host against an in-process Anthropic gateway.
	gateway := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tingly/claude_code/v1/messages" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			Messages []struct {
				Role    string `json:"role"`
				Content []struct {
					Text string `json:"text"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		last := ""
		for _, message := range body.Messages {
			if message.Role == "user" {
				for _, block := range message.Content {
					last = block.Text
				}
			}
		}
		if strings.HasPrefix(last, "<event ") && strings.HasSuffix(last, "\n</event>") {
			_, last, _ = strings.Cut(last, "\n")
			last = strings.TrimSuffix(last, "\n</event>")
		}
		reply, _ := json.Marshal("You said: " + last)
		w.Header().Set("Content-Type", "text/event-stream")
		emit := func(kind, data string) { _, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", kind, data) }
		emit("message_start", `{"type":"message_start","message":{"id":"m1","type":"message","role":"assistant","model":"harness","content":[],"stop_reason":null,"usage":{"input_tokens":1,"output_tokens":0}}}`)
		emit("content_block_start", `{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`)
		emit("content_block_delta", fmt.Sprintf(`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":%s}}`, reply))
		emit("content_block_stop", `{"type":"content_block_stop","index":0}`)
		emit("message_delta", `{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":1}}`)
		emit("message_stop", `{"type":"message_stop"}`)
	}))
	t.Cleanup(gateway.Close)
	cfg := newConfig(t)
	t.Cleanup(func() { _ = cfg.CloseStores() })
	host, port, err := net.SplitHostPort(strings.TrimPrefix(gateway.URL, "http://"))
	require.NoError(t, err)
	n, err := strconv.Atoi(port)
	require.NoError(t, err)
	require.NoError(t, cfg.SetServerHost(host))
	require.NoError(t, cfg.SetServerPort(n))
	tbHost := NewHost(cfg)
	tbHost.SetChatBridge(bridge)
	app, err := projectsapp.New(projectsapp.Options{Host: tbHost, DataDir: t.TempDir()})
	require.NoError(t, err)
	t.Cleanup(func() { _ = app.Close() })
	pairing := false
	workdir := t.TempDir()
	reg := channel.NewRegistry()
	stores, err := hostdb.NewStoreManager(t.TempDir())
	require.NoError(t, err)
	t.Cleanup(func() { _ = stores.Close() })
	for _, id := range []string{otherID, chosenID} {
		_, err := stores.ImBotSettings().CreateSettings(hostdb.Settings{UUID: id, Name: id, Platform: "tingly", AuthType: "none", Auth: map[string]string{}, Enabled: true, RequirePairing: &pairing, DefaultCwd: workdir})
		require.NoError(t, err)
		require.NoError(t, stores.BotAccess().PutCapability(context.Background(), access.BotCapability{BotUUID: id, Name: access.CapabilityRemoteControl, Enabled: true}))
	}
	stores.BotAccess().SetTransportFactsSource(harnessTransportFacts{registry: reg})
	settings := adapter.NewSettingsStore(stores.ImBotSettings())
	manager := bot.NewManager(settings, bot.NewNotifyConsumer(), remoteagent.NewConsumer(settings, remoteagent.WithProjectRouter(bridge)))
	manager.SetChatStore(stores.RemoteChats())
	manager.SetAccessStore(stores.BotAccess())
	manager.SetChannelRegistry(reg)
	enabled := &atomic.Bool{}
	enabled.Store(true)
	bridge.SetTargetBlocked(stores.BotAccess().IsExternalTargetBlocked)
	bridge.Bind(app, reg, enabled.Load, func() { app.Start(env.Context()) }, stores.RemoteChats().IsChatDisabled)
	for _, id := range []string{otherID, chosenID} {
		require.NoError(t, manager.Start(env.Context(), id))
	}
	t.Cleanup(func() {
		manager.StopAll()
		for _, id := range []string{otherID, chosenID} {
			manager.WaitForStop(id, 5*time.Second)
		}
	})
	require.Eventually(t, func() bool { return reg.Len() == 2 }, 5*time.Second, 10*time.Millisecond)
	app.Start(env.Context())
	router := gin.New()
	module.Mount(&module.Routes{Manager: swagger.NewRouteManager(router)}, NewHandler(app, func() bool { return true }, nil))
	call := func(method, path string, body any, out any) {
		t.Helper()
		data, err := json.Marshal(body)
		require.NoError(t, err)
		req := httptest.NewRequest(method, "/api/v1"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Less(t, rec.Code, http.StatusBadRequest, rec.Body.String())
		if out != nil {
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), out))
		}
	}
	return &tinglyProjectsHarness{env: env, bridge: bridge, stores: stores, otherID: otherID, chosenID: chosenID, workdir: workdir, enabled: enabled, call: call}
}

func TestTinglySessionIMRoundTrip(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	env, bridge, stores, otherID, chosenID, call := h.env, h.bridge, h.stores, h.otherID, h.chosenID, h.call
	var folder struct {
		ID string `json:"id"`
	}
	call("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &folder)
	var thread struct {
		ID string `json:"id"`
	}
	call("POST", "/projects/"+folder.ID+"/threads", map[string]any{"content": "initial"}, &thread)
	var lastDetail json.RawMessage
	require.Eventually(t, func() bool {
		var detail struct {
			Thread struct {
				Status     string `json:"status"`
				WaitReason string `json:"wait_reason"`
			} `json:"thread"`
		}
		call("GET", "/threads/"+thread.ID, nil, &detail)
		lastDetail, _ = json.Marshal(detail)
		return detail.Thread.Status == "idle"
	}, 3*time.Second, 25*time.Millisecond, "last thread state: %s", &lastDetail)
	alice := env.NewUser("alice")
	selected := alice.OpenDM(chosenID)
	other := alice.OpenDM(otherID)
	call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": selected.ChatID, "bot_uuid": chosenID, "thread_id": thread.ID}, nil)
	// A stale remembered bot must not override the explicit web binding.
	bridge.mu.Lock()
	bridge.bots[chatKey("tingly", selected.ChatID)] = otherID
	bridge.mu.Unlock()
	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "from web"}, nil)
	selected.WaitText(10*time.Second).AssertContains(t, "Harness reply: from web")
	other.ExpectNoEvent(200*time.Millisecond, tingly.EventSend)
	selected.SendText("from IM")
	selected.WaitText(10*time.Second).AssertContains(t, "Harness reply: from IM")
	var events struct {
		Events []struct {
			Content string `json:"content"`
			Source  string `json:"source"`
		} `json:"events"`
	}
	call("GET", "/threads/"+thread.ID+"/events", nil, &events)
	found := false
	for _, event := range events.Events {
		if event.Content == "from IM" && event.Source == "im" {
			found = true
		}
	}
	require.True(t, found, "IM input should be recorded on the bound session")

	// Approval cards travel to the selected bot, and a button response goes
	// through the real bot authorization gate back into the running session.
	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "request approval"}, nil)
	prompt := selected.WaitText(10 * time.Second)
	prompt.AssertContains(t, "echo harness")
	button, ok := prompt.ButtonByLabel("Allow")
	require.True(t, ok, "approval must be actionable from IM")
	button.Click()
	selected.WaitText(10*time.Second).AssertContains(t, "Harness reply: request approval approved")
	selected.WaitRestate(prompt.MessageID, 10*time.Second)

	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "request denial"}, nil)
	denied := selected.WaitText(10 * time.Second)
	denyButton, ok := denied.ButtonByLabel("Deny")
	require.True(t, ok)
	denyButton.Click()
	selected.WaitText(10*time.Second).AssertContains(t, "Harness reply: request denial rejected")
	selected.WaitRestate(denied.MessageID, 10*time.Second)

	// A text answer to AskUserQuestion is consumed by the pending request,
	// not enqueued as another user turn.
	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "ask question"}, nil)
	selected.WaitText(10*time.Second).AssertContains(t, "Pick a color")
	selected.SendText("2")
	selected.WaitText(10*time.Second).AssertContains(t, "Harness reply: answered Blue")

	// Rebinding the same chat to another bot must move outbound delivery even
	// though the first bot was the last one to receive an inbound message.
	call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": other.ChatID, "bot_uuid": otherID, "thread_id": thread.ID}, nil)
	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "after rebind"}, nil)
	other.WaitText(10*time.Second).AssertContains(t, "Harness reply: after rebind")
	selected.ExpectNoEvent(200*time.Millisecond, tingly.EventSend)
	call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": selected.ChatID, "bot_uuid": chosenID, "thread_id": thread.ID}, nil)

	// Blocking the chat stops both directions without silently reassigning it.
	_, err := stores.RemoteChats().GetOrCreateChat(selected.ChatID, "tingly")
	require.NoError(t, err)
	require.NoError(t, stores.RemoteChats().SetChatDisabled(selected.ChatID, true))
	require.True(t, stores.RemoteChats().IsChatDisabled(selected.ChatID))
	call("POST", "/threads/"+thread.ID+"/messages", map[string]any{"content": "blocked output"}, nil)
	selected.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	selected.SendText("blocked input")
	selected.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	call("GET", "/threads/"+thread.ID+"/events", nil, &events)
	for _, event := range events.Events {
		require.NotEqual(t, "blocked input", event.Content)
	}

}

func TestTinglyNewChatAndSessionCommands(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("new-user").OpenDM(h.chosenID)
	// With no prior association, the bot adopts its configured folder and the
	// first message starts a session. Even an immediate reply must reach IM.
	chat.SendText("first message")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: first message")
	var list struct {
		Bindings []struct {
			ThreadID string `json:"thread_id"`
			BotUUID  string `json:"bot_uuid"`
		} `json:"bindings"`
	}
	h.call("GET", "/projects/channel-bindings", nil, &list)
	require.Len(t, list.Bindings, 1)
	first := list.Bindings[0].ThreadID
	require.NotEmpty(t, first)
	require.Equal(t, h.chosenID, list.Bindings[0].BotUUID)
	h.bridge.mu.Lock()
	h.bridge.bots = map[string]string{}
	h.bridge.mu.Unlock()
	h.call("POST", "/threads/"+first+"/messages", map[string]any{"content": "after restart"}, nil)
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: after restart")
	chat.SendText("/new")
	chat.WaitText(10*time.Second).AssertContains(t, "next message")
	chat.SendText("second session")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: second session")
	h.call("GET", "/projects/channel-bindings", nil, &list)
	require.Len(t, list.Bindings, 1)
	require.NotEqual(t, first, list.Bindings[0].ThreadID)
	require.Equal(t, h.chosenID, list.Bindings[0].BotUUID)
	chat.SendText("/verbose on")
	chat.WaitText(10*time.Second).AssertContains(t, "Verbose is on")
}

func TestTinglyBindFolderBeforeFirstSession(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("folder-binding-user").OpenDM(h.chosenID)
	var folder struct {
		ID string `json:"id"`
	}
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, &folder)
	h.call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": chat.ChatID, "bot_uuid": h.chosenID, "project_id": folder.ID}, nil)
	var list struct {
		Bindings []struct {
			ProjectID string `json:"project_id"`
			ThreadID  string `json:"thread_id"`
			BotUUID   string `json:"bot_uuid"`
		} `json:"bindings"`
	}
	h.call("GET", "/projects/channel-bindings?project_id="+folder.ID, nil, &list)
	require.Len(t, list.Bindings, 1)
	require.Empty(t, list.Bindings[0].ThreadID)
	chat.SendText("from a folder binding")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: from a folder binding")
	h.call("GET", "/projects/channel-bindings?project_id="+folder.ID, nil, &list)
	require.Len(t, list.Bindings, 1)
	require.NotEmpty(t, list.Bindings[0].ThreadID)
	require.Equal(t, h.chosenID, list.Bindings[0].BotUUID)
}

func TestTinglyCoordinatorIMRoundTrip(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("coordinator-user").OpenDM(h.chosenID)
	var project struct {
		ID           string `json:"id"`
		MainThreadID string `json:"main_thread_id"`
	}
	h.call("POST", "/projects", map[string]any{"name": "Harness project"}, &project)
	h.call("PUT", "/projects/channel-bindings", map[string]any{"platform": "tingly", "chat_id": chat.ChatID, "bot_uuid": h.chosenID, "project_id": project.ID}, nil)
	h.call("POST", "/threads/"+project.MainThreadID+"/messages", map[string]any{"content": "coordinator web"}, nil)
	chat.WaitText(10*time.Second).AssertContains(t, "You said: coordinator web")
	chat.SendText("coordinator IM")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: coordinator IM")
}

func TestTinglyProjectsFeatureGate(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("feature-user").OpenDM(h.chosenID)
	h.enabled.Store(false)
	chat.SendText("hello while disabled")
	chat.WaitText(10*time.Second).AssertContains(t, "Turn on Projects")
	h.enabled.Store(true)
	chat.SendText("hello when enabled")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: hello when enabled")
}

func TestTinglyBlockedAccessStopsSessionDelivery(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("blocked-user").OpenDM(h.chosenID)
	chat.SendText("establish session")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: establish session")
	targets, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.NoError(t, h.stores.BotAccess().SetDirectChatBlocked(context.Background(), h.chosenID, targets[0].ID, true))
	var list struct {
		Bindings []struct {
			ThreadID string `json:"thread_id"`
		} `json:"bindings"`
	}
	h.call("GET", "/projects/channel-bindings", nil, &list)
	require.Len(t, list.Bindings, 1)
	id := list.Bindings[0].ThreadID
	h.call("POST", "/threads/"+id+"/messages", map[string]any{"content": "must not be delivered"}, nil)
	require.Eventually(t, func() bool {
		var events struct {
			Events []struct {
				Content string `json:"content"`
			} `json:"events"`
		}
		h.call("GET", "/threads/"+id+"/events", nil, &events)
		for _, e := range events.Events {
			if strings.Contains(e.Content, "Harness reply: must not be delivered") {
				return true
			}
		}
		return false
	}, 10*time.Second, 25*time.Millisecond)
	chat.ExpectNoEvent(500*time.Millisecond, tingly.EventSend)
}

func TestTinglyRegisteredDirectories(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("directory-user").OpenDM(h.chosenID)
	chat.SendText("start")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: start")
	first, second := t.TempDir(), t.TempDir()
	var f1, f2 struct {
		ID string `json:"id"`
	}
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": first}, &f1)
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": second}, &f2)
	h.call("PATCH", "/projects/"+f2.ID, map[string]any{"archived": true}, nil)
	// A regular paired chat has start permission but not privileged filesystem
	// access. /dir must not reveal the registered paths until it is granted.
	chat.SendText("/dir")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	chat.SendText("/project")
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	chat.SendCallback("old-message", "project:switch:"+first)
	chat.ExpectNoEvent(300*time.Millisecond, tingly.EventSend)
	current, err := h.bridge.CurrentFolder(context.Background(), "tingly", chat.ChatID)
	require.NoError(t, err)
	require.Equal(t, h.workdir, current, "unprivileged callbacks cannot switch folders")
	targets, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, targets[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	chat.SendText("/dir")
	result := chat.WaitText(10 * time.Second)
	result.AssertContains(t, "Registered folders:").AssertContains(t, first).AssertContains(t, h.workdir)
	require.NotContains(t, result.Text, second, "archived folders are excluded")
	number := ""
	for _, line := range strings.Split(result.Text, "\n") {
		if strings.Contains(line, first) {
			words := strings.Fields(line)
			if len(words) > 0 {
				number = strings.TrimSuffix(words[0], ".")
			}
		}
	}
	require.NotEmpty(t, number)
	// A newly registered folder must not change an already displayed number.
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": t.TempDir()}, nil)
	chat.SendText("/dir " + number)
	chat.WaitText(10*time.Second).AssertContains(t, first)
	chat.SendText("new directory message")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: new directory message")
	// A button on an old list must not switch to a folder after it is archived.
	h.call("PATCH", "/projects/"+f1.ID, map[string]any{"archived": true}, nil)
	chat.SendCallback("old-message", "project:switch:"+first)
	chat.WaitText(10*time.Second).AssertContains(t, "no longer available")
}

func TestTinglyProjectsAndFoldersAreDistinct(t *testing.T) {
	h := newTinglyProjectsHarness(t)
	chat := h.env.NewUser("picker-user").OpenDM(h.chosenID)
	chat.SendText("start")
	chat.WaitText(10*time.Second).AssertContains(t, "Harness reply: start")
	targets, err := h.stores.BotAccess().ListDirectChats(context.Background(), h.chosenID)
	require.NoError(t, err)
	require.Len(t, targets, 1)
	require.NoError(t, h.stores.BotAccess().SetDirectChatPermission(context.Background(), h.chosenID, targets[0].ID, access.CapabilityRemoteControl, access.ActionRemoteControlPrivileged, access.EffectAllow))
	var project, archived struct {
		ID string `json:"id"`
	}
	h.call("POST", "/projects", map[string]any{"name": "Project Alpha"}, &project)
	h.call("POST", "/projects", map[string]any{"name": "Archived Project"}, &archived)
	h.call("PATCH", "/projects/"+archived.ID, map[string]any{"archived": true}, nil)
	otherFolder := t.TempDir()
	h.call("POST", "/projects", map[string]any{"type": "folder", "path": otherFolder}, nil)
	chat.SendText("/project")
	listed := chat.WaitText(10 * time.Second)
	listed.AssertContains(t, "Project Alpha")
	require.NotContains(t, listed.Text, "Archived Project")
	require.NotContains(t, listed.Text, otherFolder, "folders are not projects")
	number := ""
	for _, line := range strings.Split(listed.Text, "\n") {
		if strings.Contains(line, "Project Alpha") {
			parts := strings.Fields(line)
			if len(parts) > 0 {
				number = strings.TrimSuffix(parts[0], ".")
			}
		}
	}
	require.NotEmpty(t, number)
	// New projects added after the list must not change which ID that number means.
	h.call("POST", "/projects", map[string]any{"name": "Project Added Later"}, nil)
	chat.SendText("/project " + number)
	chat.WaitText(10*time.Second).AssertContains(t, "coordinator of Project Alpha")
	chat.SendText("hello coordinator")
	chat.WaitText(10*time.Second).AssertContains(t, "You said: hello coordinator")
	chat.SendText("/dir")
	dirs := chat.WaitText(10 * time.Second)
	dirs.AssertContains(t, otherFolder)
	require.NotContains(t, dirs.Text, "Project Alpha", "projects are not folders")
	require.Contains(t, dirs.Text, "This chat has not picked a folder.", "a project is not a folder binding")
	chat.SendText("/project")
	chat.WaitText(10*time.Second).AssertContains(t, "Project Alpha ✓")
}
