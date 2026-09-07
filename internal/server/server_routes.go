package server

import (
	"context"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/managedagent"
	"github.com/tingly-dev/tingly-box/internal/managedagent/agentrun"
	"github.com/tingly-dev/tingly-box/internal/managedagent/gitrepo"
	"github.com/tingly-dev/tingly-box/internal/middleware"
	managedagentmodule "github.com/tingly-dev/tingly-box/internal/server/module/managedagent"
	sharing "github.com/tingly-dev/tingly-box/internal/server/module/sharing"
	team "github.com/tingly-dev/tingly-box/internal/server/module/team"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/swagger"
)

// setupMiddleware configures server middleware
func (s *Server) setupMiddleware() {
	// Recovery middleware
	s.engine.Use(gin.Recovery())

	// Memory log middleware for HTTP request logging
	if s.memoryLogMW != nil {
		s.engine.Use(s.memoryLogMW.Middleware())
	}

	// CORS middleware
	s.engine.Use(middleware.CORS())
}

// setupRoutes configures server routes
func (s *Server) setupRoutes(ctx context.Context) {

	s.UseAIEndpoints()

	s.UseLoadBalanceEndpoints()

	// Multi-tenant token management API
	s.UseTokenManagementEndpoints()

	// Managed agent sessions control plane
	s.UseManagedAgentEndpoints()

	// Virtual model endpoints for testing
	s.UseVirtualModelEndpoints()

	// Integrate Web UI routes if enabled
	if s.enableUI {
		s.UseUIEndpoints(ctx)
	}
}

func (s *Server) UseAIEndpoints() {
	// The gateway route shape (/tingly/:scenario[/v1]/...) is owned by
	// protocolserver; the host only supplies the engine and model auth.
	s.aiHandler.RegisterRoutes(s.engine, s.getModelAuthMiddleware())
}

// UseVirtualModelEndpoints sets up the direct virtual-model entrypoints,
// split per protocol:
//
//	/virtual/openai/v1/{models,chat/completions,responses}
//	/virtual/anthropic/v1/{models,messages}
//
// These bypass the provider/rule/scenario pipeline and call the in-process
// handler directly — useful when a client wants a fixed URL pointed at the
// vmodel registry without configuring a provider. The protocol split
// ensures /models returns only the model IDs the chosen protocol can
// actually dispatch.
//
// The canonical path for virtual models in normal use is still
// /v1/messages and /v1/chat/completions, where a vmodel provider is dispatched
// like any other provider: through the SDK to the private in-memory
// virtualserver listener (see .design/vmodel-transport.md).
func (s *Server) UseVirtualModelEndpoints() {
	mw := s.getModelAuthMiddleware()

	openai := s.engine.Group("/virtual/openai")
	openai.Use(mw)
	s.virtualModelService.SetupOpenAIRoutes(openai)

	anthropic := s.engine.Group("/virtual/anthropic")
	anthropic.Use(mw)
	s.virtualModelService.SetupAnthropicRoutes(anthropic)
}

func (s *Server) UseLoadBalanceEndpoints() {
	// API routes for load balancer management
	api := s.engine.Group("/api/v1/load-balancer")
	api.Use(s.getUserAuthMiddleware()) // Require user authentication for management APIs

	// Load balancer API routes
	s.loadBalancerAPI.RegisterRoutes(api)
}

// UseTokenManagementEndpoints registers the token management API endpoints.
func (s *Server) UseTokenManagementEndpoints() {
	if s.config == nil {
		return
	}
	sm := s.config.StoreManager()
	if sm == nil {
		return
	}
	store := sm.APIToken()
	if store == nil {
		return
	}

	manager := swagger.NewRouteManager(s.engine)
	api := manager.NewGroup("api", "v1", "")
	api.Router.Use(s.getUserAuthMiddleware())
	sharing.RegisterRoutes(api, sharing.NewHandler(store))
	team.RegisterRoutes(api, team.NewHandler(sm.Team()))
}

