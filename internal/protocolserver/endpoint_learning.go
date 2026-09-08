package protocolserver

import (
	"bytes"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ctxKeyEndpointDecision carries the endpointDecision below. One key rather
// than one per field: these values are facets of a single decision with a
// single lifetime, and the dispatch loop resets them together.
const ctxKeyEndpointDecision = "tingly_endpoint_decision"

// endpointDecision is the per-attempt contract between endpoint resolution
// (inside the attempt) and the dispatch loop that owns the retry decision.
//
// The gin context is the channel because dispatchAttempt is
// func(provider, model): widening that signature to carry an outcome would
// reach every handler closure and FailAttemptSetup. That is the deeper fix if
// this contract ever grows again.
type endpointDecision struct {
	// resolved is the endpoint the current attempt used; forced is the one
	// the loop is making it use for a learning retry.
	resolved, forced protocol.APIType
	// pinned marks a resolution the rule's openai_endpoint_override dictated.
	// setupFailed marks an attempt the gateway failed itself, before the
	// upstream saw it. Neither is a verdict on the endpoint.
	pinned, setupFailed bool
	// mode caches EffectiveEndpointMode for modeProvider. Resolution, the
	// snapshot decision and the retry check all ask for it within one attempt,
	// and deriving it parses URLs.
	mode         ai.OpenAIEndpointMode
	modeProvider string
}

// endpointDecisionFor returns the request's decision, creating it on first use.
func endpointDecisionFor(c *gin.Context) *endpointDecision {
	if c == nil {
		return &endpointDecision{}
	}
	if v, ok := c.Get(ctxKeyEndpointDecision); ok {
		if d, ok := v.(*endpointDecision); ok {
			return d
		}
	}
	d := &endpointDecision{}
	c.Set(ctxKeyEndpointDecision, d)
	return d
}

// reset clears everything scoped to one attempt. forced survives: the loop
// sets it for the retry attempt it is about to run.
func (d *endpointDecision) reset() {
	d.resolved = ""
	d.pinned = false
	d.setupFailed = false
}

// effectiveMode is EffectiveEndpointMode memoized for the attempt's provider.
func (d *endpointDecision) effectiveMode(provider *typ.Provider) ai.OpenAIEndpointMode {
	uuid := ""
	if provider != nil {
		uuid = provider.UUID
	}
	if d.modeProvider != uuid || d.mode == "" {
		d.mode = EffectiveEndpointMode(provider)
		d.modeProvider = uuid
	}
	return d.mode
}

// EffectiveEndpointMode is the provider's declared OpenAIEndpointMode, with one
// host-derived fallback: an OpenCode Zen provider carrying no declaration is
// treated as per-model.
//
// The fallback exists because the declaration cannot reach these providers.
// ProviderTemplate.OpenAIEndpointMode is only ever copied onto a Provider by
// the OAuth path (Codex); a provider created from a template through the normal
// API keeps the zero value — which is why openai-com's "both" has never taken
// effect either.
//
// This is a stopgap, and the deeper fix is named in
// .design/openai-endpoint-routing.md §10: serve the template's declaration
// lazily at request time, the way TemplateManager already serves max_tokens
// and web-search capability, which would retire the host match here and repair
// "both" at the same time.
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

// ResolveOpenAIEndpointForRequest wraps the pure resolver with the request
// state it deliberately does not read: the endpoint the dispatch loop is
// forcing for a learning retry, and what has already been learned for this
// service. It also records the decision for the loop.
//
// Every handler resolves through here so no dispatch path can silently opt out
// of learning — or of being retried.
func ResolveOpenAIEndpointForRequest(
	c *gin.Context,
	provider *typ.Provider,
	model string,
	flags typ.RuleFlags,
	incoming IncomingAPIType,
) (protocol.APIType, error) {
	d := endpointDecisionFor(c)
	if d.forced != "" {
		d.resolved = d.forced
		return d.forced, nil
	}

	// A rule that pins the endpoint owns the decision; record that so the
	// learning retry stands down instead of overruling it.
	d.pinned = ParseEndpointOverride(flags.OpenAIEndpointOverride) != OverrideAuto

	var learned protocol.APIType
	if d.effectiveMode(provider) == ai.EndpointModePerModel {
		learned = defaultEndpointMemory.Lookup(endpointMemoryKey(provider.UUID, model))
	}

	target, err := ResolveOpenAIEndpoint(provider, flags, incoming, learned)
	if err != nil {
		return "", err
	}
	d.resolved = target
	return target, nil
}

// DispatchMayRetry reports whether `attempt` can run more than once for this
// request — because the rule has a fallback tier, or because endpoint learning
// may retry the same service on the other endpoint.
//
// It states one invariant in one place for the two consumers that must agree:
// the dispatch loop (which installs the response buffer) and the handlers
// (which snapshot a pristine request). If they disagree, a second attempt
// re-transforms the request object the first one already mutated.
//
// Once an endpoint is learned there is nothing left to retry, so the cost of
// buffering and snapshotting is paid only until the answer is known — see
// forgetStaleEndpoint for how a mapping that stops working gets re-learned.
func DispatchMayRetry(rule *typ.Rule, provider *typ.Provider, model string) bool {
	if rule != nil && len(rule.GetActiveServices()) > 1 {
		return true
	}
	if EffectiveEndpointMode(provider) != ai.EndpointModePerModel {
		return false
	}
	return defaultEndpointMemory.Lookup(endpointMemoryKey(provider.UUID, model)) == ""
}

// MarkAttemptSetupFailed records that the gateway itself failed the attempt
// before reaching the upstream. Such a failure is written as a 500, which the
// mismatch matcher would otherwise read as "maybe the wrong endpoint".
func MarkAttemptSetupFailed(c *gin.Context) {
	endpointDecisionFor(c).setupFailed = true
}

// forgetStaleEndpoint drops a learned mapping after the endpoint it names
// failed with a server error. Without this, a model the upstream moves between
// formats stays broken for the rest of the TTL: the learned value routes every
// request to the dead endpoint, and DispatchMayRetry — seeing an answer
// already known — installs no buffer to retry with. Forgetting costs one
// failed request and lets the next one re-learn.
func forgetStaleEndpoint(c *gin.Context, provider *typ.Provider, model string, status int) {
	if status < 500 || endpointDecisionFor(c).effectiveMode(provider) != ai.EndpointModePerModel {
		return
	}
	serviceID := endpointMemoryKey(provider.UUID, model)
	if defaultEndpointMemory.Lookup(serviceID) == "" {
		return
	}
	defaultEndpointMemory.Forget(serviceID)
	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"stage": "endpoint_forgotten", "provider": provider.UUID, "model": model, "status": status,
	}).Infof("[endpoint] %s/%s failed on its learned endpoint; will re-learn", provider.UUID, model)
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
// EndpointModePerModel: a provider whose format does not vary by model never
// reaches this test, so an ordinary upstream 500 is never retried elsewhere.
var endpointMismatchMarkers = [][]byte{
	[]byte("not supported for format"),
	[]byte("endpoint is unavailable"),
	[]byte("unsupported endpoint"),
}

