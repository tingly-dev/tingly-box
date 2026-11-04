package remoteagent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
)

// bootGroup is a bound, whitelisted group chat of a bot with Projects on.
func bootGroup(t *testing.T) (*fakeRouter, *testenv.Chat) {
	t.Helper()
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	group := env.NewGroup("eng", alice, env.NewUser("bob"))
	chat := group.Chat(harness.Setting.UUID)
	harness.WhitelistGroup(chat.ChatID, alice.ID)
	router.bind(chat.ChatID)
	return router, chat
}

func bootDirect(t *testing.T) (*fakeRouter, *testenv.Chat) {
	t.Helper()
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)
	return router, chat
}

// In a group people @mention the bot, and each platform writes that
// differently: Telegram "/status@bot", Slack and Discord "<@id>", Feishu
// "@_user_1", WeCom and WhatsApp "@name", a full-width slash from Chinese
// keyboards, and any of it with or without a space. All of it is the command.
func Test_GroupDecoratedCommandsStillRun(t *testing.T) {
	router, chat := bootGroup(t)

	type run struct{ text, want string }
	for _, tc := range []run{
		{"/status", "status:"},
		{"/status@tingly_bot", "status:"},
		{"/Status@Tingly_Bot", "status:"},
		{" /status", "status:"},
		{"\u00a0/status", "status:"},
		{"\u200b/status", "status:"},
		{"@tingly_bot /status", "status:"},
		{"@tingly_bot/status", "status:"},
		{"@tingly_bot    /status   ", "status:"},
		{"<@U123ABC> /status", "status:"},
		{"<@U123ABC>/status", "status:"},
		{"<@!123456> /status", "status:"},
		{"@_user_1 /status", "status:"},
		{"@_user_1/status", "status:"},
		{"／status", "status:"},
		{"  /status\n", "status:"},
		{"/new    fix   the  bug  ", "new:fix   the  bug"},
		{"/new@tingly_bot fix the login bug", "new:fix the login bug"},
		{"@tingly_bot /stop #2", "stop:#2"},
		{"/session@tingly_bot 3", "session:3"},
		{"/verbose@tingly_bot on", "verbose:on"},
	} {
		before := len(router.commands())
		chat.Drain()
		chat.SendText(tc.text)
		chat.WaitText(3*time.Second).AssertContains(t, "project did ")
		cmds := router.commands()
		require.Len(t, cmds, before+1, tc.text)
		assert.Equal(t, tc.want, cmds[before], tc.text)
	}
	assert.Empty(t, router.inbound(), "commands are not sent as messages")
}

func Test_GroupDecoratedRegistryCommandsStillRun(t *testing.T) {
	router, chat := bootGroup(t)

	for _, text := range []string{"/help@tingly_bot", " /help", "@tingly_bot /help", "@tingly_bot/help", "<@U1> /help", "@_user_1 /help", "／help", "/start@tingly_bot"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "Your ID:")
	}
	for _, text := range []string{"@tingly_bot/dir", "<@U1>/dir", "  /dir   ", "/dir@tingly_bot", " /dir", "@tingly_bot /dir", "<@U1> /dir", "@_user_1 /dir", "／dir", "/DIR"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "folder")
	}

	dir := t.TempDir()
	for _, text := range []string{"/cd@tingly_bot " + dir, "@tingly_bot /cd " + dir, "@tingly_bot/cd   " + dir, "／cd " + dir} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, dir)
	}
	assert.Empty(t, router.inbound(), "commands are not sent as messages")
}

// In a group a mention in front of plain text is still a message, and an
// unknown command is named without the bot's suffix.
func Test_GroupNonCommandsAreMessages(t *testing.T) {
	router, chat := bootGroup(t)

	chat.SendText("@tingly_bot please fix the bug")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot please fix the bug", router.inbound()[0].Text)

	chat.Drain()
	chat.SendText("/nosuch@tingly_bot 1")
	chat.WaitText(3*time.Second).AssertContains(t, "Unknown command /nosuch.")
	assert.Empty(t, router.commands())
}

// A direct chat has one party, so it never needs an @: plain "/name args"
// runs (with spaces, a full-width slash, surrounding whitespace), and the
// mentioned forms are ordinary text.
func Test_DirectChatReadsPlainCommandsOnly(t *testing.T) {
	router, chat := bootDirect(t)

	for _, tc := range []struct{ text, want string }{
		{"/status", "status:"},
		{"  /status\n", "status:"},
		{"／status", "status:"},
		{"/new    fix   the  bug  ", "new:fix   the  bug"},
		{"/stop\t#2", "stop:#2"},
	} {
		before := len(router.commands())
		chat.Drain()
		chat.SendText(tc.text)
		chat.WaitText(3*time.Second).AssertContains(t, "project did ")
		cmds := router.commands()
		require.Len(t, cmds, before+1, tc.text)
		assert.Equal(t, tc.want, cmds[before], tc.text)
	}
	assert.Empty(t, router.inbound())

	mentioned := []string{"@tingly_bot /status", "@tingly_bot/status", "<@U123> /status", "<@U123>/status", "@_user_1 /status"}
	for i, text := range mentioned {
		chat.SendText(text)
		want := i + 1
		require.Eventually(t, func() bool { return len(router.inbound()) == want }, 3*time.Second, 10*time.Millisecond, text)
		assert.Equal(t, text, router.inbound()[i].Text, "a mention makes it ordinary text here")
	}
	assert.Len(t, router.commands(), 5, "none of them ran a command")

	chat.Drain()
	chat.SendText("/status@tingly_bot")
	chat.WaitText(3*time.Second).AssertContains(t, "Unknown command /status@tingly_bot")
	assert.Len(t, router.commands(), 5)
}
