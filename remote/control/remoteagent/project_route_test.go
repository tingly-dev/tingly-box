package remoteagent_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"
)

// bootHelperNotStarted is bootHelper without starting the manager, so a test
// can plug in a router before messages flow.
func bootHelperNotStarted(t *testing.T) (*testenv.TestEnv, *remoteagent.TestHarness, *testenv.User) {
	t.Helper()
	env := testenv.NewTestEnv(t)
	rp := false
	setting := bot.BotSetting{
		UUID: env.BotUUID(), Name: "tingly-test", Platform: "tingly", AuthType: "none",
		Auth: map[string]string{}, Enabled: true, RequirePairing: &rp,
	}
	harness := remoteagent.BootForTest(t, env.Manager(), setting)
	return env, harness, env.NewUser("alice")
}

// fakeRouter binds the chats in bound and records what it was handed.
type fakeRouter struct {
	mu      sync.Mutex
	bound   map[string]bool
	err     error
	got     []remoteagent.ProjectInbound
	cmds    []string
	answers []string
	// files are the bodies of the files handed over, read while they exist.
	files map[string]string
	// off switches Projects off; adopts makes ToFolder give a chat a place.
	off, adopts bool
	folders     []string
	profiles    []string
}

func (f *fakeRouter) Enabled() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.off
}

func (f *fakeRouter) ToFolder(_ context.Context, in remoteagent.ProjectInbound, folder string, ifNew bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if ifNew && f.bound[in.ChatID] {
		return nil
	}
	f.folders = append(f.folders, fmt.Sprintf("%s:%v", folder, ifNew))
	f.profiles = append(f.profiles, in.Profile)
	if f.adopts {
		f.bound[in.ChatID] = true
	}
	return nil
}

func (f *fakeRouter) movedTo() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.folders...)
}

func (f *fakeRouter) bind(chatID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.bound[chatID] = true
}

func (f *fakeRouter) setErr(err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.err = err
}

func (f *fakeRouter) RouteInbound(_ context.Context, in remoteagent.ProjectInbound) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.bound[in.ChatID] {
		return false, nil
	}
	f.got = append(f.got, in)
	for _, file := range in.Files {
		if f.files == nil {
			f.files = map[string]string{}
		}
		b, _ := os.ReadFile(file.Path)
		f.files[file.Name] = string(b)
	}
	return true, f.err
}

func (f *fakeRouter) RouteCommand(_ context.Context, name string, in remoteagent.ProjectInbound) (bool, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.bound[in.ChatID] {
		return false, "", nil
	}
	f.cmds = append(f.cmds, name+":"+in.Text)
	return true, "project did " + name, nil
}

func (f *fakeRouter) RouteAnswer(_ context.Context, in remoteagent.ProjectInbound, threadID, requestID, choice string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.answers = append(f.answers, in.ChatID+":"+threadID+":"+requestID+":"+choice)
	if choice == "late" {
		return "This was already answered.", nil
	}
	return "", f.err
}

func (f *fakeRouter) answered() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.answers...)
}

func (f *fakeRouter) commands() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.cmds...)
}

func (f *fakeRouter) inbound() []remoteagent.ProjectInbound {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]remoteagent.ProjectInbound(nil), f.got...)
}

// A chat bound to a project talks to the project: the text (even "@cc ...")
// goes to the router, and @cc / @tb never see it.
func Test_BoundChatGoesToItsProject(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	chat.SendText("@cc fix the login bug")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	in := router.inbound()[0]
	assert.Equal(t, "fix the login bug", in.Text, "the old agent word is dropped")
	assert.Equal(t, "tingly", in.Platform)
	assert.Equal(t, chat.ChatID, in.ChatID)
	assert.Equal(t, harness.Setting.UUID, in.BotUUID)

	// Delivered: the reply is the project's job, so the handler says nothing.
	chat.ExpectNoEvent(700*time.Millisecond, tingly.EventSend, tingly.EventEdit)

	// A project that cannot take the message says why, in the chat.
	router.setErr(errors.New("the project is paused; resume it first"))
	chat.SendText("hello?")
	chat.WaitText(3*time.Second).AssertContains(t, "Could not send this to the project").AssertContains(t, "paused")

	// Commands still work in a bound chat; /help lists the Projects ones.
	chat.Drain()
	chat.SendText("/help")
	help := chat.WaitText(3*time.Second).AssertContains(t, "Your ID:").AssertContains(t, "/session")
	assert.NotContains(t, help.Text, "@cc", "a bound chat no longer reaches @cc")
	assert.Len(t, router.inbound(), 2, "commands are not sent to the project")
}

// A bound chat's files go to its project with the message, downloaded first
// and removed once handed over.
func Test_BoundChatFilesGoToItsProject(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("png bytes"))
	}))
	defer srv.Close()
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)
	chat.SendMedia([]imbot.MediaAttachment{{Type: "image", URL: srv.URL + "/AgAC.png", MimeType: "image/png", Filename: "screen.png"}})
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	in := router.inbound()[0]
	require.Len(t, in.Files, 1)
	assert.Equal(t, "screen.png", in.Files[0].Name)
	router.mu.Lock()
	assert.Equal(t, "png bytes", router.files["screen.png"])
	router.mu.Unlock()
	_, err := os.Stat(in.Files[0].Path)
	assert.True(t, os.IsNotExist(err), "the download is removed once handed over")

	chat.Drain()
	chat.SendMedia([]imbot.MediaAttachment{{Type: "video", URL: srv.URL + "/v.mp4", MimeType: "video/mp4"}})
	chat.WaitText(3*time.Second).AssertContains(t, "File type not supported")
	assert.Len(t, router.inbound(), 1, "nothing is sent when a file cannot be taken")
}

