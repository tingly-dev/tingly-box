// Package aimodel hosts the AI Model API surface split out of internal/server:
// the LLM gateway routes (/tingly/:scenario/...) covering OpenAI/Anthropic
// compatible chat/completions/responses/messages/embeddings/images, MCP tool
// handling during model requests, protocol dispatch/transform/passthrough,
// failover/load-balance dispatch, guardrails runtime evaluation, and
// usage/token tracking.
//
// This package intentionally does not depend on *server.Server. Handlers are
// built from the narrow ProtocolHandlerDeps struct below, following the same pattern
// already used by internal/server/module/* (see module/scenario/handler.go).
package server

import (
	"fmt"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/data"
	"github.com/tingly-dev/tingly-box/internal/guardrails"
	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/recording"
	"github.com/tingly-dev/tingly-box/internal/server/routing"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/internal/visionproxy"
	"github.com/tingly-dev/tingly-box/pkg/otel/tracker"
)

// ProtocolHandlerDeps declares exactly what the AI Model API handlers need from the host
// server. It is populated and passed in once, from server.NewServer, after
// all of *Server's fields have been constructed.
//
// This grows as each subsequent migration step moves a file in and wires up
// the fields/methods it actually touches on *Server today.
type ProtocolHandlerDeps struct {
	Config *config.Config

	// TokenTracker records usage to the OTel meter pipeline (may be nil if
	// OTel setup failed at startup — callers must nil-check).
	TokenTracker *tracker.TokenTracker

	// HealthMonitor reports per-service health outcomes (success / rate
	// limit / auth error / general error) back into the load-balance health
	// filter (may be nil — callers must nil-check).
	HealthMonitor *loadbalance.HealthMonitor

	// ClientPool caches upstream provider clients (OpenAI/Anthropic/Google).
	ClientPool *client.ClientPool

	// LoadBalancer selects the active service for a rule (tier/random/etc
	// tactics). Used by failover to pick the next candidate on retry.
	LoadBalancer *LoadBalancer

	// TemplateManager resolves per-provider model metadata (max tokens,
	// description, context window) from the provider template catalog.
	TemplateManager *data.TemplateManager

	// RoutingSelector runs the full selection pipeline (health → affinity →
	// smart → strategy) for a scenario/rule/request, used by the top-level
	// OpenAI/Anthropic entry handlers (Step 10).
	RoutingSelector *routing.SimpleSelector

	// VisionProxyService rewrites image content into text descriptions for
	// scenarios/rules configured with a vision proxy plugin. Concrete type,
	// not a callback: internal/visionproxy.Service takes *config.Config
	// directly and has no *server.Server dependency.
	VisionProxyService *visionproxy.Service

	// MCPRuntime holds the virtual tool registry and advisor state for
	// external MCP tools invoked during a live model request.
	MCPRuntime *mcpruntime.Runtime

	// GetServertoolPipeline returns the current virtual-tool-provider pipeline
	// (advisor quota hooks, etc.). A callback rather than a plain field
	// because root's config hot-reload (registerAdviserFromConfig) reassigns
	// *Server.servertoolPipeline in place — a plain field copied at
	// NewHandler time would silently go stale on the next reload. May return
	// nil — callers fall back to servertool.NewDefaultExecutor.
	GetServertoolPipeline func() *servertool.Pipeline

	// The callbacks below reach back into root *Server state that has not
	// (yet) moved to aimodel: OTel usage-tracking wiring, the affinity
	// store, scenario recording sinks, and the guardrails runtime pointer.
	// Wiring them as funcs keeps this package independent of *server.Server
	// while letting already-moved gateway logic still reach that state.
	TrackUsageWithTokenUsage func(c *gin.Context, usage *protocol.TokenUsage, err error)
	TrackUsageFromContext    func(c *gin.Context, inputTokens, outputTokens int, err error)
	UpdateAffinityMessageID  func(c *gin.Context, rule *typ.Rule, messageID string)
	GetOrCreateScenarioSink  func(scenario typ.RuleScenario) *obs.Sink
	CurrentGuardrailsRuntime func() *guardrails.Guardrails

	// GetScenarioRecordMode resolves the effective recording mode for a
	// scenario. Backed by root's s.recordMode/s.scenarioRecordSinks, which
	// have not moved to aimodel (recording lifecycle stays root-owned).
	GetScenarioRecordMode func(scenario typ.RuleScenario) obs.RecordMode
}

