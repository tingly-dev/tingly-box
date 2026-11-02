package remoteagent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/remoteagent"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
)

// gateBoot wires a TestEnv + harness with no fixture agent — the gate drops
// traffic before any executor runs, so none is needed.
func gateBoot(t *testing.T) (*testenv.TestEnv, *remoteagent.TestHarness, *testenv.Chat) {
	t.Helper()

	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	rp := false
	setting := bot.BotSetting{
		UUID:           uuid,
		Name:           "tingly-test",
		Platform:       "tingly",
		AuthType:       "none",
		Auth:           map[string]string{},
		Enabled:        true,
		RequirePairing: &rp,
	}
	harness := remoteagent.BootForTest(t, env.Manager(), setting)
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(harness.Setting.UUID)
	return env, harness, chat
}

// TestDisabledChatDroppedSilently is the inbound half of the blocklist: a
// disabled chat's messages — including commands like /pair that could
// otherwise re-establish state — produce no outbound traffic at all. Silence
// is the point: a reply would hand the blocked party a probe signal.
func TestDisabledChatDroppedSilently(t *testing.T) {
	_, harness, chat := gateBoot(t)

	// Sanity: before disabling, the bot reacts to a command. Let the async
	// received-reaction land, then drain everything so it can't bleed into
	// the post-disable silence window.
	chat.SendText("/help")
	chat.WaitAnySend(3 * time.Second)
	time.Sleep(300 * time.Millisecond)
	chat.Drain()

	// Disable through the store, as the API path does. The row must exist —
	// SetChatDisabled deliberately no-ops on missing chats (the HTTP layer
	// 404s first via resolveReachableChat).
	_, err := harness.ChatStore.GetOrCreateChat(chat.ChatID, harness.Setting.Platform)
	require.NoError(t, err)
	require.NoError(t, harness.ChatStore.SetChatDisabled(chat.ChatID, true))

	chat.SendText("hello?")
	chat.SendText("/help")
	chat.SendText("/pair 123456") // must not be able to talk its way back in
	chat.ExpectIdle(700 * time.Millisecond)

	// Re-enable restores normal handling.
	require.NoError(t, harness.ChatStore.SetChatDisabled(chat.ChatID, false))
	chat.SendText("/help")
	chat.WaitAnySend(3 * time.Second)
}
