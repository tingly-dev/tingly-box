// testharness.go is TEST INFRASTRUCTURE, not runtime code: it stands up a real
// db-backed environment (chat store, session manager, agentboot) so the
// remoteagent tests can drive the production BotHandler end-to-end. The
// internal/data/db import is intentional here — building a real store manager
// is host-side wiring that does not belong on the runtime path, and this file
// is only linked into test binaries. The runtime packages (bot, remoteagent
// non-test code) remain free of db.* imports.

package remoteagent

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/agentboot/claude/fixture"
	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/db"
	bot2 "github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/feature"
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
	Setting      bot2.BotSetting
	Handler      *BotHandler
	ChatStore    bot2.ChatStoreInterface
	SessionMgr   *session.Manager
	AgentService *agentboot.AgentService
	Pairing      *bot2.PairingManager
	DataDir      string
	Manager      *imbot.Manager

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
func BootForTest(t *testing.T, manager *imbot.Manager, setting bot2.BotSetting, opts ...TestBootOptions) *TestHarness {
	t.Helper()

	var opt TestBootOptions
	if len(opts) > 0 {
		opt = opts[0]
	}
	if opt.DataDir == "" {
		opt.DataDir = t.TempDir()
	}

	sm, err := db.NewStoreManager(opt.DataDir)
	if err != nil {
		t.Fatalf("BootForTest: store manager: %v", err)
	}
	t.Cleanup(func() { _ = sm.Close() })
	chatStore := sm.RemoteChats()

	sessionMgr := session.NewManager(session.Config{
		Timeout:          10 * time.Minute,
		MessageRetention: time.Hour,
	}, nil)

	agentCfg := agentboot.Config{}
	agentService, err := claude.NewService(
		agentCfg,
		claude.WithProjectsDir(filepath.Join(opt.DataDir, "claude-projects")),
	)
	if err != nil {
		// Custom history initialization failed; fall back to Claude's default.
		agentService, err = claude.NewService(agentboot.Config{})
		if err != nil {
			t.Fatalf("BootForTest: agent service: %v", err)
		}
	}
	if opt.FixtureScript != nil {
		fixtureAgent := claude.NewAgentWithFactory(claude.Config{}, fixture.Factory(opt.FixtureScript))
		agentService.RegisterAgent(agentboot.AgentTypeClaude, fixtureAgent)
		if err := agentService.SetDefaultAgent(agentboot.AgentTypeClaude); err != nil {
			t.Fatalf("BootForTest: set default agent: %v", err)
		}
	}

	pairing := bot2.NewPairingManager(bot2.NewLogAuditor())
	dirBrowser := feature.NewDirectoryBrowser()

	ctx := t.Context()
	handler := NewBotHandler(
		ctx,
		setting,
		chatStore,
		sessionMgr,
		agentService,
		dirBrowser,
		manager,
		nil, // prompter — standalone: the handler creates (and routes) its own
		nil, // tbClient — SmartGuide path not exercised by tests; falls back to mock/claude as configured
		pairing,
		nil, // store — not needed in test harness
	)

	manager.OnMessage(handler.HandleMessage)

	h := &TestHarness{
		Setting:      setting,
		Handler:      handler,
		ChatStore:    chatStore,
		SessionMgr:   sessionMgr,
		AgentService: agentService,
		Pairing:      pairing,
		DataDir:      opt.DataDir,
		Manager:      manager,
	}
	if h.cleanup != nil {
		t.Cleanup(h.cleanup)
	}
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