// ProtocolHandler is the aggregate handler for the AI Model API. Individual method
// files (openai_*.go, anthropic_*.go, protocol_*.go, etc.) will be moved
// here in later steps and become methods on *ProtocolHandler.
type ProtocolHandler struct {
	deps ProtocolHandlerDeps

	// mcpTC caches the stateless MCP chain transforms (see
	// protocol_transform.go); they depend only on construction-time deps.
	mcpTC mcpTransformCache
}

// NewHandler constructs the AI Model API handler from its dependencies.
func NewHandler(deps ProtocolHandlerDeps) *ProtocolHandler {
	return &ProtocolHandler{deps: deps}
}

// The methods below are thin wrappers wiring the Deps callbacks to the
// unexported call sites moved-in gateway files use (matching the style of
// their pre-move `s.xxx(...)` calls on *Server). Nil-checked since the
// corresponding Deps field may be unset in tests that construct a bare
// Handler.

func (ph *ProtocolHandler) trackUsageWithTokenUsage(c *gin.Context, usage *protocol.TokenUsage, err error) {
	if ph.deps.TrackUsageWithTokenUsage == nil {
		return
	}
	ph.deps.TrackUsageWithTokenUsage(c, usage, err)
}

func (ph *ProtocolHandler) trackUsageFromContext(c *gin.Context, inputTokens, outputTokens int, err error) {
	if ph.deps.TrackUsageFromContext == nil {
		return
	}
	ph.deps.TrackUsageFromContext(c, inputTokens, outputTokens, err)
}

func (ph *ProtocolHandler) updateAffinityMessageID(c *gin.Context, rule *typ.Rule, messageID string) {
	if ph.deps.UpdateAffinityMessageID == nil {
		return
	}
	ph.deps.UpdateAffinityMessageID(c, rule, messageID)
}

func (ph *ProtocolHandler) currentGuardrailsRuntime() *guardrails.Guardrails {
	if ph.deps.CurrentGuardrailsRuntime == nil {
		return nil
	}
	return ph.deps.CurrentGuardrailsRuntime()
}

func (ph *ProtocolHandler) guardrailsEnabledForScenario(scenario string) bool {
	return GuardrailsEnabledForScenario(ph.deps.Config, ph.currentGuardrailsRuntime(), scenario)
}

func (ph *ProtocolHandler) mcpEnabled() bool {
	return MCPEnabled(ph.deps.Config)
}

// mcpStripDisabledToolsEnabled returns whether dangerous disabled MCP strip
// is enabled. Pure function of Config, mirrors root's (now-removed)
// s.mcpStripDisabledToolsEnabled.
func (ph *ProtocolHandler) mcpStripDisabledToolsEnabled() bool {
	if ph.deps.Config == nil {
		return false
	}
	cfg := ph.deps.Config.GetMCPRuntimeConfig()
	if cfg == nil {
		return false
	}
	return cfg.StripDisabledMCPTools
}

func (ph *ProtocolHandler) getScenarioRecordMode(scenario typ.RuleScenario) obs.RecordMode {
	if ph.deps.GetScenarioRecordMode == nil {
		return ""
	}
	return ph.deps.GetScenarioRecordMode(scenario)
}

// MCPEnabled centralizes the MCP feature-flag check so gateway handlers do
// not repeat scenario-flag lookups. Mirrors GuardrailsEnabledForScenario.
func MCPEnabled(cfg *config.Config) bool {
	if cfg == nil {
		return false
	}
	return cfg.GetScenarioFlag(typ.ScenarioGlobal, config.ExtensionMCP) ||
		cfg.GetScenarioFlag(typ.ScenarioClaudeCode, config.ExtensionMCP)
}

// applyVisionProxy is the single entry point for the vision proxy plugin,
// covering both the rule-level and scenario-level scopes. It must run before
// service selection (after the rule is resolved). Delegates to
// visionproxy.Service — see internal/server/module/visionproxy and
// .design/vision-proxy.md for the design.
func (ph *ProtocolHandler) applyVisionProxy(c *gin.Context, scenarioType typ.RuleScenario, rule *typ.Rule, typedRequest any) {
	if ph.deps.VisionProxyService == nil {
		return
	}
	ph.deps.VisionProxyService.Apply(c.Request.Context(), ph.deps.Config, scenarioType, rule, typedRequest)
}

