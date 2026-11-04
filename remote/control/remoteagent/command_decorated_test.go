package remoteagent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The same command arrives differently by platform and place: Telegram groups
// add "@botname", Slack and Discord a "<@id>" mention, Feishu an "@_user_1"
// placeholder, WeCom a plain "@name", DingTalk a leading space, and Chinese
// keyboards a full-width slash. All of them are the command.
func Test_DecoratedCommandsStillRun(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	type run struct{ text, want string }
	for _, tc := range []run{
		{"/status@tingly_bot", "status:"},
		{"/Status@Tingly_Bot", "status:"},
		{" /status", "status:"},
		{" /status", "status:"},
		{"@tingly_bot /status", "status:"},
		{"<@U123ABC> /status", "status:"},
		{"<@!123456> /status", "status:"},
		{"@_user_1 /status", "status:"},
		{"／status", "status:"},
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

func Test_DecoratedRegistryCommandsStillRun(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	for _, text := range []string{"/help@tingly_bot", " /help", "@tingly_bot /help", "<@U1> /help", "@_user_1 /help", "／help", "/start@tingly_bot"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "Your ID:")
	}
	for _, text := range []string{"/dir@tingly_bot", " /dir", "@tingly_bot /dir", "<@U1> /dir", "@_user_1 /dir", "／dir", "/DIR"} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, "folder")
	}

	dir := t.TempDir()
	for _, text := range []string{"/cd@tingly_bot " + dir, "@tingly_bot /cd " + dir, "／cd " + dir} {
		chat.Drain()
		chat.SendText(text)
		chat.WaitText(3*time.Second).AssertContains(t, dir)
	}
	assert.Empty(t, router.inbound(), "commands are not sent as messages")
}

// A mention in front of a plain message is still a message, an unknown
// command is named without the bot's suffix, and the old agent words are not
// commands even behind a mention.
func Test_DecoratedNonCommandsAreMessages(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	chat.SendText("@tingly_bot please fix the bug")
	require.Eventually(t, func() bool { return len(router.inbound()) == 1 }, 3*time.Second, 10*time.Millisecond)
	assert.Equal(t, "@tingly_bot please fix the bug", router.inbound()[0].Text)

	chat.Drain()
	chat.SendText("/nosuch@tingly_bot 1")
	chat.WaitText(3*time.Second).AssertContains(t, "Unknown command /nosuch.")
	assert.Empty(t, router.commands())
}
