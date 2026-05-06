package bot

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/remote_control/bot/feature"
	"github.com/tingly-dev/tingly-box/remote/audit"
	"github.com/tingly-dev/tingly-box/remote/session"
)

// TestHarness wires the production BotHandler against a test imbot.Manager
// (typically backed by the tingly platform). It owns the support
// infrastructure — chat store, session manager, agentboot, pairing — and
// exposes them so tests can drive state directly.
//
// Construction:
//
//	env := testenv.NewTestEnv(t)
//	uuid := env.BotUUID() // creates a tingly bot in env.Manager()
//	harness := bot.BootForTest(t, env.Manager(), bot.BotSetting{
//	    UUID:     uuid,
//	    Platform: "tingly",
//	    Enabled:  true,
//	})
//	require.NoError(t, env.Manager().Start(env.Context()))
//
// Tests then drive the bot through the testenv chat helpers.
type TestHarness struct {
	Setting    BotSetting
	Handler    *BotHandler
	ChatStore  ChatStoreInterface
	SessionMgr *session.Manager
	AgentBoot  *agentboot.AgentBoot
	Pairing    *PairingManager
	Audit      *audit.Logger
	DataDir    string
	Manager    *imbot.Manager

	cleanup func()
}

// TestBootOptions tweaks BootForTest defaults. All fields are optional.
type TestBootOptions struct {
	// DataDir overrides the chat-store directory (default: t.TempDir()).
	DataDir string

	// FixtureScript, when non-nil, registers a Claude agent backed by a
	// fixture.Factory(script). The fixture replaces the legacy mockagent —
	// tests now drive the real claude.Driver + claude.Transport + Runner
	// pipeline against scripted wire-format output.
	//
	// When nil (default), no Claude agent is registered and tests that
	// depend on agent execution must register their own.
	FixtureScript fixture.Script
}

// BootForTest spins up a production BotHandler against the given
// imbot.Manager. It assumes the Manager already has a bot registered for
// setting.UUID (the tingly testenv arranges this via AddTinglyBotWithUUID
// when env.BotUUID() is called).
//
// The harness registers the BotHandler.HandleMessage callback on the
// Manager. Callers must Start the Manager themselves — keeping that step
// in the test makes it explicit when inbound messages start flowing.
func BootForTest(t *testing.T, manager *imbot.Manager, setting BotSetting, opts ...TestBootOptions) *TestHarness {
	t.Helper()

	var opt TestBootOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.DataDir == "" {
		opt.DataDir = t.TempDir()
	}

	chatStorePath := filepath.Join(opt.DataDir, "chats.json")
	chatStore, err := NewChatStoreJSON(chatStorePath)
	if err != nil {
		t.Fatalf("BootForTest: chat store: %v", err)
	}

	sessionMgr := session.NewManager(session.Config{
		Timeout:          10 * time.Minute,
		MessageRetention: time.Hour,
	}, nil)

	agentCfg := agentboot.Config{
		ClaudeProjectsDir: filepath.Join(opt.DataDir, "claude-projects"),
	}
	ab, err := agentboot.New(agentCfg)
	if err != nil {
		// ClaudeProjectsDir failed; fall back to a config without it.
		ab, err = agentboot.New(agentboot.Config{})
		if err != nil {
			t.Fatalf("BootForTest: agentboot: %v", err)
		}
	}
	if opt.FixtureScript != nil {
		fixtureAgent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(opt.FixtureScript))
		ab.RegisterAgent(agentboot.AgentTypeClaude, fixtureAgent)
		_ = ab.SetDefaultAgent(agentboot.AgentTypeClaude)
	}

	auditLog := audit.NewLogger(audit.Config{Console: false, MaxEntries: 1000})
	pairing := NewPairingManager(auditLog)
	dirBrowser := feature.NewDirectoryBrowser()

	ctx := t.Context()
	handler := NewBotHandler(
		ctx,
		setting,
		chatStore,
		sessionMgr,
		ab,
		dirBrowser,
		manager,
		nil, // tbClient — SmartGuide path not exercised by tests; falls back to mock/claude as configured
		pairing,
		auditLog,
		nil, // store — not needed in test harness
	)

	manager.OnMessage(handler.HandleMessage)

	h := &TestHarness{
		Setting:    setting,
		Handler:    handler,
		ChatStore:  chatStore,
		SessionMgr: sessionMgr,
		AgentBoot:  ab,
		Pairing:    pairing,
		Audit:      auditLog,
		DataDir:    opt.DataDir,
		Manager:    manager,
		cleanup: func() {
			_ = chatStore.Close()
		},
	}
	t.Cleanup(h.cleanup)
	return h
}

// MintPairingCode mints a fresh pairing code for the harness's bot. Tests
// that exercise the pairing-required path use this to obtain the code the
// user must send via /bind.
func (h *TestHarness) MintPairingCode() (code string, expiresAt time.Time) {
	return h.Pairing.Mint(h.Setting.UUID)
}

// MarkChatPaired records a pairing for the harness's bot via the same
// production API path that VerifyAndPair uses. Tests focused on
// post-pairing behavior can skip the /bind handshake without bypassing
// the real persistence path — exercising any future bug in SetPaired.
func (h *TestHarness) MarkChatPaired(chatID, senderID string) {
	if err := h.ChatStore.SetPaired(chatID, h.Setting.Platform, h.Setting.UUID, senderID); err != nil {
		panic(err)
	}
}

// WhitelistGroup adds a group chat to the bot's whitelist (required for
// the bot to respond to group messages).
func (h *TestHarness) WhitelistGroup(chatID, ownerID string) {
	chat, err := h.ChatStore.GetOrCreateChat(chatID, h.Setting.Platform)
	if err != nil {
		panic(err)
	}
	chat.IsWhitelisted = true
	chat.WhitelistedBy = ownerID
	if err := h.ChatStore.UpsertChat(chat); err != nil {
		panic(err)
	}
}

// SetCurrentAgent updates the current-agent binding for a chat through the
// same production path the @cc/@tb handoff uses. Going through
// chatStore.SetCurrentAgent (rather than mutating Chat directly) keeps
// the harness honest: any regression in the persistence path — e.g. a
// silent no-op on a missing chat row — surfaces as a test failure.
func (h *TestHarness) SetCurrentAgent(chatID, agentType string) {
	if err := h.ChatStore.SetCurrentAgent(chatID, h.Setting.Platform, agentType); err != nil {
		panic(err)
	}
}

// EnsureContext provides a context that propagates either through
// t.Context() (Go 1.24+) or a fresh background context.
func EnsureContext(t testing.TB) context.Context {
	type ctxer interface{ Context() context.Context }
	if c, ok := t.(ctxer); ok {
		return c.Context()
	}
	return context.Background()
}
