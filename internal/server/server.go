package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/constant"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/data/db"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	guardrailsutils "github.com/tingly-dev/tingly-box/internal/guardrails/utils"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/probe"
	"github.com/tingly-dev/tingly-box/internal/server/advisortool"
	"github.com/tingly-dev/tingly-box/internal/server/background"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/hooks"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	imbotmodule "github.com/tingly-dev/tingly-box/internal/server/module/imbot"
	oauthmodule "github.com/tingly-dev/tingly-box/internal/server/module/oauth"
	providerQuotaModule "github.com/tingly-dev/tingly-box/internal/server/module/provider_quota"
	"github.com/tingly-dev/tingly-box/internal/server/processor"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/pkg/auth"
	pkgobs "github.com/tingly-dev/tingly-box/pkg/obs"
	pkgotel "github.com/tingly-dev/tingly-box/pkg/otel"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
	"github.com/tingly-dev/tingly-box/remote/channel"
	"github.com/tingly-dev/tingly-box/remote/interaction"
	"github.com/tingly-dev/tingly-box/remote/scenario"
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
	errorMW         *middleware.ErrorLogMiddleware
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
	oauthRefresher *background.OAuthRefresher

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
	affinityStore *AffinityStore

	// routing selector for service selection pipeline
	routingSelector *routing.SimpleSelector

	// vision proxy processor, reused by the scenario-level vision proxy plugin
	// (also registered into the smart-routing registry for the proxy_vision op)
	visionProxyProcessor *processor.VisionProxyProcessor

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
	var errorMW *middleware.ErrorLogMiddleware
	errorLogPath := filepath.Join(cfg.ConfigDir, constant.LogDirName, constant.DebugLogFileName)
	errorMW = middleware.NewErrorLogMiddleware(errorLogPath, 10)

	// Set filter expression from config
	filterExpr := cfg.GetErrorLogFilterExpression()
	if filterExpr != "" {
		if err := errorMW.SetFilterExpression(filterExpr); err != nil {
			logrus.Debugf("Warning: Failed to set error log filter expression '%s': %v, using default", filterExpr, err)
		} else {
			logrus.Debugf("ErrorLog middleware initialized with filter: %s, logging to: %s", filterExpr, errorLogPath)
		}
	} else {
		logrus.Debugf("ErrorLog middleware initialized with default filter, logging to: %s", errorLogPath)
	}

	// Create server struct first with applied options
	server.jwtManager = jwtManager
	server.engine = gin.New()
	server.clientPool = client.NewClientPool() // Initialize client pool (once mode with auto-cleanup via finalizer)
	server.errorMW = errorMW
	server.scenarioRecordSinks = make(map[typ.RuleScenario]*obs.Sink)
	historyStore := guardrailsutils.NewStore(200, GetGuardrailsHistoryPath(cfg.ConfigDir))
	grRuntime := server.currentGuardrailsRuntime()
	if grRuntime == nil {
		server.setGuardrailsRuntimeRef(&guardrails.Guardrails{History: historyStore})
	} else if grRuntime.HistoryStore() == nil {
		grRuntime.SetHistoryStore(historyStore)
	}

	// Auto-load guardrails if enabled and not injected explicitly.
	server.initGuardrailsRuntime()
	server.refreshGuardrailsCredentialCacheOrWarn("server init")

	// Initialize record sink if recording is enabled
	switch server.recordMode {
	case "":
		// Recording disabled
	case obs.RecordModeAll:
		recordSink := obs.NewSink(server.recordDir, server.recordMode, server.sinkOpts()...)
		server.clientPool.SetRecordSink(recordSink)
		logrus.Debugf("Request recording enabled, mode: %s, directory: %s", server.recordMode, server.recordDir)
	case obs.RecordModeScenario, obs.RecordModeRequestOnly, obs.RecordModeRequestResponse, obs.RecordModeStagedRequestResponse:
		// Scenario recording is now on-demand, created when scenario flag is enabled
		logrus.Debugf("Scenario recording mode enabled, sinks will be created on-demand per scenario")
	default:
		log.Panicf("Unknown recording mode %s", server.recordMode)
	}

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
	affinityStore := NewAffinityStore(0) // 0 = use default TTL

	// Initialize routing selector with pipeline. Pass multiLogger so smart
	// routing stages emit per-request evaluation traces to the smart_routing
	// log source viewable from the frontend system log page.
	serviceSelector := routing.NewServiceSelectorWithLogger(cfg, affinityStore, loadBalancer, server.multiLogger)
	simpleSelector := routing.NewSimpleSelector(serviceSelector)

	// Initialize load balancer API
	loadBalancerAPI := NewLoadBalancerAPI(loadBalancer, cfg)

	// Initialize OAuth manager and handler
	// Note: BaseURL will be dynamically updated for providers with port constraints
	registry := oauth.DefaultRegistry()
	oauthConfig := &oauth.Config{
		BaseURL:           fmt.Sprintf("http://localhost:%d", cfg.GetServerPort()),
		ProviderConfigs:   make(map[ai.Issuer]*oauth.ProviderConfig),
		TokenStorage:      oauth.NewMemoryTokenStorage(),
		StateExpiry:       10 * time.Minute,
		TokenExpiryBuffer: 5 * time.Minute,
	}
	oauthManager := oauth.NewManager(oauthConfig, registry)

	// Initialize token refresher for OAuth auto-refresh
	tokenRefresher := background.NewTokenRefresher(oauthManager, cfg)

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

	// Register op-level processors (vision proxy, etc.) into the smart-routing
	// registry. Idempotent — safe across config reloads.
	server.visionProxyProcessor = processor.RegisterAll(server.clientPool, server.config, logrus.StandardLogger())

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
	// The smart-routing callback closes over the server so probe doesn't import server.
	server.probeE2EService = probe.NewE2EService(cfg, server.clientPool, server.SelectServiceFromSmartRouting)
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

	// Initialize provider quota manager
	if err := server.initQuotaManager(cfg); err != nil {
		logrus.WithError(err).Warn("Failed to initialize provider quota manager")
	}

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

		// Update error log filter expression if changed
		if s.errorMW != nil {
			newFilterExpr := newConfig.GetErrorLogFilterExpression()
			if newFilterExpr != "" {
				if err := s.errorMW.SetFilterExpression(newFilterExpr); err != nil {
					logrus.Errorf("Failed to update error log filter expression: %v", err)
				} else {
					logrus.Debugf("Error log filter expression updated: %s", newFilterExpr)
				}
			}
		}

		// Re-sync guardrails based on updated config flags.
		s.syncGuardrailsFromConfig()

		// Re-register adviser with expanded env vars in case advisor config changed.
		s.registerAdviserFromConfig()
	})
}

