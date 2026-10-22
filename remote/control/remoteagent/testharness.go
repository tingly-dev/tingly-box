// testharness.go is TEST INFRASTRUCTURE, not runtime code: it stands up a real
// db-backed environment (chat store, pairing) so the
// remoteagent tests can drive the production BotHandler end-to-end. The
// internal/data/db import is intentional here — building a real store manager
// is host-side wiring that does not belong on the runtime path, and this file
// is only linked into test binaries. The runtime packages (bot, remoteagent
// non-test code) remain free of db.* imports.

package remoteagent

import (
	"context"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/internal/db"
	"github.com/tingly-dev/tingly-box/remote/control/bot"
	"github.com/tingly-dev/tingly-box/remote/control/feature"
)

// TestHarness wires the production BotHandler against a test imbot.Manager
// (typically backed by the tingly platform). It owns the support
// infrastructure — chat store, pairing — and
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
	Setting   bot.BotSetting
	Handler   *BotHandler
	ChatStore bot.ChatStoreInterface
	Pairing   *bot.PairingManager
	DataDir   string
	Manager   *imbot.Manager

	cleanup func()
}

// TestBootOptions tweaks BootForTest defaults. All fields are optional.
type TestBootOptions struct {
	// DataDir overrides the chat-store directory (default: t.TempDir()).
	DataDir string

	// Router hands chats to Projects (see ProjectRouter); nil leaves chat
	// control off. A test may also set it later with SetProjectRouter,
	// before messages flow.
	Router ProjectRouter
}

// BootForTest spins up a production BotHandler against the given
// imbot.Manager. It assumes the Manager already has a bot registered for
// setting.UUID (the tingly testenv arranges this via AddTinglyBotWithUUID
// when env.BotUUID() is called).
//
// The harness registers the BotHandler.HandleMessage callback on the
// Manager. Callers must Start the Manager themselves — keeping that step
// in the test makes it explicit when inbound messages start flowing.
func BootForTest(t *testing.T, manager *imbot.Manager, setting bot.BotSetting, opts ...TestBootOptions) *TestHarness {
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

	pairing := bot.NewPairingManager()
	dirBrowser := feature.NewDirectoryBrowser()

	ctx := t.Context()
	handler := NewBotHandler(ctx, setting, chatStore, dirBrowser, manager, pairing, nil)
	if opt.Router != nil {
		handler.SetProjectRouter(opt.Router)
	}

	manager.OnMessage(handler.HandleMessage)

	h := &TestHarness{
		Setting:   setting,
		Handler:   handler,
		ChatStore: chatStore,
		Pairing:   pairing,
		DataDir:   opt.DataDir,
		Manager:   manager,
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

// EnsureContext provides a context that propagates either through
// t.Context() (Go 1.24+) or a fresh background context.
func EnsureContext(t testing.TB) context.Context {
	type ctxer interface{ Context() context.Context }
	if c, ok := t.(ctxer); ok {
		return c.Context()
	}
	return context.Background()
}
