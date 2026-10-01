package server

import (
	"context"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/module"
	"github.com/tingly-dev/tingly-box/internal/server/module/codeximport"
	"github.com/tingly-dev/tingly-box/internal/server/module/configapply"
	debugmodule "github.com/tingly-dev/tingly-box/internal/server/module/debug"
	deskmodule "github.com/tingly-dev/tingly-box/internal/server/module/desk"
	"github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	mcpmodule "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	notifymodule "github.com/tingly-dev/tingly-box/internal/server/module/notify"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/providerquota"
	"github.com/tingly-dev/tingly-box/internal/server/module/sharing"
	"github.com/tingly-dev/tingly-box/internal/server/module/statusline"
	"github.com/tingly-dev/tingly-box/internal/server/module/team"
	"github.com/tingly-dev/tingly-box/internal/server/module/uiprefs"
	usagemodule "github.com/tingly-dev/tingly-box/internal/server/module/usage"
	virtualmodelmodule "github.com/tingly-dev/tingly-box/internal/server/module/virtualmodel"
	"github.com/tingly-dev/tingly-box/remote/access"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	remotescenario "github.com/tingly-dev/tingly-box/remote/scenario"
	"github.com/tingly-dev/tingly-box/remote/scenario/builtin/claudecode"
)

// This file is the single place that decides which HTTP modules the server
// mounts and how each is constructed. The running server and OpenAPI schema
// generation both go through it, so the route set cannot drift between the
// two. schema=true builds the same module set against the bare Server that
// GenerateOpenAPI uses: handlers are only referenced, never invoked, so
// store- and registry-backed dependencies are left nil.

// engineModules returns the modules that register on the bare engine
// (outside the swagger-managed /api groups): the Claude Code status line and
// the notify hooks. status is also returned because the desk module resolves
// routes through it.
func (s *Server) engineModules(schema bool) (mods []module.Module, status *statusline.Handler) {
	var quotaMgr statusline.QuotaManager
	if s.quotaManager != nil {
		quotaMgr = s.quotaManager
	}
	status = statusline.NewHandler(s.config, s.loadBalancer, statusline.NewCache(), quotaMgr)

	// Remote middle-layer wiring for /tingly/:scenario hook events.
	// When the bot settings store is reachable we set up:
	//   - channelRegistry: running bots register imbot-backed Channels
	//     (see internal/remote/channel/imchannel)
	//   - interactionRegistry: shared long-poll registry the wait
	//     endpoint reads from
	//   - scenarioRegistry: name → plugin (Phase 1 ships claudecode)
	// Everything stays nil for setups without imbot settings; in that
	// case the notify HTTP module falls back to desktop notifications.
	// Schema generation never builds the registries: it must not spin up
	// remote state just to describe routes.
	notify := notifymodule.NewHandler()
	if sm := s.config.StoreManager(); !schema && sm != nil && sm.ImBotSettings() != nil {
		s.channelRegistry = channel.NewRegistry()
		s.interactionRegistry = interaction.New[interaction.Result](30 * time.Second)
		s.scenarioRegistry = remotescenario.NewRegistry()
		s.scenarioRegistry.Register(claudecode.New(s.interactionRegistry))
		runtime := remotescenario.NewRouteRuntime(s.channelRegistry, sm.BotAccess(), access.NewEvaluator(sm.BotAccess()), RuntimeAuditSink())
		notify = notifymodule.NewHandlerWithRouting(s.scenarioRegistry, s.interactionRegistry, runtime)
	}
	return []module.Module{status, notify}, status
}

