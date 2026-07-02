package server

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// IncomingAPIType describes which OpenAI-style endpoint the client originally
// hit on this gateway. Only consulted when the provider declares
// EndpointModeBoth; otherwise the provider's declared mode dictates the
// upstream endpoint regardless of what the client sent.
type IncomingAPIType string

const (
	IncomingAPIChat      IncomingAPIType = "chat"
	IncomingAPIResponses IncomingAPIType = "responses"
)

// ResolveOpenAIEndpoint picks an OpenAI endpoint using the optional per-rule
// override first, then the provider's declared OpenAIEndpointMode.
//
// Precedence:
//
//  1. Rule flag (flags.OpenAIEndpointOverride). Overrides provider settings.
//  2. provider.OpenAIEndpointMode:
//     EndpointModeUnknown / zero value → Chat
//     EndpointModeChat                 → Chat
//     EndpointModeResponses            → Responses
//     EndpointModeBoth                 → mirror incoming
//
// Rule override is honored unconditionally (per design intent). When an override
// conflicts with the provider's declared mode, a warning is logged but the override
// takes effect. This allows explicit routing control for debugging and special cases.
//
// Defaulting unknown providers to Chat (not "mirror incoming") is intentional:
// most OpenAI-compatible vendors implement only /chat/completions. Providers
// that genuinely support Responses must declare it via template or OAuth.
//
// When an incoming Responses request routes to Chat, Responses-only fields
// (previous_response_id, include, background, truncation, reasoning) are
// silently dropped by ConvertOpenAIResponsesToChat — the same posture as
// Anthropic→Chat downgrades. The user accepts this by declaring the mode.
//
// Pure function: no Server state, no probe lookups, no I/O.
func ResolveOpenAIEndpoint(provider *typ.Provider, flags typ.RuleFlags, incoming IncomingAPIType) (protocol.APIType, error) {
	if provider == nil {
		return "", fmt.Errorf("provider is required for endpoint selection")
	}

	mode := provider.OpenAIEndpointMode

	// Rule override takes first priority (per design intent from .design/openai-endpoint-routing.md)
	// Log warning when override conflicts with provider's declared mode
	switch ParseEndpointOverride(flags.OpenAIEndpointOverride) {
	case OverrideChat:
		if mode == ai.EndpointModeResponses {
			logrus.Warnf("Rule forces chat endpoint on responses-only provider %s", provider.UUID)
		}
		return protocol.TypeOpenAIChat, nil

	case OverrideResponses:
		if mode == ai.EndpointModeChat {
			logrus.Warnf("Rule forces responses endpoint on chat-only provider %s", provider.UUID)
		}
		return protocol.TypeOpenAIResponses, nil
	}

	// Fall back to provider mode when no override specified
	switch mode {
	case ai.EndpointModeResponses:
		return protocol.TypeOpenAIResponses, nil
	case ai.EndpointModeBoth, ai.EndpointModeAuto:
		// Both mirrors the incoming API. Auto also mirrors incoming here —
		// this is the non-auto fallback path taken when runtime endpoint
		// auto-detection is gated off; when enabled, resolveAutoTarget +
		// dispatchWithAutoFallback handle Auto providers instead.
		if incoming == IncomingAPIResponses {
			return protocol.TypeOpenAIResponses, nil
		}
		return protocol.TypeOpenAIChat, nil
	default: // EndpointModeChat / zero value
		return protocol.TypeOpenAIChat, nil
	}
}

// EndpointOverride is the typed value of the openai_endpoint_override rule
// flag. It forces an OpenAI request onto a specific endpoint, overriding the
// provider's declared OpenAIEndpointMode default (provider declarations
// trump conflicting overrides — see ResolveOpenAIEndpoint).
type EndpointOverride string

const (
	OverrideAuto      EndpointOverride = "auto"
	OverrideChat      EndpointOverride = "chat"
	OverrideResponses EndpointOverride = "responses"
)

// ParseEndpointOverride coerces a raw rule-flag string to a known
// EndpointOverride. Empty, "auto" and any unrecognized value map to
// OverrideAuto so misconfigured rules degrade safely.
func ParseEndpointOverride(s string) EndpointOverride {
	switch s {
	case string(OverrideChat):
		return OverrideChat
	case string(OverrideResponses):
		return OverrideResponses
	default:
		return OverrideAuto
	}
}

