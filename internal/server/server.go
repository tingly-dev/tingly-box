package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/server/affinity"

	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/ai/quota"
	"github.com/tingly-dev/tingly-box/ai/quota/fetcher"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/server/advisortool"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/hooks"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	imbotmodule "github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/providerquota"
	"github.com/tingly-dev/tingly-box/internal/server/module/tokenrefresh"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
	"github.com/tingly-dev/tingly-box/pkg/auth"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
	pkgotel "github.com/tingly-dev/tingly-box/pkg/otel"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
	"github.com/tingly-dev/tingly-box/vmodel/virtualserver"
)

// Server represents the HTTP server
type Server struct {
	config     *config.Config
	jwtManager *auth.JWTManager
	engine     *gin.Engine
	httpServer *http.Server
	watcher    *config.Watcher

	// multi-mode logger for text + JSON + memory output
	multiLogger *pkgobs.MultiLogger

	// middleware
	authMW          *middleware.AuthMiddleware
	memoryLogMW     *middleware.MultiModeMemoryLogMiddleware
	loadBalancer    *LoadBalancer
	loadBalancerAPI *LoadBalancerAPI
	healthMonitor   *loadbalance.HealthMonitor

	// client pool for caching
	clientPool *client.ClientPool

	// OAuth manager
	oauthManager *oauth.Manager

	// OAuth handler (module)
	oauthHandler *oauthmodule.Handler

	// ImBot lifecycle controller (module). Concrete impl is *imbot.Handler;
	// using the interface keeps the seam narrow and replaces the previous
	// untyped interface{} that required inline type assertions everywhere.
	imbotSettingsHandler imbotmodule.LifecycleController

	// remote middle-layer state. The channel registry holds running
	// imbot Channel adapters; the interaction registry stores pending
	// long-poll results; the scenario registry routes /tingly/:scenario
	// events to plugins (claudecode is the first one). All three are
	// lazily constructed in UseUIEndpoints; the notify HTTP module
	// falls back to desktop notifications when they're absent.
	channelRegistry     *channel.Registry
	interactionRegistry *interaction.Registry[interaction.Result]
	scenarioRegistry    *scenario.Registry

	// OAuth refresher for OAuth auto-refresh
	oauthRefresher *tokenrefresh.OAuthRefresher

	// OAuth callback server (for providers requiring specific ports like Codex on 1455)
	oauthCallbackServer *http.Server

	// Dynamic callback servers (one per active OAuth flow)
	callbackServers   map[string]*oauth.CallbackServer
	callbackServersMu sync.RWMutex

	// template manager for provider templates
	templateManager *data.TemplateManager

	// probeE2EService runs SDK-level end-to-end probes for the /api/v2/probe endpoint.
	probeE2EService *probe.E2EService

	// probeLightweight powers /api/v2/probe/lightweight — optional key validation.
	probeLightweight *probe.LightweightService

	// mcp runtime for external MCP tools
	mcpRuntime *mcpruntime.Runtime

	// servertool pipeline — owns virtual tool providers and hook list
	servertoolPipeline *servertool.Pipeline

	// guardrails runtime (optional)
	guardrailsRuntime   *guardrails.Guardrails
	guardrailsRuntimeMu sync.RWMutex
	guardrailsConfigMu  sync.Mutex

	// recording sinks
	recordSink *obs.Sink

	// scenario-specific recording sinks (created on-demand when recording flag is enabled)
	scenarioRecordSinks   map[typ.RuleScenario]*obs.Sink
	scenarioRecordSinksMu sync.RWMutex

	// affinity store for smart routing session-model locking
	affinityStore *affinity.AffinityStore

	// routing selector for service selection pipeline
	routingSelector *routing.SimpleSelector

	// vision proxy service, reused by the scenario-level vision proxy plugin
	// (also registered into the smart-routing registry for the proxy_vision op)
	visionProxyService *visionproxy.Service

	// OTel meter setup for unified token tracking
	meterSetup   *pkgotel.MeterSetup
	tokenTracker *tracker.TokenTracker

	// virtual model service for testing
	virtualModelService *virtualserver.Service

	// quota manager for provider quota tracking
	quotaManager providerQuotaModule.Manager

	// options
	enableUI      bool
	enableAdaptor bool
	openBrowser   bool
	host          string
	debug         bool

	// record options
	recordMode obs.RecordMode
	recordDir  string
	recordCAS  bool // additionally write content-addressed slim records + blobs

	// recording flag - enables dual-stage request recording
	enableRecording bool

	// remote control lifecycle management
	remoteCoderCtx    context.Context
	remoteCoderCancel context.CancelFunc
	remoteCoderMu     sync.Mutex

	// lifecycle management for long-lived background components created during setup
	ctx    context.Context
	cancel context.CancelFunc

	// custom auth middleware (optional, for TBE integration)
	customUserAuthMiddleware  gin.HandlerFunc // For Web UI routes
	customModelAuthMiddleware gin.HandlerFunc // For Model API routes

	version string

	// controlHandler is the WebUI Management API's aggregate handler
	// (internal/server/webui.ControlHandler). Constructed as the LAST step of
	// NewServer, after every field it depends on (memoryLogMW, multiLogger,
	// config, jwtManager, ...) has already been set — do not move this
	// construction earlier without checking every field it reads.
	controlHandler *WebHandler

	// guardrailsHandler is the WebUI Management API's guardrails admin
	// handler (internal/server/webui.GuardrailsHandler). Same construction
	// constraint as controlHandler above.
	guardrailsHandler *GuardrailsHandler

	// aiHandler is the AI Model API's aggregate handler
	// (internal/server/aimodel.AIHandler), covering MCP-in-gateway dispatch,
	// recording, and (eventually) protocol dispatch/transform/passthrough.
	// Same last-step construction constraint as controlHandler above — every
	// field/callback in aimodel.Deps must already be set.
	aiHandler *ProtocolHandler
}

