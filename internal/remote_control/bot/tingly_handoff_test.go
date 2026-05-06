package bot_test

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly/testenv"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// freshAgentBoot is the same setup as agentBoot but does NOT pre-bind the
// chat to "claude". Tests for the @cc handoff path need the chat to start
// from the default current_agent so the handoff actually does work.
func freshAgentBoot(t *testing.T, script fixture.Script) (*testenv.TestEnv, *bot.TestHarness, *testenv.Chat) {
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
	harness := bot.BootForTest(t, env.Manager(), setting, bot.TestBootOptions{
		FixtureScript: script,
	})
	require.NoError(t, env.Manager().Start(env.Context()))

	alice := env.NewUser("alice")
	chat := alice.OpenDM(harness.Setting.UUID)
	return env, harness, chat
}

// Test_AtCcHandoff_FreshChat covers the @cc <task> path on a chat that has
// never been bound (/cd) or paired (/bind). This is the regression case
// for the silent-no-op SetCurrentAgent bug: the executor running for THIS
// turn doesn't depend on the persisted current-agent (targetAgent is
// passed directly), so this test alone wouldn't catch a re-introduction
// of that bug — but it locks down the basic "@cc <task> reaches Claude"
// contract that nothing tested before.
func Test_AtCcHandoff_FreshChat(t *testing.T) {
	_, harness, chat := freshAgentBoot(t, fixture.Script{
		fixture.AssistantText("claude saw the task"),
		fixture.Result(true),
	})

	chat.SendText("@cc do the thing")

	// First the handoff confirmation, then the executor preface, then the
	// fixture's assistant text and the completion card.
	chat.ExpectInOrderLoose(5*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Handoff complete", Name: "handoff-ack"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "CC: Processing", Name: "preface"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "claude saw the task", Name: "assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID),
		"@cc <task> on a fresh chat must run via Claude and complete normally")
}

// Test_AtCcHandoff_PersistsAcrossTurns is the direct regression test for
// the silent-no-op bug: send "@cc" alone (no trailing task) on a fresh
// chat, then a plain message on the next turn, and assert the second
// turn was routed to Claude. Pre-fix this failed because
// chatStore.SetCurrentAgent silently dropped the write on a missing chat
// row, so getCurrentAgent on turn 2 returned the default Smart Guide.
func Test_AtCcHandoff_PersistsAcrossTurns(t *testing.T) {
	_, harness, chat := freshAgentBoot(t, fixture.Script{
		fixture.AssistantText("turn-2 claude reply"),
		fixture.Result(true),
	})

	// Turn 1: just the handoff. No executor runs.
	chat.SendText("@cc")
	ack := chat.WaitText(3 * time.Second)
	require.True(t, strings.Contains(ack.Text, "Handoff complete"),
		"first turn should produce only the handoff confirmation, got %q", ack.Text)

	// Turn 2: plain text. If persistence works this routes to Claude.
	chat.SendText("hello")
	chat.ExpectInOrderLoose(5*time.Second,
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "CC: Processing", Name: "preface"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "turn-2 claude reply", Name: "assistant"},
		testenv.Matcher{Kind: tingly.EventSend, TextContains: "Task done", Name: "completion"},
	)

	require.Equal(t, session.StatusCompleted, lastClaudeSession(t, harness, chat.ChatID),
		"plain message after @cc must route to Claude, proving handoff persisted")
}

// Test_HandoffPath_RemovesMockEntry locks down the P0 cleanup: @mock
// (and /mock) used to be a routable handoff target but the executor was
// deleted in Phase 2, so any user typing @mock got "no executor found".
// The cleanup dropped the @mock entry from DetectHandoffCommand — so
// @mock is now treated as a regular message and falls to the default
// agent (Smart Guide). In tests Smart Guide can't initialize (no API
// key wired), which is exactly the signal we use: a Smart-Guide-shaped
// error proves @mock did NOT trigger any handoff or Claude executor.
func Test_HandoffPath_RemovesMockEntry(t *testing.T) {
	// Fixture is required by BootForTest's claude-agent registration even
	// though @mock should never reach it.
	_, _, chat := freshAgentBoot(t, fixture.Script{
		fixture.AssistantText("claude should NOT have run"),
		fixture.Result(true),
	})

	chat.SendText("@mock test")

	// Smart Guide reports unavailability; this is the positive signal.
	evt := chat.WaitText(3 * time.Second)
	require.True(t,
		strings.Contains(evt.Text, "Smart Guide") || strings.Contains(evt.Text, "BaseURL"),
		"@mock should fall through to default agent (Smart Guide), got %q", evt.Text)

	// Negative: the handoff confirmation and Claude preface must not appear.
	require.False(t, strings.Contains(evt.Text, "Handoff complete"))
	require.False(t, strings.Contains(evt.Text, "Mock Agent"))
	require.False(t, strings.Contains(evt.Text, "CC: Processing"))
}
