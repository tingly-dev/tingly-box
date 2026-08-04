package bot

import (
	"context"
	"fmt"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/channel/imchannel"
)

// runBotWithSettings starts a bot against the caller-supplied chat store.
// The host owns the bot's CHANNEL — the send/prompt surface: the shared
// IMPrompter, the remote.channel.Channel registration, and the routing of
// prompt replies — because the channel is a property of a running bot, not of
// any one purpose. The bot's behavior is supplied by the injected Consumers
// (remote_agent, notify, …), which are users of that channel; inbound
// messages go to the host's disabled-chat gate first, then the prompt-reply
// router, then through the consumers in order until one claims, so the
// catch-all (remote_agent) sits last. The gate must run before the
// prompt-reply router: it is the one place that can drop a disabled chat's
// traffic before promptReplyRouter, which sits ahead of every consumer's own
// blocklist check, gets to answer it.
//
// chatStore is injected into the Manager and shared by every bot it runs —
// see Manager.SetChatStore. This function must not close it.
func runBotWithSettings(ctx context.Context, setting BotSetting, chatStore ChatStoreInterface, consumers []Consumer, pairing *PairingManager, channels *channel.Registry, accessStore AccessStore, authorizer access.Authorizer) error {
	// Create platform-specific auth config
	authConfig := buildAuthConfig(setting)
	platform := imbot.Platform(setting.Platform)

	if len(consumers) == 0 {
		return fmt.Errorf("no inbound consumer")
	}

	manager := imbot.NewManager(
		imbot.WithAutoReconnect(true),
		imbot.WithMaxReconnectAttempts(5),
		imbot.WithReconnectDelay(3000),
	)

	options := map[string]interface{}{
		"updateTimeout": 30,
	}
	if setting.ProxyURL != "" {
		options["proxy"] = setting.ProxyURL
	}

	// Some platforms carry extra credentials as connection options rather than
	// as auth fields (Weixin's user_id / base_url). Which ones is a fact in
	// imbot's platform table.
	for k, v := range imbot.AuthOptions(setting.Platform, setting.Auth) {
		options[k] = v
	}
	err := manager.AddBot(&imbot.Config{
		UUID:     setting.UUID,
		Platform: platform,
		Enabled:  true,
		Auth:     authConfig,
		Options:  options,
	})
	if err != nil {
		return fmt.Errorf("failed to start %s bot: %w", setting.Platform, err)
	}

	// The bot's shared channel prompter. Consumers send prompts through it
	// (directly or via the registered Channel below); the host routes every
	// reply back to it ahead of consumer dispatch.
	prompter := imchannel.NewIMPrompter(manager)

	// Bind the bot's behavior through the injected consumers. This is the
	// decoupling seam: the lifecycle here knows nothing about the agent /
	// SmartGuide machinery — each consumer owns its own.
	attachedList := make([]*Attached, 0, len(consumers))
	for _, consumer := range consumers {
		attached, err := consumer.Attach(ctx, setting, manager, prompter, chatStore, pairing)
		if err != nil {
			return fmt.Errorf("attach inbound consumer %q: %w", consumer.Name(), err)
		}
		if attached.Cleanup != nil {
			defer attached.Cleanup()
		}
		attachedList = append(attachedList, attached)
	}
	handlers := make([]OnMessage, 0, len(attachedList)+2)
	handlers = append(handlers, DisabledChatGate(chatStore))
	if accessStore != nil && authorizer != nil {
		handlers = append(handlers, AuthorizationGate(accessStore, authorizer, chatStore, setting.IsRequirePairing(), func(chatID string) (access.CapabilityName, access.ActionName) {
			pending := prompter.GetPendingRequestsForChat(chatID)
			if len(pending) == 0 {
				return "", ""
			}
			capability := access.CapabilityRemoteControl
			action := access.ActionRemoteControlApprove
			if value, ok := pending[0].Metadata["capability"].(string); ok {
				capability = access.CapabilityName(value)
			}
			if value, ok := pending[0].Metadata["reply_action"].(string); ok {
				action = access.ActionName(value)
			}
			return capability, action
		}, func(msg imbot.Message, platform imbot.Platform, botUUID string) {
			b := manager.GetBot(botUUID, platform)
			if b == nil {
				return
			}
			opts := &imbot.SendMessageOptions{
				Text: "⚠️ You are not authorized to answer this request. Ask the bot owner to grant the approve permission for this chat.",
			}
			ForwardReplyContext(opts, msg)
			_, _ = b.SendMessage(ctx, msg.GetReplyTarget(), opts)
		}))
	}
	handlers = append(handlers, promptReplyRouter(manager, prompter))
	for _, attached := range attachedList {
		if attached.OnMessage != nil {
			handlers = append(handlers, attached.OnMessage)
		}
	}
	manager.OnMessage(func(msg imbot.Message, platform imbot.Platform, botUUID string) {
		for _, h := range handlers {
			if h(msg, platform, botUUID) {
				return
			}
		}
	})

	if err := manager.Start(ctx); err != nil {
		return fmt.Errorf("failed to start bot manager: %w", err)
	}

	// Mint and surface a pairing code if pairing is required for this bot.
	if setting.IsRequirePairing() && pairing != nil {
		code, expiresAt := pairing.Mint(setting.UUID)
		if code != "" {
			source := "explicit"
			if setting.RequirePairing == nil {
				source = "platform default"
			}
			logrus.WithFields(logrus.Fields{
				"uuid":       setting.UUID,
				"name":       setting.Name,
				"platform":   setting.Platform,
				"source":     source,
				"expires_at": expiresAt.Format(time.RFC3339),
			}).Warnf("Pairing code: %s — DM /bind %s within %s",
				code, code, time.Until(expiresAt).Round(time.Second))
			fmt.Fprintf(os.Stderr,
				"\n[tingly-box] Bot %q (%s) pairing code: %s  (expires %s, %s)\nIn the bot DM, send: /bind %s\n\n",
				setting.Name, setting.Platform, code,
				expiresAt.Format(time.RFC3339), source, code)
		}
	}

	// Setup menu button after bot is connected
	// This is called here so it applies to all code paths using runBotWithSettings
	bot := manager.GetBotByUUID(setting.UUID)
	if bot != nil {
		platform := bot.PlatformInfo().ID

		// Register the bot's channel: every running bot exposes itself as a
		// remote.channel.Channel so scenario plugins routed through
		// /tingly/:scenario/notify can Send/Prompt into its chats. This is
		// host infrastructure, not a purpose — it lives exactly as long as
		// the bot runs.
		if channels != nil {
			channels.Register(imchannel.New(setting.UUID, setting.Platform, bot, prompter))
			defer channels.Unregister(setting.UUID)
		}

		// Platform menu comes from the first consumer that supplies one
		// (today only remote_agent does).
		var commandRegistry *imbot.CommandRegistry
		for _, attached := range attachedList {
			if attached.CommandRegistry != nil {
				commandRegistry = attached.CommandRegistry
				break
			}
		}

		if commandRegistry != nil {
			// Platforms without a native command menu are a no-op inside
			// SetupCommandMenu, so there is nothing to branch on here.
			err := imbot.SetupCommandMenu(bot, platform, commandRegistry)
			if err != nil {
				// Log warning but don't fail startup
				logrus.WithError(err).WithField("platform", setting.Platform).Warn("Failed to setup menu button")
			} else {
				logrus.WithField("platform", setting.Platform).Info("Menu button configured successfully")
			}
		}
	}

	// Wait for context cancellation
	// The manager will automatically clean up when context is cancelled
	<-ctx.Done()

	return nil
}