// endpointMismatchScanLimit caps how much of a failure body is scanned. The
// markers sit in the first JSON object; a relay echoing a request back would
// otherwise be lowercased in full.
const endpointMismatchScanLimit = 4096

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
	if len(body) > endpointMismatchScanLimit {
		body = body[:endpointMismatchScanLimit]
	}
	lower := bytes.ToLower(body)
	for _, marker := range endpointMismatchMarkers {
		if bytes.Contains(lower, marker) {
			return true
		}
	}
	return false
}

// retryOnAlternateEndpoint gives a failed attempt on a per-model provider one
// second chance on the other OpenAI endpoint, remembers the answer when it
// works, and returns the status the dispatch loop should judge.
//
// It is the whole of the "adaptive" behavior. Two properties keep it from
// repeating the AdaptiveProbe (PR #976) mistakes: it is reactive — it runs
// only after a real request already failed at the upstream, so there is no
// cold-start probe and no synthetic token spend — and it is gated on the
// provider's mode, so no other upstream can pay for it. The remaining guards
// are inline below.
func (ph *ProtocolHandler) retryOnAlternateEndpoint(
	c *gin.Context,
	gate *firstChunkGate,
	provider *typ.Provider,
	model string,
	status int,
	attemptNo int,
	attempt dispatchAttempt,
) int {
	d := endpointDecisionFor(c)
	if d.effectiveMode(provider) != ai.EndpointModePerModel || gate.Committed() {
		return status
	}
	if d.forced != "" {
		return status // already the retry
	}
	if d.pinned {
		return status // the rule chose this endpoint explicitly
	}
	if d.setupFailed {
		return status // our own failure, not the upstream's verdict
	}
	alternate := alternateEndpoint(d.resolved)
	if alternate == "" {
		return status // not an OpenAI-shaped attempt
	}
	if !looksLikeEndpointMismatch(status, gate.BufferedBody()) {
		return status
	}
	serviceID := endpointMemoryKey(provider.UUID, model)
	if !defaultEndpointMemory.ShouldRetry(serviceID) {
		return status
	}
	defaultEndpointMemory.MarkRetry(serviceID)

	logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
		"stage": "endpoint_learning_retry", "provider": provider.UUID, "model": model,
		"attempt": attemptNo, "status": status, "from": d.resolved, "to": alternate,
	}).Infof("[endpoint] %s/%s failed on %s, retrying on %s", provider.UUID, model, d.resolved, alternate)

	gate.Discard()
	d.forced = alternate
	defer func() { d.forced = "" }()

	attempt(provider, model)

	retried := gate.Status()
	if gate.Committed() || isSuccessStatus(retried) {
		defaultEndpointMemory.Remember(serviceID, alternate)
		logrus.WithContext(c.Request.Context()).WithFields(logrus.Fields{
			"stage": "endpoint_learned", "provider": provider.UUID, "model": model,
			"endpoint": alternate, "ttl": endpointMemoryTTL.String(),
		}).Infof("[endpoint] learned %s/%s speaks %s", provider.UUID, model, alternate)
		return retried
	}

	// A terminal status from the retry must not mask a retryable original:
	// the first failure had earned this service a fallback to a healthy
	// sibling, and the second endpoint's verdict does not take that away.
	if isRetryableStatus(retried) || !isRetryableStatus(status) {
		return retried
	}
	return status
}

// isSuccessStatus treats a buffered 2xx as success. A gate that was never
// written (status 0) is not success: the attempt produced nothing.
func isSuccessStatus(status int) bool {
	return status >= 200 && status < 300
}
