package remoteagent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/imbot"
)

// /help lists what the chat can run. A chat without privileged access cannot
// pick a free path, so /cd is not offered; /dir is, because picking from the
// registered folders needs only start.
func Test_HelpHidesWhatTheChatCannotRun(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	granted := false
	harness.Handler.SetPrivilegeChecker(func(_ imbot.Platform, _, _, _ string, _ bool) bool { return granted })
	require.NoError(t, env.Manager().Start(env.Context()))

	chat := alice.OpenDM(harness.Setting.UUID)
	router.bind(chat.ChatID)

	chat.SendText("/help")
	help := chat.WaitText(3*time.Second).AssertContains(t, "/dir").AssertContains(t, "/project").AssertContains(t, "/session")
	assert.NotContains(t, help.Text, "/cd", "a chat without privileged access cannot choose a free path")
	assert.NotContains(t, help.Text, "path", "no way to name a free path is offered either")

	granted = true
	chat.Drain()
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/dir [N|path]")
}

// In a group every folder and project command is privileged, so a chat
// without it is offered none of them.
func Test_GroupHelpHidesFolderAndProjectCommands(t *testing.T) {
	env, harness, alice := bootHelperNotStarted(t)
	router := &fakeRouter{bound: map[string]bool{}}
	harness.Handler.SetProjectRouter(router)
	granted := false
	var seenGroup []bool
	harness.Handler.SetPrivilegeChecker(func(_ imbot.Platform, _, _, _ string, group bool) bool {
		seenGroup = append(seenGroup, group)
		return granted
	})
	require.NoError(t, env.Manager().Start(env.Context()))

	group := env.NewGroup("eng", alice, env.NewUser("bob"))
	chat := group.Chat(harness.Setting.UUID)
	harness.WhitelistGroup(chat.ChatID, alice.ID)
	router.bind(chat.ChatID)

	chat.SendText("/help")
	help := chat.WaitText(3*time.Second).AssertContains(t, "/session")
	for _, hidden := range []string{"/cd", "/dir", "/project"} {
		assert.NotContains(t, help.Text, hidden)
	}
	require.Equal(t, []bool{true}, seenGroup, "the check knows it is a group")

	granted = true
	chat.Drain()
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "/cd <path>").AssertContains(t, "/dir").AssertContains(t, "/project")
}
