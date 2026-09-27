package protocolserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/gin-gonic/gin"

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

	ph.runDecisionAttempt(c, provider, actualModel, sessionID, body, requestModel)
}

// runDecisionAttempt forwards one decision request. Single-shot by design: a
// decision is an advisory micro-model call with no retryable stream, so there
// is no failover gate and no candidate rotation.
func (ph *ProtocolHandler) runDecisionAttempt(c *gin.Context, provider *typ.Provider, model string, sessionID typ.SessionID, body []byte, requestModel string) {
	if err := requireDecisionEndpoint(provider); err != nil {
		SendErrorResponse(c, err, "decision")
		return
	}

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
			decisionWriteResponse(c, upstreamErr.StatusCode, "", upstreamErr.Body)
			return
		}
		// Transport-level failure (no upstream response at all).
		ph.failForward(c, err)
		return
	}

	ph.trackUsageWithTokenUsage(c, decisionUsageFromBody(resp.Body), nil)
	decisionWriteResponse(c, resp.StatusCode, resp.ContentType, decisionRewriteResponseModel(resp.Body, requestModel))
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
// opaque so new Jev fields stay forward-compatible.
func validateDecisionBody(body []byte) (string, error) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return "", fmt.Errorf("invalid request body: %w", err)
	}
	var model string
	if err := json.Unmarshal(req["model"], &model); err != nil || strings.TrimSpace(model) == "" {
		return "", fmt.Errorf("model is required")
	}
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(req["questions"], &questions); err != nil || len(questions) == 0 {
		return "", fmt.Errorf("at least one question is required")
	}
	return model, nil
}

// rewriteDecisionModel replaces the request model with the routed service
// model, preserving every other field.
func rewriteDecisionModel(body []byte, model string) ([]byte, error) {
	var req map[string]json.RawMessage
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, err
	}
	routed, err := json.Marshal(model)
	if err != nil {
		return nil, err
	}
	req["model"] = routed
	return json.Marshal(req)
}

// decisionRewriteResponseModel keeps the gateway-wide invariant that the
// client-visible model is the model the request asked for — the upstream
// service model is routing internals (see TestResponseCarriesRequestedModel).
// Only the top-level "model" field of a JSON-object body is touched; anything
// else passes through unchanged.
func decisionRewriteResponseModel(body []byte, requestModel string) []byte {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(body, &payload); err != nil {
		return body
	}
	if _, ok := payload["model"]; !ok {
		return body
	}
	requested, err := json.Marshal(requestModel)
	if err != nil {
		return body
	}
	payload["model"] = requested
	out, err := json.Marshal(payload)
	if err != nil {
		return body
	}
	return out
}

// decisionUsageFromBody extracts the optional usage object the decision
// upstream may report. Both OpenAI-style (prompt_tokens/completion_tokens)
// and Anthropic-style (input_tokens/output_tokens) spellings are accepted;
// absent usage means zero — decision answers are structured, and the
// micro-model may simply not report tokens.
func decisionUsageFromBody(body []byte) *protocol.TokenUsage {
	var payload struct {
		Usage *struct {
			InputTokens      *int `json:"input_tokens"`
			OutputTokens     *int `json:"output_tokens"`
			PromptTokens     *int `json:"prompt_tokens"`
			CompletionTokens *int `json:"completion_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(body, &payload); err != nil || payload.Usage == nil {
		return protocol.ZeroTokenUsage()
	}
	u := payload.Usage
	input := decisionFirstInt(u.InputTokens, u.PromptTokens)
	output := decisionFirstInt(u.OutputTokens, u.CompletionTokens)
	return protocol.NewTokenUsageWithCache(input, output, 0)
}

func decisionFirstInt(values ...*int) int {
	for _, v := range values {
		if v != nil {
			return *v
		}
	}
	return 0
}