// isEnterpriseContextPresent reports whether the request carries an
// enterprise-runtime identity (already authorized upstream by TBE).
func isEnterpriseContextPresent(c *gin.Context) bool {
	return strings.TrimSpace(c.GetString("enterprise_user_id")) != ""
}

// resolveSessionID returns the session identifier for the current request as
// a string. It delegates to routing.ResolveSessionID which checks (in
// priority order): Anthropic metadata.user_id > X-Tingly-Session-ID header >
// ClientIP fallback.
func resolveSessionID(c *gin.Context, req interface{}) typ.SessionID {
	return routing.ResolveSessionID(c, req)
}

// determineRuleWithScenario resolves the active *typ.Rule for a scenario +
// request model name, honoring the probe headers used by service-target
// probes (X-Tingly-Probe-Rule / X-Tingly-Probe-Service).
func (ph *ProtocolHandler) determineRuleWithScenario(ctx *gin.Context, scenario typ.RuleScenario, modelName string) (*typ.Rule, error) {
	cfg := ph.deps.Config

	// X-Tingly-Probe-Rule: load a specific rule by UUID (for applying its flags
	// while service selection is overridden by X-Tingly-Probe-Service).
	if ruleUUID := ctx.GetHeader("X-Tingly-Probe-Rule"); ruleUUID != "" {
		if cfg != nil {
			if rule := cfg.GetRuleByUUID(ruleUUID); rule != nil {
				return rule, nil
			}
		}
		return nil, fmt.Errorf("probe rule not found: %s", ruleUUID)
	}

	// X-Tingly-Probe-Service: no matching rule needed — build a minimal synthetic
	// rule so the handler can proceed with service selection pinned by the header.
	if probeService := ctx.GetHeader("X-Tingly-Probe-Service"); probeService != "" {
		if providerUUID, model, ok := strings.Cut(probeService, ":"); ok {
			svc := &loadbalance.Service{Provider: providerUUID, Model: model, Active: true}
			return &typ.Rule{
				UUID:         ProbeSyntheticRuleUUID,
				Scenario:     scenario,
				RequestModel: model,
				Services:     []*loadbalance.Service{svc},
				Active:       true,
			}, nil
		}
	}

	if cfg != nil {
		// Use the new MatchRuleByModelAndScenario which supports wildcard matching
		rule := cfg.MatchRuleByModelAndScenario(modelName, scenario)
		if rule != nil && rule.Active {
			return rule, nil
		}
		// Enterprise runtime context is already authorized by TBE.
		// If endpoint scenario has no matching rule, allow lookup by model across scenarios.
		if isEnterpriseContextPresent(ctx) {
			for _, anyRule := range cfg.GetRequestConfigs() {
				if anyRule.Active && anyRule.RequestModel == modelName {
					return &anyRule, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("provider or model not configured for request model '%s'", modelName)
}

// EnsureProtocolRecorder returns a ProtocolRecorder for the given scenario,
// reusing any recorder already stored in the gin context. Returns nil when
// recording is disabled (no sink) or the request body cannot be read.
//
// GetOrCreateScenarioSink is a ProtocolHandlerDeps callback rather than a direct call
// because scenario sink lifecycle (creation, mutex, recordDir) still lives on
// root *Server — see ProtocolHandlerDeps.
func (ph *ProtocolHandler) EnsureProtocolRecorder(c *gin.Context, scenario string, provider *typ.Provider, model string, mode obs.RecordMode, bs []byte) *recording.ProtocolRecorder {
	if rec, ok := recording.GetRecorderFromContext(c); ok {
		rec.BindProvider(provider, model, mode)
		return rec
	}

	if ph.deps.GetOrCreateScenarioSink == nil {
		return nil
	}
	scenarioType := typ.RuleScenario(scenario)
	sink := ph.deps.GetOrCreateScenarioSink(scenarioType)
	if sink == nil {
		return nil
	}

	rec, err := recording.NewProtocolRecorder(c, sink, scenario, mode, bs)
	if err != nil {
		logrus.Debugf("obs: failed to build ProtocolRecorder: %v", err)
		return nil
	}
	rec.BindProvider(provider, model, mode)
	c.Set(recording.RecorderContextKey, rec)
	return rec
}