// NewServer creates a new HTTP server instance with functional options
func NewServer(cfg *config.Config, opts ...ServerOption) *Server {
	ctx, cancel := context.WithCancel(context.Background())

	// Start with default options
	allOpts := append([]ServerOption{WithDefault()}, opts...)

	// Default options
	server := &Server{
		config: cfg,
		ctx:    ctx,
		cancel: cancel,
	}

	// Apply all options (defaults + provided)
	for _, opt := range allOpts {
		opt(server)
	}

	// Set gin mode based on debug flag
	if server.debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	// Check and generate tokens if needed
	jwtManager := auth.NewJWTManager(cfg.GetJWTSecret())

	if !cfg.HasUserToken() {
		log.Println("No user token found in global config, generating new user token...")
		apiKey, err := jwtManager.GenerateAPIKey("user")
		if err != nil {
			logrus.Debugf("Failed to generate user API key: %v", err)
		} else {
			if err := cfg.SetUserToken(apiKey); err != nil {
				logrus.Debugf("Failed to save generated user token: %v", err)
			} else {
				logrus.Debugf("Generated and saved new user API token: %s", apiKey)
			}
		}
	} else {
		logrus.Debugf("Using existing user token from global config")
	}

	if !cfg.HasModelToken() {
		log.Println("No model token found in global config, generating new model token...")
		apiKey, err := jwtManager.GenerateAPIKey("model")
		if err != nil {
			logrus.Debugf("Failed to generate model API key: %v", err)
		} else {
			apiKey = "tingly-box-" + apiKey
			if err := cfg.SetModelToken(apiKey); err != nil {
				logrus.Debugf("Failed to save generated model token: %v", err)
			} else {
				logrus.Debugf("Generated and saved new model API token: %s", apiKey)
			}
		}
	} else {
		logrus.Debugf("Using existing model token from global config")
	}

	// Create server struct first with applied options
	server.jwtManager = jwtManager
	server.engine = gin.New()
	server.clientPool = client.NewClientPool()
	server.scenarioRecordSinks = make(map[typ.RuleScenario]*obs.Sink)
	historyStore := guardrailsutils.NewStore(200, config.HistoryPath(cfg.ConfigDir))
	grRuntime := server.currentGuardrailsRuntime()
	if grRuntime == nil {
		server.setGuardrailsRuntimeRef(&guardrails.Guardrails{History: historyStore})
	} else if grRuntime.HistoryStore() == nil {
		grRuntime.SetHistoryStore(historyStore)
	}

	// Auto-load guardrails if enabled and not injected explicitly.
	server.initGuardrailsRuntime()
	server.refreshGuardrailsCredentialCacheOrWarn("server init")

	// Log recording flag if enabled
	if server.enableRecording {
		logrus.Debugf("Dual-stage recording enabled")
	}

	// Initialize multi-mode memory log middleware for HTTP request logging
	// Logs are written to both multi-mode logger (persistence) and memory (quick access)
	memoryLogMW := middleware.NewMultiModeMemoryLogMiddleware(server.multiLogger)

	// Initialize API token manager (for multi-tenant authentication)
	var apiTokenManager *auth.APITokenManager
	if cfg.IsMultiTenantEnabled() {
		apiTokenMgr, err := auth.NewAPITokenManager(auth.APITokenManagerConfig{
			SecretKey:     cfg.GetAPITokenSecret(),
			SigningMethod: cfg.GetAPITokenAlgorithm(),
			Issuer:        cfg.GetAPITokenIssuer(),
		})
		if err != nil {
			logrus.Warnf("Failed to create API token manager: %v", err)
		} else {
			apiTokenManager = apiTokenMgr
		}
	}

	// Initialize auth middleware
	authMW := middleware.NewAuthMiddleware(cfg, jwtManager, apiTokenManager, nil) // apiTokenStore will be set later

	// Initialize health monitor
	healthMonitor := loadbalance.NewHealthMonitor(cfg.HealthMonitor)

	// Initialize health filter
	healthFilter := typ.NewHealthFilter(healthMonitor)

	// Initialize template manager first (needed for capacity config)
	var templateURL string
	if cfg.ProviderTemplateSource != "" {
		templateURL = cfg.ProviderTemplateSource
	} else {
		templateURL = data.TemplateGitHubURL
	}
	templateManager := data.NewTemplateManager(templateURL)
	if err := templateManager.Initialize(context.Background()); err != nil {
		logrus.Debugf("Failed to fetch from GitHub, using embedded provider templates: %v", err)
	} else {
		logrus.Debugf("Provider templates initialized (version: %s)", templateManager.GetVersion())
	}

	// Initialize load balancer
	loadBalancer := NewLoadBalancer(cfg, healthFilter)

	// Initialize affinity store for smart routing
	affinityStore := affinity.NewAffinityStore(0) // 0 = use default TTL

	// Initialize routing selector with pipeline. Pass multiLogger so smart
	// routing stages emit per-request evaluation traces to the smart_routing
	// log source viewable from the frontend system log page.
	serviceSelector := routing.NewServiceSelectorWithLogger(cfg, affinityStore, loadBalancer, server.multiLogger)
	simpleSelector := routing.NewSimpleSelector(serviceSelector)

	// Initialize load balancer API
	loadBalancerAPI := NewLoadBalancerAPI(loadBalancer, cfg)

	// Initialize OAuth module dependencies.
	// Per-flow callback base URLs are passed as request options for providers with port constraints.
	oauthConfig := oauth.NewConfig(
		oauth.WithConfigBaseURL(fmt.Sprintf("http://localhost:%d", cfg.GetServerPort())),
	)
	oauthManager := oauth.NewManager(
		oauth.WithConfig(oauthConfig),
		oauth.WithRegistry(oauth.DefaultRegistry()),
	)
	tokenRefresher := tokenrefresh.NewTokenRefresher(
		tokenrefresh.WithTokenManager(oauthManager),
		tokenrefresh.WithProviderConfig(cfg),
	)

	// Register provider lifecycle hooks for automatic cache invalidation
	poolHook := hooks.NewClientPoolInvalidationHook(server.clientPool)
	cfg.RegisterProviderUpdateHook(poolHook)
	cfg.RegisterProviderDeleteHook(poolHook)
	logrus.Debug("Registered client pool invalidation hook for provider updates")

	// Update server with dependencies
	server.authMW = authMW
	server.memoryLogMW = memoryLogMW
	server.loadBalancer = loadBalancer
	server.loadBalancerAPI = loadBalancerAPI
	server.healthMonitor = healthMonitor
	server.oauthManager = oauthManager
	server.oauthRefresher = tokenRefresher
	server.affinityStore = affinityStore
	server.routingSelector = simpleSelector

	// Wire the vision proxy service. Idempotent — safe across config reloads.
	server.visionProxyService = visionproxy.NewServiceFromPool(server.clientPool, server.config)

	// Start affinity store background GC
	affinityStore.StartGC()

	// Initialize OAuth handler
	server.oauthHandler = oauthmodule.NewHandler(oauthManager, cfg)
	// Set callback server manager (the server itself implements this interface)
	server.oauthHandler.SetCallbackServerManager(server)

	server.templateManager = templateManager

	// Set template manager in config for model fetching fallback
	server.config.SetTemplateManager(templateManager)

	server.mcpRuntime = mcpruntime.NewRuntime(cfg.GetMCPRuntimeConfig)
	server.mcpRuntime.SetClientPool(server.clientPool)
	// Auto-register built-in tools (e.g., webtools) if not already present
	if err := mcpruntime.RegisterBuiltinTools(cfg.GetMCPRuntimeConfig, cfg.SetToolConfig); err != nil {
		logrus.WithError(err).Warn("mcp: failed to register builtin tools")
	}

	// Register adviser as virtual tool if configured
	server.registerAdviserFromConfig()

	// E2E probe service handles /api/v2/probe end-to-end without touching *Server.
	server.probeE2EService = probe.NewE2EService(cfg, server.clientPool)
	server.probeLightweight = probe.NewLightweightService(server.clientPool)

	// Initialize OTel meter setup for token tracking
	sm := cfg.StoreManager()
	if sm == nil {
		logrus.Warnf("StoreManager not available, skipping OTel meter setup")
	} else {
		meterSetup, err := pkgotel.NewMeterSetup(context.Background(), pkgotel.DefaultConfig(), &pkgotel.StoreRefs{
			StatsStore: sm.Stats(),
			UsageStore: sm.Usage(),
			Sink:       server.recordSink,
		})
		if err != nil {
			logrus.Warnf("Failed to initialize OTel meter setup: %v", err)
		} else if meterSetup != nil {
			server.meterSetup = meterSetup
			server.tokenTracker = meterSetup.Tracker()
			logrus.Debugf("OTel meter setup initialized")
		}

		// Initialize API token store for multi-tenant authentication
		if apiTokenManager != nil {
			apiTokenStore := sm.APIToken()
			if apiTokenStore != nil {
				// Update auth middleware with API token store
				server.authMW = middleware.NewAuthMiddleware(cfg, jwtManager, apiTokenManager, apiTokenStore)
				logrus.Debugf("API token store initialized for multi-tenant authentication")
			}
		}
	}

	// Initialize virtual model service
	server.virtualModelService = virtualserver.NewService()
	logrus.Debugf("Virtual model service initialized with default models")

	// Seed builtin virtual-model providers (idempotent). These become first-class
	// rows in the provider store so they show up in the standard UI and dispatch
	// pipeline; the dispatcher short-circuits to the in-process handler when it
	// resolves to a vmodel provider.
	if store := cfg.GetProviderStore(); store != nil {
		if err := server.virtualModelService.EnsureBuiltinProviders(store); err != nil {
			logrus.WithError(err).Warn("Failed to seed builtin virtual-model providers")
		} else {
			logrus.Debugf("Builtin virtual-model providers seeded")
		}
	}

	// Wire in-process vmodel clients into the pool so virtual providers
	// traverse the exact same dispatch path as real providers.
	vmodelProvider := &typ.Provider{Name: "vmodel-internal", AuthType: typ.AuthTypeVirtual}
	server.clientPool.SetVirtualClients(
		vmodelclient.NewOpenAIClient(server.virtualModelService.GetOpenAIRegistry(), vmodelProvider),
		vmodelclient.NewAnthropicClient(server.virtualModelService.GetAnthropicRegistry(), vmodelProvider),
	)

	// Initialize provider quota manager
	quotaMgr, err := initQuotaManager(cfg)
	if err != nil {
		logrus.WithError(err).Warn("Failed to initialize provider quota manager")
	} else {
		server.quotaManager = quotaMgr
	}

	// Construct the WebUI Management API's control handler. This MUST be the
	// last step before setupMiddleware/setupRoutes — every field it reads
	// (memoryLogMW, multiLogger, jwtManager, config) needs to already be set.
	server.controlHandler = NewWebHandler(WebDeps{
		MemoryLogMW: server.memoryLogMW,
		MultiLogger: server.multiLogger,
		Config:      server.config,
		JWTManager:  server.jwtManager,
	})

	// Construct the WebUI Management API's guardrails admin handler. Same
	// last-step constraint as controlHandler above — Runtime is *Server
	// itself via the exported adapter methods in guardrails_runtime_adapter.go.
	server.guardrailsHandler = NewGuardrailsHandler(GuardrailsDeps{
		Config:             server.config,
		Runtime:            server,
		GuardrailsConfigMu: &server.guardrailsConfigMu,
	})

	// Construct the AI Model API's aggregate handler. Same last-step
	// constraint as controlHandler above. The callback fields reach back into
	// root state that has not moved to aimodel yet (usage tracking, affinity
	// store, recording sinks, guardrails runtime) — see aimodel.Deps.
	server.aiHandler = NewHandler(ProtocolHandlerDeps{
		Config:                   server.config,
		TokenTracker:             server.tokenTracker,
		HealthMonitor:            server.healthMonitor,
		ClientPool:               server.clientPool,
		LoadBalancer:             server.loadBalancer,
		MCPRuntime:               server.mcpRuntime,
		TemplateManager:          server.templateManager,
		RoutingSelector:          server.routingSelector,
		VisionProxyService:       server.visionProxyService,
		GetServertoolPipeline:    func() *servertool.Pipeline { return server.servertoolPipeline },
		TrackUsageWithTokenUsage: server.trackUsageWithTokenUsage,
		TrackUsageFromContext:    server.trackUsageFromContext,
		UpdateAffinityMessageID:  server.updateAffinityMessageID,
		GetOrCreateScenarioSink:  server.GetOrCreateScenarioSink,
		CurrentGuardrailsRuntime: server.currentGuardrailsRuntime,
		GetScenarioRecordMode:    server.GetScenarioRecordMode,
	})

	// Setup middleware
	server.setupMiddleware()

	// Setup routes
	server.setupRoutes(server.ctx)

	// Setup configuration watcher
	server.setupConfigWatcher()

	// Initialize dynamic callback servers map
	server.callbackServers = make(map[string]*oauth.CallbackServer)

	// Set up health monitor probe function. The lightweight probe (OPTIONS +
	// /models) is cheaper than a full chat request and adequate for liveness:
	// reachability + auth validity is what the health monitor cares about, not
	// per-endpoint capability (which is now declared, not probed).
	if server.healthMonitor != nil {
		server.healthMonitor.SetProbeFunc(func(serviceID string) bool {
			// serviceID format: "<providerUUID>:<model>" (from Service.ServiceID())
			parts := strings.Split(serviceID, ":")
			if len(parts) < 1 {
				return false
			}
			providerUUID := parts[0]

			provider, err := cfg.GetProviderByUUID(providerUUID)
			if err != nil || provider == nil {
				return false
			}

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			return server.probeLightweight.Probe(ctx, provider).Valid
		})
	}

	return server
}

