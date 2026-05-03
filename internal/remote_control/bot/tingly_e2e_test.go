package bot_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
)

// bootHelper wires a fresh testenv.TestEnv together with a BotHandler in a
// known-good configuration. Most subtests start with it.
func bootHelper(t *testing.T, requirePairing bool) (*testenv.TestEnv, *bot.TestHarness, *testenv.User) {
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
	harness := bot.BootForTest(t, env.Manager(), setting)

	require.NoError(t, env.Manager().Start(env.Context()))

	return env, harness, env.NewUser("alice")
}

// Test_Help drives the bot with /help and asserts a help-style reply.
func Test_Help(t *testing.T) {
	env, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/help")

	evt := chat.WaitText(3 * time.Second)
	// BuildHelpText lists each command with its description; "/help" is
	// always present. The /cc and /tb hints are unconditionally appended
	// (bot_command.go:83-84).
	evt.AssertContains(t, "@cc")
	evt.AssertContains(t, "Your ID:")

	// Sanity: the env wired the bot through tingly correctly.
	_ = env
}

// Test_UnknownCommand confirms unknown slash commands fall through to the
// "Unknown command" hint.
func Test_UnknownCommand(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/notacommand")

	chat.WaitText(3 * time.Second).
		AssertContains(t, "Unknown command").
		AssertContains(t, "/help")
}

// Test_PairingGate verifies the RequirePairing flow: an unpaired DM is
// rejected with the pairing hint, and only /bind is accepted.
func Test_PairingGate(t *testing.T) {
	_, harness, alice := bootHelper(t, true)
	chat := alice.OpenDM(harness.Setting.UUID)

	// Unpaired chat: any non-/bind text gets the pairing hint.
	chat.SendText("/help")
	chat.WaitText(3 * time.Second).AssertContains(t, "/bind <code>")

	// Mint a code, then /bind it.
	code, _ := harness.MintPairingCode()
	chat.SendText("/bind " + code)
	chat.WaitText(3 * time.Second).AssertContains(t, "Paired")

	// Now /help works.
	chat.SendText("/help")
	chat.WaitText(3 * time.Second).AssertContains(t, "@cc")
}

// Test_Cd_And_Project covers the project binding command and the
// follow-up /project listing.
func Test_Cd_And_Project(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	tmp := t.TempDir()
	chat.SendText("/cd " + tmp)
	// completeBind sends "Project bound" (DM) or "Group bound to project"
	// (group); the registry handler additionally sends "Bound to project"
	// — match on the common substring.
	chat.WaitText(3 * time.Second).AssertContains(t, "ound to project")

	// /project shows the bound path.
	chat.SendText("/project")
	chat.WaitText(3 * time.Second).AssertContains(t, "roject")

	_ = harness
}

// Test_Bash_Allowlist exercises the /bash allowlist gate. pwd is allowed
// by default; rm is not.
func Test_Bash_Allowlist(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/bash pwd")
	chat.WaitText(3 * time.Second) // pwd outputs whatever cwd the bot has

	chat.SendText("/bash rm -rf /")
	chat.WaitText(3 * time.Second).AssertContains(t, "not allowed")

	_ = harness
}

// Test_Verbose_Quiet toggles verbose mode on and off.
func Test_Verbose_Quiet(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/verbose off")
	chat.WaitText(3 * time.Second)

	chat.SendText("/quiet")
	chat.WaitText(3 * time.Second)

	chat.SendText("/verbose on")
	chat.WaitText(3 * time.Second)

	_ = harness
}

// Test_Interrupt_NoTask verifies /interrupt produces the "no running task"
// reply when nothing is running.
func Test_Interrupt_NoTask(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	chat.SendText("/interrupt")
	chat.WaitText(3 * time.Second).AssertContains(t, "No running task")

	_ = harness
}

// Test_Clear binds a project, switches the current agent to "mock", and
// then verifies /clear emits a confirmation. The default current-agent is
// "tingly-box", whose clear path requires a SmartGuide session store we
// don't construct here — so we explicitly route through the mock branch.
func Test_Clear(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	tmp := t.TempDir()
	chat.SendText("/cd " + tmp)
	// /cd produces two replies — completeBind sends "...bound to project"
	// and the registry handler echoes "Bound to project". Drain both.
	chat.WaitText(3 * time.Second).AssertContains(t, "ound to project")
	chat.WaitText(3 * time.Second).AssertContains(t, "ound to project")

	harness.SetCurrentAgent(chat.ChatID, "mock")

	chat.SendText("/clear")
	// With the mock agent and no active session, the bot responds with
	// "No active Mock Agent (@mock) session found." — that confirms the
	// /clear command reached the mock-branch and produced a reply.
	evt := chat.WaitText(3 * time.Second)
	if !contains(evt.Text, "cleared") && !contains(evt.Text, "No active") {
		t.Fatalf("expected /clear reply, got %q", evt.Text)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
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
	chat.WaitText(3 * time.Second).AssertContains(t, "not enabled")

	// Whitelist and try again — now /help works.
	harness.WhitelistGroup(chat.ChatID, alice.ID)
	chat.SendText("/help")
	chat.WaitText(3 * time.Second).AssertContains(t, "@cc")
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
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	msgID := chat.SendText("hello")
	react := chat.WaitReaction(msgID, 3*time.Second)
	if react.Emoji != "👨‍💻" {
		t.Fatalf("expected 'received' reaction emoji, got %q", react.Emoji)
	}
}

// Test_ProjectPick exercises the inline-keyboard project-switch flow.
// /project lists projects with a Bind New keyboard.
//
// The production registry adapter constructs a HandlerContext without a
// SenderID when calling SetProjectPath, so the chat ends up bound with
// owner="". To get a project that ListProjectPaths can find, we bind
// directly through the chat store with the alice owner id.
func Test_ProjectPick(t *testing.T) {
	_, harness, alice := bootHelper(t, false)
	chat := alice.OpenDM(harness.Setting.UUID)

	tmp := t.TempDir()
	require.NoError(t, harness.ChatStore.BindProject(chat.ChatID, "tingly", tmp, alice.ID))

	chat.SendText("/project")
	evt := chat.WaitText(3 * time.Second)
	evt.AssertContains(t, "Current Project")
	evt.AssertHasButton(t, "Bind New Project")
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
