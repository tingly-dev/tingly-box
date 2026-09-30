package protocolserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const maxDecisionRequestBytes = 8 << 20

// HandleDecision serves the native structured-decision protocol
// (POST /tingly/:scenario/v1/decisions). Decision dispatches through the
// dedicated DecisionClient — a special endpoint fork that OpenAI/Anthropic
// upstreams do not natively have — so any provider that exposes a decision
// endpoint (Jev-native or a chat provider with a decision fork URL) can serve
// it, regardless of its chat style. Like the other structured non-chat
// surfaces (embeddings, count_tokens) it is single-shot: routing pipeline and
// usage tracking apply, but there is deliberately no failover. See
// .design/decision-protocol.md.
func (ph *ProtocolHandler) HandleDecision(c *gin.Context) {
	scenario := typ.RuleScenario(c.Param("scenario"))
	if !IsValidRuleScenario(scenario) || !typ.ScenarioSupportsTransport(scenario, typ.TransportDecision) {
		SendErrorResponse(c, fmt.Errorf("scenario %q does not support the decisions transport", scenario), "decision")
		return
	}

	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxDecisionRequestBytes+1))
	if err != nil {
		SendErrorResponse(c, fmt.Errorf("failed to read request body: %w", err), "decision")
		return
	}
	if len(body) > maxDecisionRequestBytes {
		SendErrorResponse(c, fmt.Errorf("request body exceeds %d MiB", maxDecisionRequestBytes>>20), "decision")
		return
	}
	requestModel, err := validateDecisionBody(body)
	if err != nil {
		SendErrorResponse(c, err, "decision")
		return
	}

	rule, err := ph.determineRuleWithScenario(c, scenario, requestModel)
	if err != nil {
		SendErrorResponse(c, err, "decision")
		return
	}
	provider, service, err := ph.selectService(c, scenario, rule, nil)
	if err != nil {
		SendErrorResponse(c, err, "decision")
		return
	}
	if err := requireDecisionEndpoint(provider); err != nil {
		SendErrorResponse(c, err, "decision")
		return
	}

	actualModel := service.Model
	SetTrackingContext(c, rule, provider, actualModel, requestModel, false)

	// Session resolution feeds the affinity/LB stages; the session rides the
	// request context like the chat handlers do (the decision client reads it
	// from there when it acquires its transport).
	sessionID := resolveSessionID(c, nil)
	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), sessionID))

	ph.runDecisionAttempt(c, provider, actualModel, body, requestModel)
}

// runDecisionAttempt forwards one decision request. Single-shot by design: a
// decision is an advisory micro-model call with no retryable stream, so there
// is no failover gate and no candidate rotation.
func (ph *ProtocolHandler) runDecisionAttempt(c *gin.Context, provider *typ.Provider, model string, body []byte, requestModel string) {
	rewritten, err := rewriteDecisionModel(body, model)
	if err != nil {
		SendErrorResponse(c, fmt.Errorf("failed to encode upstream decision request: %w", err), "decision")
		return
	}

	wrapper := ph.deps.ClientPool.GetDecisionClient(c.Request.Context(), provider, model)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)
	resp, cancel, err := forwarding.ForwardDecision(fc, wrapper, rewritten)
	defer cancel()
	if err != nil {
		var upstreamErr *forwarding.DecisionUpstreamError
		if errors.As(err, &upstreamErr) {
			// The upstream answered with an error status: propagate its status
			// and body verbatim, so the caller sees the provider's own error
			// shape and the real status code reaches the access log.
			ph.trackUsageWithTokenUsage(c, protocol.ZeroTokenUsage(), upstreamErr)
			decisionWriteResponse(c, upstreamErr.Result.StatusCode, upstreamErr.Result.ContentType, upstreamErr.Result.Body)
			return
		}
		// Transport-level failure (no upstream response at all).
		ph.failForward(c, err)
		return
	}

	respModel := decisionRewriteResponseModel(resp.Body, requestModel)
	ph.trackUsageWithTokenUsage(c, decisionUsageFromBody(resp.Body), nil)
	decisionWriteResponse(c, resp.StatusCode, resp.ContentType, respModel)
}

func decisionWriteResponse(c *gin.Context, status int, contentType string, body []byte) {
	if contentType != "" {
		c.Header("Content-Type", contentType)
	}
	c.Status(status)
	_, _ = c.Writer.Write(body)
}

// requireDecisionEndpoint enforces that a decision request is only served by
// a provider that exposes a decision endpoint — its own api_style (Jev
// native) or a configured decision fork URL. Rule binding already refuses the
// mismatch (typ.ProviderSupportsScenario); this is the request-time defense
// in depth.
func requireDecisionEndpoint(provider *typ.Provider) error {
	if !provider.HasDecisionEndpoint() {
		return fmt.Errorf("provider %q exposes no decision endpoint (set its decision fork URL, or use a Jev-native provider) — the OpenAI and Anthropic chat endpoints do not speak the decision protocol", provider.Name)
	}
	return nil
}

// validateDecisionBody checks the two fields the gateway itself needs — model
// (for routing) and questions (non-empty map) — and leaves everything else
// opaque so new Jev fields stay forward-compatible. gjson keeps this a single
// indexed pass; the body is not copied into an intermediate map.
func validateDecisionBody(body []byte) (string, error) {
	if !gjson.ValidBytes(body) {
		return "", fmt.Errorf("invalid request body: not valid JSON")
	}
	modelResult := gjson.GetBytes(body, "model")
	if modelResult.Type != gjson.String || strings.TrimSpace(modelResult.String()) == "" {
		return "", fmt.Errorf("model is required")
	}
	questions := gjson.GetBytes(body, "questions")
	if !questions.IsObject() || len(questions.Map()) == 0 {
		return "", fmt.Errorf("at least one question is required")
	}
	return modelResult.String(), nil
}

// rewriteDecisionModel replaces the request model with the routed service
// model. sjson edits in place so every other byte of the upstream request —
// key order included — survives untouched: the decision protocol is
// passthrough, not a re-encoding boundary.
func rewriteDecisionModel(body []byte, model string) ([]byte, error) {
	return sjson.SetBytes(body, "model", model)
}

// decisionRewriteResponseModel keeps the gateway-wide invariant that the
// client-visible model is the model the request asked for — the upstream
// service model is routing internals (see TestResponseCarriesRequestedModel).
// Only the top-level "model" field of a JSON-object body is touched; a
// compliant upstream echo (or a body without a model) passes through
// byte-identically.
func decisionRewriteResponseModel(body []byte, requestModel string) []byte {
	model := gjson.GetBytes(body, "model")
	if !model.Exists() || model.String() == requestModel {
		return body
	}
	if out, err := sjson.SetBytes(body, "model", requestModel); err == nil {
		return out
	}
	return body
}

// decisionUsageFromBody extracts the optional usage object the decision
// upstream may report (ai.DecisionUsage owns the spelling table); absent
// usage means zero.
func decisionUsageFromBody(body []byte) *protocol.TokenUsage {
	usage := gjson.GetBytes(body, "usage")
	if !usage.IsObject() {
		return protocol.ZeroTokenUsage()
	}
	var u ai.DecisionUsage
	if err := json.Unmarshal([]byte(usage.Raw), &u); err != nil {
		return protocol.ZeroTokenUsage()
	}
	return protocol.NewTokenUsageWithCache(u.Input(), u.Output(), 0)
}
