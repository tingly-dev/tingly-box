package protocolserver

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Request-scoped plumbing between the endpoint resolution done inside an
// attempt and the dispatch loop that owns the retry decision.
//
//   - ctxKeyResolvedEndpoint: what the attempt actually used, so the loop can
//     compute the alternative without re-deriving it.
//   - ctxKeyForcedEndpoint: the loop's instruction for the retry attempt.
//
// The gin context is the same channel the rest of the per-request routing
// state travels on (rule flags, tracking); a parameter would have to thread
// through every transform signature to reach the same place.
const (
	ctxKeyResolvedEndpoint = "tingly_resolved_openai_endpoint"
	ctxKeyForcedEndpoint   = "tingly_forced_openai_endpoint"
	// ctxKeyEndpointPinned marks a resolution that came from the rule's
	// openai_endpoint_override. A pinned endpoint is the caller's explicit
	// choice and is never second-guessed by learning.
	ctxKeyEndpointPinned = "tingly_openai_endpoint_pinned"
	// ctxKeySetupFailed marks an attempt that failed inside the gateway
	// (target resolution, transform) rather than at the upstream.
	ctxKeySetupFailed = "tingly_attempt_setup_failed"
)

// EffectiveEndpointMode is the provider's declared OpenAIEndpointMode, with one
// host-derived fallback: an OpenCode Zen provider carrying no declaration is
// treated as per-model.
//
// The fallback exists because the declaration cannot reach these providers.
// ProviderTemplate.OpenAIEndpointMode is only ever copied onto a Provider by
// the OAuth path (Codex); a provider created from a template through the normal
// API keeps the zero value — which is why openai-com's "both" has never taken
// effect either. Rather than add a startup migration that still leaves a
// provider added at runtime broken until the next restart, derive the fact
// where it is used, from the same host match the OpenCode client already
// relies on.
//
// Pure: reads provider fields only.
func EffectiveEndpointMode(provider *typ.Provider) ai.OpenAIEndpointMode {
	if provider == nil {
		return ai.EndpointModeUnknown
	}
	if provider.OpenAIEndpointMode != ai.EndpointModeUnknown {
		return provider.OpenAIEndpointMode
	}
	for _, base := range []string{provider.APIBase, provider.APIBaseOpenAI, provider.APIBaseAnthropic} {
		if base != "" && client.IsOpenCodeZen(base) {
			return ai.EndpointModePerModel
		}
	}
	return ai.EndpointModeUnknown
}

// ResolveOpenAIEndpointForRequest wraps the pure resolver with the two pieces
// of request state it deliberately does not read: the endpoint the dispatch
// loop is forcing for a learning retry, and what has already been learned for
// this provider+model. It also records the decision for the loop.
//
// Every handler resolves through here so no dispatch path can silently opt
// out of learning — or of being retried.
func ResolveOpenAIEndpointForRequest(
	c *gin.Context,
	provider *typ.Provider,
	model string,
	flags typ.RuleFlags,
	incoming IncomingAPIType,
) (protocol.APIType, error) {
	if forced, ok := forcedEndpoint(c); ok {
		c.Set(ctxKeyResolvedEndpoint, forced)
		return forced, nil
	}

	// A rule that pins the endpoint owns the decision; record that so the
	// learning retry stands down instead of overruling it.
	c.Set(ctxKeyEndpointPinned, ParseEndpointOverride(flags.OpenAIEndpointOverride) != OverrideAuto)

	var learned protocol.APIType
	if endpointLearningEnabled(provider) {
		learned = defaultEndpointMemory.Lookup(provider.UUID, model)
	}

	target, err := ResolveOpenAIEndpoint(provider, flags, incoming, learned)
	if err != nil {
		return "", err
	}
	c.Set(ctxKeyResolvedEndpoint, target)
	return target, nil
}

// endpointLearningEnabled reports whether this provider opted into the
// learning fallback by declaring EndpointModePerModel. Everything downstream
// — installing the buffer for a single-service rule, spending an extra
// round-trip, writing to the memory — is gated on this one declaration, which
// is what keeps the blast radius at "providers whose upstream is known to be
// per-model" instead of "every provider", the flaw that sank the old
// AdaptiveProbe.
func endpointLearningEnabled(provider *typ.Provider) bool {
	return EffectiveEndpointMode(provider) == ai.EndpointModePerModel
}

// ResetEndpointDecision clears the per-attempt decision state. The dispatch
// loop calls it before every attempt: a later attempt may land on a provider
// of another API style that never resolves an OpenAI endpoint at all, and a
// stale value there would let the retry fire on someone else's failure.
func ResetEndpointDecision(c *gin.Context) {
	if c == nil {
		return
	}
	c.Set(ctxKeyResolvedEndpoint, protocol.APIType(""))
	c.Set(ctxKeyEndpointPinned, false)
	c.Set(ctxKeySetupFailed, false)
}

// MarkAttemptSetupFailed records that the gateway itself failed the attempt
// before reaching the upstream. Such a failure is written as a 500, which the
// mismatch matcher would otherwise read as "maybe the wrong endpoint" and pay
// a pointless round-trip for.
func MarkAttemptSetupFailed(c *gin.Context) {
	if c != nil {
		c.Set(ctxKeySetupFailed, true)
	}
}

func boolFromContext(c *gin.Context, key string) bool {
	if c == nil {
		return false
	}
	v, ok := c.Get(key)
	if !ok {
		return false
	}
	b, _ := v.(bool)
	return b
}