// buildAuthConfig creates the platform auth config from a bot's stored auth
// map, driven by imbot's platform table rather than a switch here.
//
// The switch this replaces omitted "lark" in both this function and the
// credential check below, so Lark bots were rejected for having no valid
// credentials — and would have been handed a token-type config the Feishu
// client rejects even if they had got that far.
func buildAuthConfig(setting BotSetting) imbot.AuthConfig {
	return imbot.BuildAuthConfig(setting.Platform, setting.Auth)
}

// Manager manages the lifecycle of running bot instances
type Manager struct {
	mu        sync.RWMutex
	running   map[string]*runningBot // uuid -> runningBot
	store     SettingsStore
	consumers []Consumer        // Supply each bot's inbound behavior, in dispatch order (the decoupling seam)
	pairing   *PairingManager   // Pairing-code (TOFU) manager
	channels  *channel.Registry // Remote channel registry for /tingly/:scenario routing (optional)

	// chatStore is the ONE chat store every bot in this manager shares, and
	// it is injected rather than opened here — the manager does not get to
	// know where chats live. Sharing is not an optimization: a per-bot store
	// is how concurrent bots used to erase each other's chats.
	// See .design/remote-storage.md.
	chatStore       ChatStoreInterface
	capabilityStore interface {
		GetCapability(context.Context, string, access.CapabilityName) (access.BotCapability, bool, error)
	}
	accessStore AccessStore
	authorizer  access.Authorizer
}