// logModeOverrideIgnored warns that a rule's openai_endpoint_override was
// discarded because the provider's declared OpenAIEndpointMode doesn't
// permit that target. Caller (ResolveOpenAIEndpoint) guarantees non-nil
// provider.
func logModeOverrideIgnored(provider *typ.Provider, requestedOverride string) {
	mode := string(provider.OpenAIEndpointMode)
	if mode == "" {
		mode = "chat"
	}
	logrus.Warnf("rule openai_endpoint_override=%s ignored: provider %s declares mode=%s", requestedOverride, provider.UUID, mode)
}

// ─────────────────────────────────────────────────────────────────────────
// Endpoint cache — per provider+model auto-detection results
// ─────────────────────────────────────────────────────────────────────────

const defaultEndpointCacheTTL = 24 * time.Hour

type endpointCacheEntry struct {
	target   protocol.APIType
	cachedAt time.Time
}

type EndpointCache struct {
	mu    sync.RWMutex
	store map[string]endpointCacheEntry
	ttl   time.Duration
}

func NewEndpointCache(ttl time.Duration) *EndpointCache {
	if ttl <= 0 {
		ttl = defaultEndpointCacheTTL
	}
	return &EndpointCache{
		store: make(map[string]endpointCacheEntry),
		ttl:   ttl,
	}
}

func endpointCacheKey(providerUUID, model string) string {
	return providerUUID + ":" + model
}

func (c *EndpointCache) Get(providerUUID, model string) (protocol.APIType, bool) {
	key := endpointCacheKey(providerUUID, model)
	c.mu.RLock()
	entry, ok := c.store[key]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if time.Since(entry.cachedAt) > c.ttl {
		c.mu.Lock()
		delete(c.store, key)
		c.mu.Unlock()
		return "", false
	}
	return entry.target, true
}

func (c *EndpointCache) Set(providerUUID, model string, target protocol.APIType) {
	key := endpointCacheKey(providerUUID, model)
	c.mu.Lock()
	c.store[key] = endpointCacheEntry{
		target:   target,
		cachedAt: time.Now(),
	}
	c.mu.Unlock()
}

// ─────────────────────────────────────────────────────────────────────────
// Runtime auto-detection — protocol fallback + failover orchestration
// ─────────────────────────────────────────────────────────────────────────

// alternateOpenAIProtocol returns the other OpenAI protocol type.
func alternateOpenAIProtocol(current protocol.APIType) protocol.APIType {
	if current == protocol.TypeOpenAIResponses {
		return protocol.TypeOpenAIChat
	}
	return protocol.TypeOpenAIResponses
}

// incomingToTarget maps IncomingAPIType to protocol.APIType.
func incomingToTarget(incoming IncomingAPIType) protocol.APIType {
	if incoming == IncomingAPIResponses {
		return protocol.TypeOpenAIResponses
	}
	return protocol.TypeOpenAIChat
}

// extractLastGinError returns the most recent error recorded on the gin
// context via c.Error(). Returns nil when no errors exist.
func extractLastGinError(c *gin.Context) error {
	errs := c.Errors
	if len(errs) == 0 {
		return nil
	}
	return errs[len(errs)-1].Err
}

// clearGinErrors removes all errors from the gin context so that a
// fallback retry starts with a clean slate.
func clearGinErrors(c *gin.Context) {
	c.Errors = c.Errors[:0]
}

// overrideToTarget converts an EndpointOverride to a protocol.APIType.
func overrideToTarget(ov EndpointOverride) protocol.APIType {
	if ov == OverrideResponses {
		return protocol.TypeOpenAIResponses
	}
	return protocol.TypeOpenAIChat
}

// scenarioPreferredProtocol returns the protocol an auto-mode provider
// should try first on a cache miss. Scenarios whose client ecosystem is
// natively Responses-based (Codex) start with Responses regardless of the
// incoming transport — providers serving such traffic overwhelmingly speak
// Responses, so leading with it saves a wasted first round trip. All other
// scenarios mirror the incoming API.
func scenarioPreferredProtocol(scenario typ.RuleScenario, incoming IncomingAPIType) protocol.APIType {
	switch scenario.Base() {
	case typ.ScenarioCodex:
		return protocol.TypeOpenAIResponses
	default:
		return incomingToTarget(incoming)
	}
}

// resolveAutoTarget handles the auto-mode target resolution shared by both
// OpenAI Chat and Responses handlers. It checks override → cache →
// scenario-preferred default. Returns the resolved target and whether
// auto-fallback should be enabled.
func (s *Server) resolveAutoTarget(
	flags typ.RuleFlags, provider *typ.Provider, model string, scenario typ.RuleScenario, incoming IncomingAPIType,
) (target protocol.APIType, autoFallback bool) {
	if ov := ParseEndpointOverride(flags.OpenAIEndpointOverride); ov == OverrideChat || ov == OverrideResponses {
		return overrideToTarget(ov), false
	}
	if cached, ok := s.endpointCache.Get(provider.UUID, model); ok {
		return cached, false
	}
	return scenarioPreferredProtocol(scenario, incoming), true
}