func forcedEndpoint(c *gin.Context) (protocol.APIType, bool) {
	if c == nil {
		return "", false
	}
	v, ok := c.Get(ctxKeyForcedEndpoint)
	if !ok {
		return "", false
	}
	ep, ok := v.(protocol.APIType)
	return ep, ok && ep != ""
}

func resolvedEndpoint(c *gin.Context) protocol.APIType {
	if c == nil {
		return ""
	}
	if v, ok := c.Get(ctxKeyResolvedEndpoint); ok {
		if ep, ok := v.(protocol.APIType); ok {
			return ep
		}
	}
	return ""
}

// endpointMismatchMarkers are the upstream rejections that mean "this model
// does not speak this endpoint". OpenCode Zen answers the same condition three
// different ways depending on which backend vendor is behind the model, so
// text matching is unavoidable — measured 2026-09-08 on /zen/go:
//
//	grok-4.6     → 401 "Model grok-4.6 is not supported for format oa-compat"
//	grok-4.5     → 500 "Upstream request failed: Endpoint is unavailable."
//	gpt-5.6-luna → 500 "Internal server error"
//
// The last one carries no marker at all, which is why looksLikeEndpointMismatch
// also accepts a bare 5xx. That is safe only because the whole path is gated on
// EndpointModePerModel: a provider that has not declared per-model variance
// never reaches this test, so an ordinary upstream 500 is never retried on a
// different endpoint.
var endpointMismatchMarkers = []string{
	"not supported for format",
	"endpoint is unavailable",
	"unsupported endpoint",
}

// looksLikeEndpointMismatch reports whether a buffered attempt failure is
// plausibly "wrong endpoint for this model".
//
// 4xx needs an explicit marker: a 401 without one is a credential problem and
// a 400 without one is a malformed request, and neither gets better on the
// other endpoint. 5xx is accepted bare, per the luna case above.
func looksLikeEndpointMismatch(status int, body []byte) bool {
	if status >= 500 {
		return true
	}
	if status != 400 && status != 401 && status != 404 {
		return false
	}
	lower := strings.ToLower(string(body))
	for _, marker := range endpointMismatchMarkers {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// retryOnAlternateEndpoint gives a failed attempt on a per-model provider one
// second chance on the other OpenAI endpoint, and remembers the answer when
// it works. Returns whether a retry actually ran (the caller then re-reads
// the gate).
//
// It is the whole of the "adaptive" behavior, and every guard here is one of
// the ways the old AdaptiveProbe went wrong:
//
//   - Reactive, never proactive: this runs after a real request already
//     failed at the upstream. A failure the gateway produced itself, or an
//     endpoint the rule pinned, is not second-guessed.
//   - Gated on the provider's declared mode, so no other upstream can ever
//     pay for it.
//   - Pre-commit only: a stream that has put bytes on the wire cannot be
//     retried, and is not.
//   - One retry, then normal failover takes over.
//   - Cooldown on failure, so a sick upstream cannot turn into a hot loop:
//     one extra request per (provider, model) per minute, at worst. Success
//     is not throttled at all — it is remembered, and a remembered mapping
//     produces no failure to retry.
//   - Only the success is remembered.
func (ph *ProtocolHandler) retryOnAlternateEndpoint(
	c *gin.Context,
	gate *firstChunkGate,
	provider *typ.Provider,
	model string,
	status int,
	attemptNo int,
	attempt dispatchAttempt,
) bool {
	if !endpointLearningEnabled(provider) || gate.Committed() {
		return false
	}
	if _, forced := forcedEndpoint(c); forced {
		return false // already the retry
	}
	if boolFromContext(c, ctxKeyEndpointPinned) {
		return false // the rule chose this endpoint explicitly
	}
	if boolFromContext(c, ctxKeySetupFailed) {
		return false // our own failure, not the upstream's verdict
	}
	used := resolvedEndpoint(c)
	alternate := alternateEndpoint(used)
	if alternate == "" {
		return false // not an OpenAI-shaped attempt
	}
	if !looksLikeEndpointMismatch(status, gate.BufferedBody()) {
		return false
	}
	if !defaultEndpointMemory.ShouldRetry(provider.UUID, model) {
		return false
	}
	defaultEndpointMemory.MarkRetry(provider.UUID, model)

	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"stage":    "endpoint_learning_retry",
		"provider": provider.UUID,
		"model":    model,
		"attempt":  attemptNo,
		"status":   status,
		"from":     used,
		"to":       alternate,
	}).Infof("[endpoint] %s/%s failed on %s, retrying on %s", provider.UUID, model, used, alternate)

	gate.Discard()
	c.Set(ctxKeyForcedEndpoint, alternate)
	defer c.Set(ctxKeyForcedEndpoint, protocol.APIType(""))

	attempt(provider, model)

	if gate.Committed() || isSuccessStatus(gate.Status()) {
		defaultEndpointMemory.Remember(provider.UUID, model, alternate)
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"stage":    "endpoint_learned",
			"provider": provider.UUID,
			"model":    model,
			"endpoint": alternate,
			"ttl":      endpointMemoryTTL.String(),
		}).Infof("[endpoint] learned %s/%s speaks %s", provider.UUID, model, alternate)
	}
	return true
}

// isSuccessStatus treats a buffered 2xx as success. A gate that was never
// written (status 0) is not success: the attempt produced nothing.
func isSuccessStatus(status int) bool {
	return status >= 200 && status < 300
}