// NewManager creates a new bot manager with a settings store and the
// consumers that supply every bot's behavior. The consumers are the
// decoupling seam: swapping them changes what a bot does without touching the
// lifecycle here.
//
// Order matters twice: inbound messages are dispatched (after the host's own
// prompt-reply routing) to each mounted consumer in this order until one
// claims the message, so the catch-all (remote_agent) goes last; and a bot
// runs only while at least one consumer reports Mounted for it.
func NewManager(store SettingsStore, consumers ...Consumer) *Manager {
	return &Manager{
		running:   make(map[string]*runningBot),
		store:     store,
		consumers: consumers,
		pairing:   NewPairingManager(NewLogAuditor()),
	}
}

// mountedConsumers returns the subset of the manager's consumers that are
// mounted on the given bot, preserving dispatch order.
func (m *Manager) mountedConsumers(setting BotSetting) []Consumer {
	mounted := make([]Consumer, 0, len(m.consumers))
	for _, c := range m.consumers {
		if c == nil {
			continue
		}
		if m.capabilityStore != nil {
			name := access.CapabilityName(c.Name())
			if c.Name() == "remote_agent" {
				name = access.CapabilityRemoteControl
			}
			capability, found, err := m.capabilityStore.GetCapability(context.Background(), setting.UUID, name)
			if err == nil && found && capability.Enabled {
				mounted = append(mounted, c)
			}
			continue
		}
		if c.Mounted(setting) {
			mounted = append(mounted, c)
		}
	}
	return mounted
}

// SetCapabilityStore makes explicit BotCapability rows the lifecycle source
// of truth. The legacy Mounted method remains only for standalone callers
// that have not wired the final-state persistence layer.
func (m *Manager) SetCapabilityStore(store interface {
	GetCapability(context.Context, string, access.CapabilityName) (access.BotCapability, bool, error)
}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.capabilityStore = store
}

func (m *Manager) SetAccessStore(store AccessStore) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.accessStore = store
	m.capabilityStore = store
	if store != nil {
		m.authorizer = access.NewEvaluator(store)
	} else {
		m.authorizer = nil
	}
}

// SetChannelRegistry wires a remote channel registry so each running
// bot exposes itself as a remote.channel.Channel reachable from
// /tingly/:scenario scenario plugins. Safe to call once at startup
// before any bot is started.
func (m *Manager) SetChannelRegistry(reg *channel.Registry) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels = reg
}

// PairingManager returns the manager's PairingManager instance. Used by CLI
// helpers that mint, rotate, or revoke pairing codes.
func (m *Manager) PairingManager() *PairingManager {
	return m.pairing
}

// ChatStore returns the chat store shared by every bot this manager runs.
//
// The store is not owned here — whoever injected it (the StoreManager, in the
// server) closes it. Callers must not.
func (m *Manager) ChatStore() (ChatStoreInterface, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.chatStore == nil {
		return nil, fmt.Errorf("chat store not configured")
	}
	return m.chatStore, nil
}

// SetChatStore injects the chat store every bot will share. Call it before
// starting any bot; Start fails without one.
func (m *Manager) SetChatStore(store ChatStoreInterface) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chatStore = store
}

