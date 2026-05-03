// Package testenv is a Go-only test harness for any imbot-based bot. It
// provides a fluent API to drive simulated user input through the tingly
// platform and to assert on outbound bot actions (sends, edits, reactions,
// inline keyboards, etc.).
//
// Typical use:
//
//	env := testenv.NewTestEnv(t)
//	alice := env.NewUser("alice")
//	chat := alice.OpenDM(env.BotUUID())
//	chat.SendText("/help")
//	chat.WaitText(2*time.Second).AssertContains(t, "Available commands")
//
// Inbound messages flow asynchronously through bot.OnMessage handlers
// (imbot/core/base.go:282-289 dispatches each handler in its own goroutine),
// so tests must use the WaitX helpers and not assume synchronous delivery.
package testenv

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/core"
	"github.com/tingly-dev/tingly-box/imbot/platform/tingly"
)

// TestEnv is the root of the harness. It owns one or more
// InProcessTransports keyed by bot UUID and an imbot.Manager that drives
// bots talking through those transports.
type TestEnv struct {
	t TestingT

	mu         sync.Mutex
	transports map[string]*tingly.InProcessTransport
	botUUIDs   []string
	manager    *imbot.Manager
	managerCtx context.Context
	cancel     context.CancelFunc
	defaultBot string

	idSeq atomic.Int64
}

// TestingT is the subset of *testing.T we depend on. Letting tests provide
// any implementation makes the harness usable from sub-frameworks.
type TestingT interface {
	Helper()
	Cleanup(func())
	Errorf(format string, args ...any)
	Fatalf(format string, args ...any)
	Logf(format string, args ...any)
}

// NewTestEnv creates a new harness scoped to the test t. It registers a
// cleanup that disconnects all bots and unregisters their transports.
func NewTestEnv(t TestingT) *TestEnv {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	env := &TestEnv{
		t:          t,
		transports: make(map[string]*tingly.InProcessTransport),
		manager:    imbot.NewManager(imbot.WithAutoReconnect(false)),
		managerCtx: ctx,
		cancel:     cancel,
	}
	t.Cleanup(env.cleanup)
	return env
}

// Manager returns the underlying imbot.Manager. Tests that need to
// register OnMessage handlers themselves use this directly.
func (e *TestEnv) Manager() *imbot.Manager { return e.manager }

// Context returns the manager's lifecycle context.
func (e *TestEnv) Context() context.Context { return e.managerCtx }

// BotUUID returns the UUID of the default bot (creating one on first use).
// Most tests have a single bot and this is the only call they need.
func (e *TestEnv) BotUUID() string {
	e.mu.Lock()
	if e.defaultBot == "" {
		e.mu.Unlock()
		_, uuid := e.AddTinglyBot()
		return uuid
	}
	defer e.mu.Unlock()
	return e.defaultBot
}

// AddTinglyBot registers a fresh InProcessTransport for a generated UUID,
// adds the bot to the imbot.Manager, and returns both the transport and
// the UUID. The first bot created becomes the default bot.
func (e *TestEnv) AddTinglyBot() (*tingly.InProcessTransport, string) {
	uuid := fmt.Sprintf("tingly-bot-%d", e.idSeq.Add(1))
	tr := e.AddTinglyBotWithUUID(uuid)
	return tr, uuid
}

// AddTinglyBotWithUUID is like AddTinglyBot but lets the caller pick the
// UUID (useful when the test wants a stable bot identity).
func (e *TestEnv) AddTinglyBotWithUUID(uuid string) *tingly.InProcessTransport {
	tr := tingly.NewInProcessTransport()
	tingly.Register(uuid, tr)

	cfg := &core.Config{
		UUID:     uuid,
		Platform: core.PlatformTingly,
		Enabled:  true,
		Auth:     core.AuthConfig{Type: "none"},
	}
	if err := e.manager.AddBot(cfg); err != nil {
		tingly.Unregister(uuid)
		e.t.Fatalf("AddTinglyBot: AddBot failed: %v", err)
	}

	e.mu.Lock()
	e.transports[uuid] = tr
	e.botUUIDs = append(e.botUUIDs, uuid)
	if e.defaultBot == "" {
		e.defaultBot = uuid
	}
	e.mu.Unlock()
	return tr
}

// Transport returns the transport associated with a bot UUID.
func (e *TestEnv) Transport(uuid string) *tingly.InProcessTransport {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.transports[uuid]
}

// nextID mints a unique synthetic id with the given prefix.
func (e *TestEnv) nextID(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, e.idSeq.Add(1))
}

func (e *TestEnv) cleanup() {
	e.cancel()
	_ = e.manager.Stop(context.Background())
	e.mu.Lock()
	uuids := append([]string(nil), e.botUUIDs...)
	e.mu.Unlock()
	for _, uuid := range uuids {
		tingly.Unregister(uuid)
	}
}
