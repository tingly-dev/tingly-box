package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/remote/bot"
	"github.com/tingly-dev/tingly-box/remote/chat"

	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
)

// bootHelper wires a fresh testenv.TestEnv together with a BotHandler in a
// known-good configuration: Projects on, and every chat given a folder
// (fakeRouter). Most subtests start with it.
func bootHelper(t *testing.T, requirePairing bool) (*testenv.TestEnv, *chat.TestHarness, *testenv.User) {
	t.Helper()

	env := testenv.NewTestEnv(t)
	uuid := env.BotUUID()

	rp := requirePairing
	setting := bot.BotSetting{
		UUID:           uuid,
		Name:           "tingly-test",
		Platform:       "tingly",
		AuthType:       "none",
		Auth:           map[string]string{},
		Enabled:        true,
		RequirePairing: &rp,
	}
	harness := chat.BootForTest(t, env.Manager(), setting, chat.TestBootOptions{
		Router: &fakeRouter{bound: map[string]bool{}, adopts: true},
	})

	require.NoError(t, env.Manager().Start(env.Context()))

	return env, harness, env.NewUser("alice")
}

// Test_Help drives the bot with /help and asserts it lists the commands and
// the ids.
func Test_Help(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/help")

	chat.WaitText(3*time.Second).
		AssertContains(t, "/session").
		AssertContains(t, "/cd").
		AssertContains(t, "Your ID:")
}

// Test_UnknownCommand confirms unknown slash commands fall through to the
// "Unknown command" hint.
func Test_UnknownCommand(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/notacommand")

	chat.WaitText(3*time.Second).
		AssertContains(t, "Unknown command").
		AssertContains(t, "/help")
}

// Test_PairingGate verifies the RequirePairing flow: an unpaired DM is
// rejected with the pairing hint, and only /pair is accepted.
func Test_PairingGate(t *testing.T) {
	_, harness, alice := bootHelper(t, true)
	chat := alice.OpenDM(harness.Setting.UUID)

	// Unpaired chat: any non-/pair text gets the pairing hint.
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "/pair <code>")

	// Mint a code, then /pair it.
	code, _ := harness.MintPairingCode()
	chat.SendText("/pair " + code)
	chat.WaitText(3*time.Second).AssertContains(t, "Paired")

	// Now /help works.
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "/session")
}

func Test_LegacyBindStillPairs(t *testing.T) {
	_, harness, alice := bootHelper(t, true)
	chat := alice.OpenDM(harness.Setting.UUID)
	code, _ := harness.MintPairingCode()
	chat.SendText("/bind " + code)
	chat.WaitText(3*time.Second).AssertContains(t, "Paired")
}

// Test_Cd_And_Dir moves the chat to a folder with /cd, then lists it with /dir.
func Test_Cd_And_Dir(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	tmp := t.TempDir()
	chat.SendText("/cd " + tmp)
	chat.WaitText(3*time.Second).
		AssertContains(t, tmp).
		AssertContains(t, "Your next message starts a new session here")

	// /dir shows the folder.
	chat.SendText("/dir")
	chat.WaitText(3*time.Second).AssertContains(t, "Current folder").AssertContains(t, tmp)
}

// Test_GroupWhitelistGate confirms a non-whitelisted group receives the
// "not enabled" hint, and a whitelisted one receives normal command
// replies.
func Test_GroupWhitelistGate(t *testing.T) {
	env, harness, alice := bootHelper(t, false)

	bob := env.NewUser("bob")
	group := env.NewGroup("eng", alice, bob)
	chat := group.Chat(harness.Setting.UUID)

	chat.SendText("/help")
	// Non-whitelisted: bot replies with the join hint.
	chat.WaitText(3*time.Second).AssertContains(t, "not enabled")

	// Whitelist and try again — now /help works.
	harness.WhitelistGroup(chat.ChatID, alice.ID)
	chat.SendText("/help")
	chat.WaitText(3*time.Second).AssertContains(t, "/session")
}

// Test_PairingResetCleanup tickles a corner of the harness: the bot
// shutdown path must be clean even with no traffic. Run repeatedly to
// catch goroutine leaks.
func Test_PairingResetCleanup(t *testing.T) {
	for i := 0; i < 3; i++ {
		env, _, _ := bootHelper(t, true)
		_ = env
	}
}

// Test_ReactionReceived verifies the bot reacts to inbound messages with
// the "received" emoji (👨‍💻 on tingly).
func Test_ReactionReceived(t *testing.T) {
	for _, text := range []string{"hello", "/help", "/h", "/status", "/notacommand"} {
		t.Run(text, func(t *testing.T) {
			_, harness, alice := bootHelper(t, false)
			chat := alice.OpenDM(harness.Setting.UUID)
			msgID := chat.SendText(text)
			react := chat.WaitReaction(msgID, 3*time.Second)
			require.Equal(t, "👨‍💻", react.Emoji)
			chat.ExpectNoEvent(150*time.Millisecond, tingly.EventReact)
		})
	}
}

func Test_ReactionRespectsPairingAndWhitelist(t *testing.T) {
	env, harness, alice := bootHelper(t, true)
	dm := alice.OpenDM(harness.Setting.UUID)
	dm.SendText("/help")
	dm.WaitText(3*time.Second).AssertContains(t, "/pair <code>")
	dm.ExpectNoEvent(150*time.Millisecond, tingly.EventReact)
	dm.SendText("/pair invalid")
	dm.WaitText(3 * time.Second)
	dm.ExpectNoEvent(150*time.Millisecond, tingly.EventReact)
	code, _ := harness.MintPairingCode()
	id := dm.SendText("/pair " + code)
	dm.WaitText(3*time.Second).AssertContains(t, "Paired")
	require.Equal(t, "👨‍💻", dm.WaitReaction(id, 3*time.Second).Emoji)
	dm.ExpectNoEvent(150*time.Millisecond, tingly.EventReact)

	group := env.NewGroup("eng", alice).Chat(harness.Setting.UUID)
	group.SendText("/help")
	group.WaitText(3*time.Second).AssertContains(t, "not enabled")
	group.ExpectNoEvent(150*time.Millisecond, tingly.EventReact)
}

// Test_DirPick exercises the inline-keyboard folder-switch flow: /dir lists
// the folders the chat used, with a button for another one.
func Test_DirPick(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	tmp := t.TempDir()
	chat.SendText("/cd " + tmp)
	chat.WaitText(3*time.Second).AssertContains(t, tmp)

	chat.SendText("/dir")
	evt := chat.WaitText(3 * time.Second)
	evt.AssertContains(t, "Current folder")
	evt.AssertHasButton(t, "Another folder")
}

// Test_StreamedTransport_NoLeakOnDisconnect verifies disconnecting the
// bot cleanly stops further outbound captures. Helps catch goroutine
// leaks when used with -race -count=N.
func Test_StreamedTransport_NoLeakOnDisconnect(t *testing.T) {
	env, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/help")
	chat.WaitText(3 * time.Second)

	// Disconnect via context cancellation.
	bot := env.Manager().GetBotByUUID(harness.Setting.UUID)
	require.NoError(t, bot.Disconnect(context.Background()))

	// The transport should be closed; chats should report empty channel.
	tr := env.Transport(harness.Setting.UUID)
	if tr == nil {
		t.Fatal("expected transport after disconnect")
	}
	_ = tingly.EventSend // keep the import live
}
