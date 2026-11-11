package chat_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
)

// The parser's tables live in imbot/core and the full folder/permission flows
// in the Tingly harness; this checks the handler passes the chat type through:
// a group reads decorated commands, a direct chat only the plain form.
func Test_GroupReadsDecoratedCommands(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	chat := env.NewGroup("eng", alice, env.NewUser("bob")).Chat(harness.Setting.UUID)
	harness.WhitelistGroup(chat.ChatID, alice.ID)
	router.bind(chat.ChatID)

	for text, want := range map[string]string{
		"/status@tingly_bot":             "status:",
		"@tingly_bot /status":            "status:",
		"<@U123ABC>/status":              "status:",
		"@_user_1 /new    fix  the bug ": "new:fix  the bug",
		"／status":                        "status:",
	} {
		before := len(router.commands())
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "project did ")
		cmds := router.commands()
		require.Len(t, cmds, before+1, text)
		assert.Equal(t, want, cmds[before], text)
	}
	chat.Drain()
	chat.SendText("@tingly_bot/help")
	chat.WaitText(3*time.Second).AssertContains(t, "Your ID:")
	chat.SendText("@tingly_bot please fix the bug")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot please fix the bug", router.inbound()[0].Text, "a mention before plain text is a message")
	chat.Drain()
	chat.SendText("/nosuch@tingly_bot 1")
	chat.WaitText(3*time.Second).AssertContains(t, "Unknown command /nosuch.")
}

func TestHelpShortcutChatPaths(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	direct := alice.OpenDM(harness.Setting.UUID)
	group := env.NewGroup("eng", alice, env.NewUser("bob")).Chat(harness.Setting.UUID)
	harness.WhitelistGroup(group.ChatID, alice.ID)
	router.bind(direct.ChatID)
	router.bind(group.ChatID)

	for _, tc := range []struct {
		name  string
		chat  *testenv.Chat
		forms []string
	}{
		{"direct", direct, []string{"/h", " /H extra arguments ", "／h", "/start", "/"}},
		{"group", group, []string{"/h", "/h@tingly_bot args", "@tingly_bot /h", "<@U123>/h", "@_user_1 /h", "／h"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.chat.Drain()
			tc.chat.SendText("/help")
			want := tc.chat.WaitText(3 * time.Second).Text
			for _, text := range tc.forms {
				tc.chat.Drain()
				tc.chat.SendText(text)
				assert.Equal(t, want, tc.chat.WaitText(3*time.Second).Text, text)
			}
		})
	}
	direct.Drain()
	direct.SendText("/h@tingly_bot")
	direct.WaitText(3*time.Second).AssertContains(t, "Unknown command /h@tingly_bot")
	direct.SendText("@tingly_bot /h")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot /h", router.inbound()[0].Text)
}

func TestPickerShortcutsMatchCanonicalCommands(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)
	for _, pair := range [][2]string{{"/d", "/dir"}, {"/p", "/project"}, {"/d 2", "/dir 2"}, {"/p 2", "/project 2"}, {"/d /path/that/does/not/exist", "/dir /path/that/does/not/exist"}} {
		chat.Drain()
		chat.SendText(pair[1])
		want := chat.WaitText(3 * time.Second).Text
		chat.Drain()
		chat.SendText(pair[0])
		assert.Equal(t, want, chat.WaitText(3*time.Second).Text, pair[0])
	}
}

func TestProjectShortcutDecorations(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	direct := alice.OpenDM(harness.Setting.UUID)
	group := env.NewGroup("eng", alice, env.NewUser("bob")).Chat(harness.Setting.UUID)
	harness.WhitelistGroup(group.ChatID, alice.ID)
	router.bind(direct.ChatID)
	router.bind(group.ChatID)

	for _, tc := range []struct {
		chat      *testenv.Chat
		canonical string
		shortcut  string
	}{
		{direct, "/project 2", "／P 2"},
		{group, "/project 2", "/p@tingly_bot 2"},
		{group, "/project 2", "<@U123> /p 2"},
	} {
		tc.chat.Drain()
		tc.chat.SendText(tc.canonical)
		want := tc.chat.WaitText(3 * time.Second).Text
		tc.chat.Drain()
		tc.chat.SendText(tc.shortcut)
		assert.Equal(t, want, tc.chat.WaitText(3*time.Second).Text, tc.shortcut)
	}
	direct.Drain()
	direct.SendText("/p@tingly_bot 2")
	direct.WaitText(3*time.Second).AssertContains(t, "Unknown command /p@tingly_bot")
	direct.SendText("@tingly_bot /p 2")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot /p 2", router.inbound()[0].Text)
}

func TestSessionShortcutsRouteCanonicalNameAndArguments(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	direct := alice.OpenDM(harness.Setting.UUID)
	group := env.NewGroup("eng", alice, env.NewUser("bob")).Chat(harness.Setting.UUID)
	harness.WhitelistGroup(group.ChatID, alice.ID)
	router.bind(direct.ChatID)
	router.bind(group.ChatID)

	for _, pair := range [][2]string{{"s", "session"}, {"n", "new"}, {"cls", "clear"}, {"stat", "status"}, {"v", "verbose"}} {
		for _, tc := range []struct {
			chat *testenv.Chat
			text string
		}{
			{direct, "/" + pair[0] + "  one  two"},
			{group, "@tingly_bot /" + pair[0] + "  one  two"},
		} {
			before := len(router.commands())
			tc.chat.Drain()
			tc.chat.SendText(tc.text)
			tc.chat.WaitText(3 * time.Second)
			require.Len(t, router.commands(), before+1, tc.text)
			assert.Equal(t, pair[1]+":one  two", router.commands()[before], tc.text)
		}
	}
	for _, tc := range []struct {
		chat *testenv.Chat
		text string
		want string
	}{
		{direct, "/s 2", "session:2"},
		{direct, "／S 2", "session:2"},
		{group, "/s@tingly_bot 2", "session:2"},
		{group, "<@U123> /s 2", "session:2"},
	} {
		before := len(router.commands())
		tc.chat.Drain()
		tc.chat.SendText(tc.text)
		tc.chat.WaitText(3 * time.Second)
		require.Len(t, router.commands(), before+1, tc.text)
		assert.Equal(t, tc.want, router.commands()[before], tc.text)
	}
	direct.Drain()
	direct.SendText("/s@tingly_bot 2")
	direct.WaitText(3*time.Second).AssertContains(t, "Unknown command /s@tingly_bot")
	direct.SendText("@tingly_bot /s 2")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot /s 2", router.inbound()[0].Text)
	for _, text := range []string{"/sess", "/proj", "/sto", "/cl", "/sp", "/cr", "/unknown"} {
		direct.Drain()
		direct.SendText(text)
		direct.WaitText(3*time.Second).AssertContains(t, "Unknown command "+text)
	}
}

func Test_DirectChatReadsPlainCommandsOnly(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))
	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	for _, text := range []string{"  /status\n", "／status"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "project did ")
	}
	assert.Equal(t, []string{"status:", "status:"}, router.commands())
	for i, text := range []string{"@tingly_bot /status", "<@U123>/status"} {
		chat.SendText(text)
		require.Eventually(t, func() bool { return len(router.inbound()) == i+1 }, 3*time.Second, 10*time.Millisecond, text)
		assert.Equal(t, text, router.inbound()[i].Text, "a mention makes it ordinary text here")
	}
	chat.Drain()
	chat.SendText("/status@tingly_bot")
	chat.WaitText(3*time.Second).AssertContains(t, "Unknown command /status@tingly_bot")
	assert.Len(t, router.commands(), 2)
}