// UsageStore returns the server's usage store instance for internal integrations.
func (s *Server) UsageStore() *db.UsageStore {
	if s == nil || s.config == nil {
		return nil
	}
	sm := s.config.StoreManager()
	if sm == nil {
		return nil
	}
	return sm.Usage()
}

// GetRoutingSelector returns the server's routing selector for service selection.
func (s *Server) GetRoutingSelector() *routing.SimpleSelector {
	return s.routingSelector
}

func (s *Server) Context() context.Context {
	return s.ctx
}

func (s *Server) Cancel() context.CancelFunc {
	return s.cancel
}

// registerAdviserFromConfig reads the MCP config and registers the adviser
// virtual tool if an enabled advisor source is found.
func (s *Server) registerAdviserFromConfig() {
	mcpCfg := s.mcpRuntime.GetConfig()
	if mcpCfg == nil {
		s.servertoolPipeline = servertool.NewPipeline()
		return
	}
	for _, source := range mcpCfg.Sources {
		if source.Advisor == nil || source.Enabled == nil || !*source.Enabled {
			continue
		}
		advisorCfg := *source.Advisor

		if advisorCfg.ProviderResolver == nil {
			advisorCfg.ProviderResolver = s.config.GetProviderByUUID
		}

		pipeline := servertool.NewPipeline()
		pipeline.Register(advisortool.NewProvider(advisorCfg, s.clientPool, s.mcpRuntime.SessionStore()))
		pipeline.RegisterInto(s.mcpRuntime.VirtualRegistry())
		s.servertoolPipeline = pipeline

		logrus.Info("mcp: registered adviser via servertool pipeline")
		return
	}

	// No advisor configured — empty pipeline.
	s.servertoolPipeline = servertool.NewPipeline()
}