// A chat with no place in Projects (no folder could be used) is told how to
// pick one, and nothing reaches Projects.
func Test_ChatWithoutAPlaceIsTold(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("hello there")
	chat.WaitText(3*time.Second).AssertContains(t, "/cd <path> picks one")
	chat.SendText("/stop")
	chat.WaitText(3*time.Second).AssertContains(t, "/cd <path> picks one")
	assert.Empty(t, router.inbound())
	assert.Empty(t, router.commands())
}

// Without a router chat control is off; access still works.
func Test_NoRouterNoChatControl(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	require.NoError(t, env.Manager().Start(env.Context()))
	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "Turn on Projects")
}

// The session commands go to Projects; the old commands are gone.
func Test_BoundChatCommandsGoToItsProject(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)
	for _, text := range []string{"/stop", "/clear", "/new", "/new fix the login bug", "/stop #2", "/session", "/session 3", "/status", "/verbose on", "/yolo"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "project did ")
	}
	assert.Equal(t, []string{"stop:", "clear:", "new:", "new:fix the login bug", "stop:#2", "session:", "session:3", "status:", "verbose:on", "yolo:"}, router.commands())
	assert.Empty(t, router.inbound(), "commands are not sent as messages")

	chat.Drain()
	chat.SendText("/help")
	help := chat.WaitText(3*time.Second).AssertContains(t, "Your ID:").AssertContains(t, "/session [N]")
	assert.NotContains(t, help.Text, "/threads")

	// The old commands are gone, and a bare "stop" is a message.
	for _, text := range []string{"/bash ls", "/resume", "/project", "/threads", "/focus 2", "/back", "/quiet", "/interrupt", "/mock"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "Unknown command")
	}
	chat.SendText("stop")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "stop", router.inbound()[0].Text)
	assert.Len(t, router.commands(), 10, "nothing else ran a command")
}

// A press on a Projects prompt's button goes to the project with the chat it
// came from; whatever the project says back is shown, and nothing else is.
func Test_ProjectPromptButtonsGoToTheProject(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendCallback("m1", remoteagent.ProjectAnswerPayload+":t1:r1:allow")
	require.Eventually(t, func() bool { return len(router.answered()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, chat.ChatID+":t1:r1:allow", router.answered()[0])
	chat.ExpectNoEvent(500*time.Millisecond, tingly.EventSend)

	chat.SendCallback("m1", remoteagent.ProjectAnswerPayload+":t1:r1:late")
	chat.WaitText(3*time.Second).AssertContains(t, "already answered")

	router.setErr(errors.New("this chat is not bound to that thread"))
	chat.SendCallback("m1", remoteagent.ProjectAnswerPayload+":t2:r2:deny")
	chat.WaitText(3*time.Second).AssertContains(t, "Could not answer").AssertContains(t, "not bound")
}

// With Projects on, a chat that never talked gets a folder first (the bot's
// default here) and then talks to it; /cd moves it.
func Test_EveryChatGetsAFolderInProjects(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}, adopts: true}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("/tb hello")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "hello", router.inbound()[0].Text)
	moved := router.movedTo()
	require.Len(t, moved, 1)
	assert.True(t, strings.HasSuffix(moved[0], ":true"), "the first folder only applies to a chat without one: %v", moved)

	dir := t.TempDir()
	chat.Drain()
	chat.SendText("/cd " + dir)
	chat.WaitText(3 * time.Second)
	require.Eventually(t, func() bool { return len(router.movedTo()) == 2 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, dir+":false", router.movedTo()[1])

	chat.SendText("@cc")
	for ev := chat.WaitText(3 * time.Second); !strings.Contains(ev.Text, "just send your message"); ev = chat.WaitText(3 * time.Second) {
	}
	assert.Len(t, router.inbound(), 1)
}

// A chat moves to a folder under its bot's Claude Code profile, so its first
// session there runs under it.
func Test_ChatsMoveUnderTheBotsProfile(t *testing.T) {
	for agent, want := range map[string]string{"": "", "claude_code": "", "claude_code:p1": "p1"} {
		env := testenv.NewTestEnv(t)
		rp := false
		harness := remoteagent.BootForTest(t, env.Manager(), bot.BotSetting{
			UUID: env.BotUUID(), Name: "tingly-test", Platform: "tingly", AuthType: "none",
			Auth: map[string]string{}, Enabled: true, RequirePairing: &rp, DefaultAgent: agent,
		})
		router := &fakeRouter{bound: map[string]bool{}, adopts: true}
		harness.Handler.SetProjectRouter(router)
		require.NoError(t, env.Manager().Start(env.Context()))

		env.NewUser("alice").OpenDM(harness.Setting.UUID).SendText("hello")
		require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
		router.mu.Lock()
		assert.Equal(t, []string{want}, router.profiles, "default agent %q", agent)
		router.mu.Unlock()
	}
}

// With Projects off, a chat hears that chatting is off; pairing still works.
func Test_ProjectsOffTurnsChatOff(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}, off: true}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	chat.SendText("@cc fix it")
	chat.WaitText(3*time.Second).AssertContains(t, "Turn on Projects")
	chat.Drain()
	chat.SendText("/bind 123456")
	ev := chat.WaitText(3 * time.Second)
	assert.NotContains(t, ev.Text, "Turn on Projects", "pairing is not chat control")
	assert.Empty(t, router.inbound())
	assert.Empty(t, router.movedTo())
}