// Start starts a bot by UUID
func (m *Manager) Start(parentCtx context.Context, uuid string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Refuse to run a bot that cannot persist anything, before doing any
	// other work: a bot with no chat store silently forgets every binding
	// and pairing, which is worse than not starting.
	if m.chatStore == nil {
		return fmt.Errorf("chat store not configured")
	}

	// Check if already running or stopping
	if rb, exists := m.running[uuid]; exists {
		if rb.stopped {
			// Bot is stopping, wait for it to finish
			logrus.WithField("uuid", uuid).Debug("Bot is stopping, cannot start yet")
			return fmt.Errorf("bot is stopping, please try again later")
		}
		logrus.WithField("uuid", uuid).Debug("Bot already running")
		return nil
	}

	s, err := m.store.GetSettingsByUUID(uuid)
	if err != nil {
		return err
	}

	platform := s.Platform
	auth := s.Auth
	name := s.Name

	if platform == "" {
		return fmt.Errorf("unknown platform: %s", platform)
	}

	// Which credentials a platform needs is a fact in imbot's platform table,
	// not a switch here. Naming the missing keys also turns an opaque "no valid
	// credentials" into something an operator can act on.
	if missing := imbot.MissingAuthKeys(platform, auth); len(missing) > 0 {
		logrus.WithFields(logrus.Fields{
			"uuid":     uuid,
			"platform": platform,
			"missing":  strings.Join(missing, ", "),
		}).Warn("Bot is missing required credentials, not starting")
		return fmt.Errorf("bot for platform %s is missing required credentials: %s",
			platform, strings.Join(missing, ", "))
	}

	// Mount gate: a bot is a resource that only runs when it has an active
	// purpose mounted. Each consumer decides for itself whether it is mounted
	// (remote_agent via its mount switch, notify via outbound scenario
	// bindings); a bot with no mounted consumer stays offline even when
	// Enabled — this is the "no mount, no bot" half of the resource/consumer
	// split. Starting with no mount is a no-op (not an error) so reconcile
	// stays quiet.
	mounted := m.mountedConsumers(s)
	if len(mounted) == 0 {
		logrus.WithFields(logrus.Fields{
			"uuid":     uuid,
			"name":     name,
			"platform": platform,
		}).Info("Bot has no active mount, not starting")
		return nil
	}

	// SmartGuide routing-rule sync is a remote_agent concern and now lives in
	// the inbound consumer (NewBotHandler ensures the rule when it builds the
	// handler), so the lifecycle no longer touches the TBClient here.
	//
	// Every bot shares the manager's one chat store; opening a per-bot store
	// here is what used to make concurrent bots erase each other's writes.
	chatStore := m.chatStore

	// Create cancellable context for this bot
	ctx, cancel := context.WithCancel(parentCtx)
	doneChan := make(chan struct{})
	m.running[uuid] = &runningBot{cancel: cancel, doneChan: doneChan}

	// Start bot in goroutine
	pairing := m.pairing
	channels := m.channels
	accessStore, authorizer := m.accessStore, m.authorizer
	go m.runBotSupervised(ctx, uuid, s, chatStore, mounted, pairing, channels, accessStore, authorizer, doneChan)

	logrus.WithField("uuid", uuid).WithField("name", name).WithField("platform", platform).Info("Bot started")
	return nil
}

// runBotSupervised executes runBotWithSettings with panic recovery so a crash in
// any third-party IM SDK is contained to this bot's goroutine instead of
// propagating to the runtime and taking down the whole tingly-box process.
// Always closes doneChan and removes the bot from the running map, regardless
// of whether the bot exited normally, with error, or via panic.
func (m *Manager) runBotSupervised(
	ctx context.Context,
	uuid string,
	s BotSetting,
	chatStore ChatStoreInterface,
	consumers []Consumer,
	pairing *PairingManager,
	channels *channel.Registry,
	accessStore AccessStore,
	authorizer access.Authorizer,
	doneChan chan struct{},
) {
	defer close(doneChan)
	defer m.removeRunning(uuid)

	defer func() {
		if r := recover(); r != nil {
			stack := string(debug.Stack())
			logrus.WithFields(logrus.Fields{
				"uuid":     uuid,
				"name":     s.Name,
				"platform": s.Platform,
				"panic":    fmt.Sprintf("%v", r),
				"stack":    stack,
			}).Error("Bot goroutine panicked; isolated from main process")
		}
	}()

	if err := runBotWithSettings(ctx, s, chatStore, consumers, pairing, channels, accessStore, authorizer); err != nil {
		logrus.WithError(err).WithField("uuid", uuid).Warn("Bot stopped with error")
	}
	logrus.WithField("uuid", uuid).Info("Bot stopped")
}