// setupConfigWatcher initializes the configuration hot-reload watcher
func (s *Server) setupConfigWatcher() {
	watcher, err := config.NewConfigWatcher(s.config)
	if err != nil {
		logrus.Debugf("Failed to create config watcher: %v", err)
		return
	}

	// Add default watch file (main config file)
	if err := watcher.AddWatchFile(s.config.ConfigFile); err != nil {
		logrus.Debugf("Failed to add config file to watcher: %v", err)
		return
	}

	s.watcher = watcher

	// Add callback for configuration changes
	watcher.AddCallback(func(newConfig *config.Config) {
		logrus.Debugln("Configuration updated, reloading...")
		// Update JWT manager with new secret if changed
		s.jwtManager = auth.NewJWTManager(newConfig.JWTSecret)
		logrus.Debugln("JWT manager reloaded with new secret")

		// Re-sync guardrails based on updated config flags.
		s.syncGuardrailsFromConfig()

		// Re-register adviser with expanded env vars in case advisor config changed.
		s.registerAdviserFromConfig()
	})
}

// initQuotaManager initializes the provider quota manager
func initQuotaManager(cfg *config.Config) (*quota.Manager, error) {
	// Create quota store
	store, err := quota.NewGormStore(cfg.ConfigDir, logrus.StandardLogger())
	if err != nil {
		return nil, err
	}

	// Create quota manager with default config
	qConfig := quota.DefaultConfig()
	quotaMgr := quota.NewManager(qConfig, store, cfg, logrus.StandardLogger())

	// Register all built-in fetchers
	fetcher.RegisterAll(quotaMgr, logrus.StandardLogger())

	logrus.Info("Provider quota manager initialized")
	return quotaMgr, nil
}

// applyVisionProxy is the single entry point for the vision proxy plugin,
// covering both the rule-level and scenario-level scopes. It must run before
// service selection (after the rule is resolved). Delegates to
// visionproxy.Service — see internal/server/module/visionproxy and
// .design/vision-proxy.md for the design.
func (s *Server) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
	s.visionProxyService.Apply(c.Request.Context(), s.config, scenarioType, rule, typedRequest)
}
