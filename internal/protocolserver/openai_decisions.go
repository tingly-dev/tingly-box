package protocolserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIDecisions serves OpenAI Decisions API requests
// (.design/decision-protocol.md).
//
// Decisions are a native OpenAI endpoint ({APIBase}/decisions), not a new
// provider style: the request rides the OpenAI client, and the only special
// handling is that the body is opaque JSON (the upstream schema is not
// public) of which the gateway reads and rewrites just "model". Like
// embeddings it is non-streaming, skips the chat transform chain, and is
// reachable from any scenario that declares TransportOpenAI.
func (ph *ProtocolHandler) HandleOpenAIDecisions(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !IsValidRuleScenario(scenarioType) || !typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) {
		decisionBadRequest(c, fmt.Sprintf("scenario %s does not support decisions", scenario))
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		decisionBadRequest(c, "Failed to read request body: "+err.Error())
		return
	}
	requestModel, err := decisionRequestModel(body)
	if err != nil {
		decisionBadRequest(c, err.Error())
		return
	}

	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		decisionBadRequest(c, err.Error())
		return
	}
	// Single-shot endpoints share content-free selection (no smart routing).
	provider, selectedService, err := ph.selectServiceForEmbeddings(c, scenarioType, rule)
	if err != nil {
		decisionBadRequest(c, err.Error())
		return
	}

	// Prefer the provider's OpenAI-compatible dual URL when configured.
	provider = provider.ResolveStyle(protocol.APIStyleOpenAI)
	if provider.APIStyle != protocol.APIStyleOpenAI {
		decisionBadRequest(c, fmt.Sprintf("unsupported provider api style for decisions: %s", provider.APIStyle))
		return
	}

	actualModel := selectedService.Model
	upstreamBody, err := sjson.SetBytes(body, "model", actualModel)
	if err != nil {
		decisionBadRequest(c, "Invalid request body: "+err.Error())
		return
	}

	c.Request = c.Request.WithContext(typ.WithSessionID(c.Request.Context(), resolveSessionID(c, nil)))
	SetTrackingContext(c, rule, provider, actualModel, requestModel, false)

	wrapper := ph.deps.ClientPool.GetOpenAIClient(c.Request.Context(), provider, actualModel)
	fc := forwarding.NewForwardContext(c.Request.Context(), provider)

	resp, cancel, err := forwarding.ForwardOpenAIDecisions(fc, wrapper, upstreamBody)
	if cancel != nil {
		defer cancel()
	}
	if err != nil {
		ph.failForward(c, err)
		return
	}

	ph.trackUsageWithTokenUsage(c, decisionUsage(resp), nil)
	c.Data(http.StatusOK, "application/json", decisionEchoModel(resp, requestModel))
}

func decisionBadRequest(c *gin.Context, msg string) {
	c.JSON(http.StatusBadRequest, ErrorResponse{
		Error: ErrorDetail{Message: msg, Type: "invalid_request_error"},
	})
}

// decisionRequestModel returns the requested model — the only field the
// gateway needs (for routing). Everything else stays opaque.
func decisionRequestModel(body []byte) (string, error) {
	if !gjson.ValidBytes(body) {
		return "", fmt.Errorf("invalid request body: not valid JSON")
	}
	model := gjson.GetBytes(body, "model")
	if model.Type != gjson.String || strings.TrimSpace(model.String()) == "" {
		return "", fmt.Errorf("model is required")
	}
	return model.String(), nil
}

// decisionUsage lifts the optional OpenAI-style usage object out of a decision
// response (prompt/completion or input/output spellings); zero when absent.
func decisionUsage(body []byte) *protocol.TokenUsage {
	u := gjson.GetBytes(body, "usage")
	in := u.Get("prompt_tokens").Int() + u.Get("input_tokens").Int()
	out := u.Get("completion_tokens").Int() + u.Get("output_tokens").Int()
	return protocol.NewTokenUsageWithCache(int(in), int(out), 0)
}

// decisionEchoModel echoes the caller's model instead of the routed upstream
// model, touching only a top-level "model" that differs; other bytes pass
// through untouched.
func decisionEchoModel(body []byte, requestModel string) []byte {
	m := gjson.GetBytes(body, "model")
	if !m.Exists() || m.String() == requestModel {
		return body
	}
	if out, err := sjson.SetBytes(body, "model", requestModel); err == nil {
		return out
	}
	return body
}