// Stop stops a bot by UUID
func (m *Manager) Stop(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if rb, exists := m.running[uuid]; exists {
		logrus.WithField("uuid", uuid).Info("Stopping bot")
		rb.stopped = true // Mark as stopping
		rb.cancel()
		// SmartGuide routing-rule cleanup is a remote_agent concern and runs
		// via the inbound consumer's Cleanup when the bot goroutine exits on
		// ctx cancel. Don't delete from map yet — let the goroutine clean up.
	}
}

// WaitForStop waits for a bot to finish stopping (with timeout)
func (m *Manager) WaitForStop(uuid string, timeout time.Duration) bool {
	m.mu.RLock()
	rb, exists := m.running[uuid]
	if !exists {
		m.mu.RUnlock()
		return true // Already stopped
	}
	doneChan := rb.doneChan
	m.mu.RUnlock()

	if doneChan == nil {
		return true
	}

	select {
	case <-doneChan:
		return true
	case <-time.After(timeout):
		logrus.WithFields(logrus.Fields{
			"uuid":    uuid,
			"timeout": timeout.String(),
		}).Warn("Timeout waiting for bot to stop; goroutine may still be running and could leak resources or duplicate connections on restart")
		return false
	}
}

// IsRunning checks if a bot is running
func (m *Manager) IsRunning(uuid string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.running[uuid]
	return exists
}

// StartEnabled starts all enabled bots
func (m *Manager) StartEnabled(ctx context.Context) error {
	settings, err := m.store.ListEnabledSettings()
	if err != nil {
		return err
	}

	for _, setting := range settings {
		if setting.UUID == "" {
			continue
		}
		if err := m.Start(ctx, setting.UUID); err != nil {
			logrus.WithError(err).WithField("uuid", setting.UUID).Warn("Failed to start bot")
		}
	}

	return nil
}

// StopAll stops all running bots
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for uuid, rb := range m.running {
		logrus.WithField("uuid", uuid).Info("Stopping bot")
		rb.stopped = true // Mark as stopping
		rb.cancel()
		// Don't delete from map - let goroutines clean up
	}
}

// Sync ensures the running bots match the enabled settings in the store.
// It starts bots that are enabled but not running, and stops bots that are running but disabled.
func (m *Manager) Sync(ctx context.Context) error {
	settings, err := m.store.ListEnabledSettings()
	if err != nil {
		return err
	}

	// Compute the set of bots that SHOULD be running: enabled AND with at
	// least one mounted consumer. A bot that is enabled but that no purpose
	// uses (remote_agent off, no outbound bindings) is a resource nobody is
	// using, so it must not run — and if it is running (last mount just
	// turned off), the stop pass below takes it down.
	shouldRun := make(map[string]bool)
	for _, setting := range settings {
		if setting.UUID != "" && len(m.mountedConsumers(setting)) > 0 {
			shouldRun[setting.UUID] = true
		}
	}

	// Start bots that should run but aren't (Start re-checks the mount gate).
	for uuid := range shouldRun {
		if !m.IsRunning(uuid) {
			if err := m.Start(ctx, uuid); err != nil {
				logrus.WithError(err).WithField("uuid", uuid).Warn("Failed to start bot during sync")
			}
		}
	}

	// Stop bots that are running but should not be (disabled or mount off).
	m.mu.Lock()
	for uuid := range m.running {
		if !shouldRun[uuid] {
			logrus.WithField("uuid", uuid).Info("Stopping bot during sync (disabled or no active mount)")
			// Mark as stopping and cancel
			if rb, exists := m.running[uuid]; exists {
				rb.stopped = true
				rb.cancel()
			}
		}
	}
	m.mu.Unlock()

	return nil
}

// removeRunning removes a bot from the running map (must be called with lock held or from within locked method)
func (m *Manager) removeRunning(uuid string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.running, uuid)
}