// UseManagedAgentEndpoints registers /api/v1/agent/* — the managed agent
// control plane (.design/managed-agent.md) — and wires its local runtime:
// host-side git under <base>/agent, Claude Code through agentboot, gateway
// routing through the same TBClient the @cc executor uses.
func (s *Server) UseManagedAgentEndpoints() {
	if s.config == nil {
		return
	}
	sm := s.config.StoreManager()
	if sm == nil || sm.ManagedAgent() == nil {
		return
	}
	base := sm.BaseDir()
	eventLog, err := managedagent.NewEventLog(constant.GetAgentEventsDir(base))
	if err != nil {
		logrus.WithError(err).Error("managed agent: event log unavailable; endpoints disabled")
		return
	}
	bus := managedagent.NewEventBus(eventLog)
	stores := sm.ManagedAgent().Stores(bus)
	git := &gitrepo.Git{MirrorsDir: constant.GetAgentMirrorsDir(base)}

	tb := tbclient.NewTBClient(s.config)
	routing := agentrun.RoutingFunc(func(ctx context.Context, ccProfile string) ([]string, string, error) {
		if _, profileID := typ.ParseScenarioProfile(typ.RuleScenario(ccProfile)); profileID != "" {
			path, perr := tb.GetClaudeCodeSettingsPathForProfile(ctx, profileID)
			if perr == nil && path != "" {
				return nil, path, nil
			}
			logrus.WithError(perr).WithField("ccProfile", ccProfile).Warn("managed agent: profile settings unavailable; using main scenario")
		}
		env, eerr := tb.GetClaudeCodeEnv(ctx)
		if eerr != nil {
			return nil, "", eerr
		}
		// The main scenario is materialised as the "default" derived
		// settings file (user's ~/.claude/settings.json as the base, gateway
		// routing on top) rather than injected as process env. A managed
		// session must route deterministically: settings.json env outranks
		// process env in Claude Code, and the host running tb may itself
		// carry Claude Code variables (a CI runner, a Claude Code remote
		// session) that would otherwise silently win. Falls back to env if
		// the file cannot be written.
		envMap := make(map[string]string, len(env))
		for _, kv := range env {
			if i := strings.IndexByte(kv, '='); i > 0 {
				envMap[kv[:i]] = kv[i+1:]
			}
		}
		path, berr := agent.BuildCCProfileSettings("default", string(typ.ScenarioClaudeCode), "", envMap)
		if berr != nil {
			logrus.WithError(berr).Warn("managed agent: default settings unavailable; injecting gateway env instead")
			return env, "", nil
		}
		return env, path, nil
	})
	ccConfig := claude.DefaultConfig()
	ccConfig.DefaultExecutionTimeout = managedAgentTurnTimeout
	launcher, err := agentrun.New(agentrun.Config{
		Stores:  stores,
		Agent:   claude.NewAgentWithConfig(ccConfig),
		Git:     git,
		Routing: routing,
		Logger:  logrus.StandardLogger(),
	})
	if err != nil {
		logrus.WithError(err).Error("managed agent: launcher unavailable; endpoints disabled")
		return
	}
	svc := managedagent.NewService(managedagent.Config{
		Stores:        stores,
		Launcher:      launcher,
		Git:           agentrun.GitAdapter{Git: git},
		WorkspacesDir: constant.GetAgentWorkspacesDir(base),
	})
	if err := svc.EnsureDefaults(context.Background()); err != nil {
		logrus.WithError(err).Error("managed agent: failed to ensure default environment")
	}
	s.managedAgent = svc
	s.managedAgentBus = bus

	manager := swagger.NewRouteManager(s.engine)
	api := manager.NewGroup("api", "v1", "")
	api.Router.Use(s.getUserAuthMiddleware())
	managedagentmodule.RegisterRoutes(api, managedagentmodule.NewHandler(svc))
}

// managedAgentTurnTimeout bounds one agent turn. Coding tasks run far longer
// than a chat reply, so this is looser than the remote-control default.
const managedAgentTurnTimeout = 2 * time.Hour