// autoDispatchFn is the callback for dispatchWithAutoFallback.
// It performs transform + dispatch for a given target protocol, using
// the provided gate. Returns the provider/model that served the final
// attempt (failover may have moved past the initially selected service);
// served is nil when dispatch never got a servable provider (e.g. the
// transform itself failed before failover could run).
type autoDispatchFn func(target protocol.APIType, gate *firstChunkGate) (served *typ.Provider, servedModel string)

// gateSucceeded reports whether the attempt behind the gate produced a
// success: either the stream committed its first chunk, or a buffered
// non-error status is waiting to flush.
func gateSucceeded(gate *firstChunkGate) bool {
	return gate.Committed() || (gate.Status() > 0 && gate.Status() < http.StatusBadRequest)
}

// dispatchWithAutoFallback wraps a dispatch attempt with protocol
// auto-detection. It tries the preferred target first; on retryable
// failure it falls back to the alternate protocol. Successful protocol
// choices are cached per provider+model, attributed to the service that
// actually served the request — under multi-service failover that may
// differ from the initially selected provider, and caching against the
// initial one would pin a protocol it never confirmed.
func (s *Server) dispatchWithAutoFallback(
	c *gin.Context,
	provider *typ.Provider,
	model string,
	preferredTarget protocol.APIType,
	dispatch autoDispatchFn,
) {
	realWriter := c.Writer
	gate := newFirstChunkGate(realWriter)
	c.Writer = gate
	defer func() {
		c.Writer = realWriter
		gate.CommitIfBuffered()
	}()

	// First attempt with preferred protocol
	served, servedModel := dispatch(preferredTarget, gate)

	if gateSucceeded(gate) {
		if served != nil {
			s.endpointCache.Set(served.UUID, servedModel, preferredTarget)
		}
		return
	}

	// Check if fallback is worthwhile
	if !isRetryableStatus(gate.Status()) {
		return
	}
	lastErr := extractLastGinError(c)
	if client.IsNonRetryableForProtocolSwitch(lastErr) {
		return
	}

	// Fallback to alternate protocol
	altTarget := alternateOpenAIProtocol(preferredTarget)
	logrus.WithContext(c.Request.Context()).Infof(
		"[auto-endpoint] %s:%s status=%d → fallback from %s to %s",
		provider.UUID, model, gate.Status(), preferredTarget, altTarget,
	)
	gate.Discard()
	clearGinErrors(c)

	served, servedModel = dispatch(altTarget, gate)

	if gateSucceeded(gate) && served != nil {
		s.endpointCache.Set(served.UUID, servedModel, altTarget)
	}
}

// autoDispatchOrFailover decides whether endpoint auto-detection applies to
// this request and dispatches accordingly. With auto-detection, provider
// failover runs nested inside protocol fallback, both sharing one gate
// (owned by dispatchWithAutoFallback). Without it, this is a plain provider
// failover using the statically resolved target (which may be "" — the
// zero value protocol.APIType — meaning attempt must resolve it itself).
//
// attempt is the per-provider-attempt callback; its target argument is the
// protocol to use for that attempt.
func (s *Server) autoDispatchOrFailover(
	c *gin.Context,
	rule *typ.Rule,
	provider *typ.Provider,
	actualModel string,
	scenarioType typ.RuleScenario,
	incoming IncomingAPIType,
	attempt func(p *typ.Provider, retryModel string, target protocol.APIType),
) {
	var target protocol.APIType
	autoFallback := false
	if s.autoEndpointEnabled() && provider.APIStyle == protocol.APIStyleOpenAI && ai.IsAutoEndpointMode(provider.OpenAIEndpointMode) {
		target, autoFallback = s.resolveAutoTarget(resolveRuleFlags(c, rule), provider, actualModel, scenarioType, incoming)
	}

	if autoFallback {
		s.dispatchWithAutoFallback(c, provider, actualModel, target,
			func(t protocol.APIType, gate *firstChunkGate) (*typ.Provider, string) {
				return s.dispatchWithPriorityFailoverGated(c, rule, provider, actualModel,
					func(p *typ.Provider, retryModel string) { attempt(p, retryModel, t) }, gate)
			})
		return
	}

	s.dispatchWithPriorityFailover(c, rule, provider, actualModel,
		func(p *typ.Provider, retryModel string) { attempt(p, retryModel, target) })
}