// apiModules returns the modules mounted on the authenticated /api groups
// after the core routes (see UseWebAPIEndpoints), in registration order.
func (s *Server) apiModules(ctx context.Context, schema bool, status *statusline.Handler) []module.Module {
	mods := []module.Module{
		s.oauthHandler,
		// Virtual-model management routes — expose registry contents for the
		// Credentials > Virtual Models sub-tab. The providers themselves are
		// served via the standard provider CRUD endpoints (Source=builtin).
		virtualmodelmodule.NewHandler(s.virtualModelService),
		// Runtime memory diagnostics — per-instance memstats + heap profile over
		// the production HTTP surface. Consumed by the duo harness (per-instance
		// leak attribution) and by live incident diagnosis.
		debugmodule.NewHandler(),
	}

	sm := s.config.StoreManager()
	if sm != nil {
		mods = append(mods, usagemodule.NewHandler(sm.Usage()))
	}

	// ImBot settings API routes. s.channelRegistry is passed in at
	// construction (not wired later via SetChannelRegistry) so every bot
	// NewHandler brings up — including bots enabled before the server
	// started, which the background sync starts immediately on its own
	// goroutine — registers itself as a remote.channel.Channel reachable from
	// /tingly/:scenario and /api/v1/bots/:bot/*. Passing it after the fact
	// left a startup race where such a bot could finish starting before the
	// registry landed and would then never expose a channel until manually
	// restarted. Schema generation has no registry (s.channelRegistry is nil).
	imbotHandler, err := imbot.NewHandler(ctx, s.config, s.channelRegistry)
	if err != nil {
		logrus.WithError(err).Warn("Failed to create imbotsettings handler, imbot settings APIs will not be available")
	} else {
		mods = append(mods, imbotHandler)
		if !schema {
			// Store handler reference for shutdown
			s.imbotSettingsHandler = imbotHandler
		}
	}

	// Bot interaction API — the general, caller-facing surface for driving a
	// running bot's channel: POST /api/v1/bots/:bot/{notify,interact} and
	// GET /api/v1/bots/:bot/interact/:id. Only registered at runtime when the
	// bot middle layer is wired (channelRegistry + interactionRegistry
	// present); without IM settings there is no channel to drive. Schema
	// generation always registers it with a nil-channel handler so the routes
	// appear in openapi.json. See .design/bot-interaction-api.md.
	if schema {
		mods = append(mods, notifymodule.NewBotAPIHandler(nil, nil, nil))
	} else if s.channelRegistry != nil && s.interactionRegistry != nil {
		// chatManager backs the chat lifecycle endpoints (GET /chats,
		// DELETE, PUT .../disabled) and the outbound blocklist check — it
		// scopes the shared chat store to each bot's platform and chat-id
		// lock. nil when no IM handler is wired, in which case those
		// endpoints report unavailable.
		var chatManager notifymodule.BotChatManager
		if imbotHandler != nil {
			chatManager = newBotChatManager(s.channelRegistry, imbotHandler)
		}
		mods = append(mods, notifymodule.NewBotAPIHandler(s.channelRegistry, s.interactionRegistry, chatManager, s.config.StoreManager().BotAccess()))
	}

	mods = append(mods,
		configapply.NewHandler(s.config, s.host),
		// UI-only preferences shared by the browser and desktop UIs
		uiprefs.NewHandler(s.config),
		codeximport.NewHandler(nil, s.config),
		mcpmodule.NewHandler(s.config, s.mcpRuntime),
		// Provider quota API routes — registered unconditionally (nil manager is
		// safe; Handler.available() answers 503 per request) so the routes and
		// their response models always appear in openapi.json regardless of
		// whether quota tracking happens to be configured on this build.
		providerQuotaModule.NewHandler(s.quotaManager, logrus.StandardLogger()),
	)

	// Desk: schema generation registers it with no service (never build the
	// live service there — see newDeskService); at runtime it is mounted only
	// when the service could be built.
	if schema {
		mods = append(mods, deskmodule.NewHandler(nil, nil).WithGate(s.deskEnabled))
	} else if s.desk = newDeskService(sm, s.config); s.desk != nil {
		mods = append(mods, deskmodule.NewHandler(s.desk, status).WithGate(s.deskEnabled))
	}

	if schema {
		// Schema generation only references these handlers, so no live token
		// store is required. At runtime they are mounted by
		// UseTokenManagementEndpoints, which does not depend on the UI.
		mods = append(mods, sharing.NewHandler(nil), team.NewHandler(nil))
	}
	return mods
}
