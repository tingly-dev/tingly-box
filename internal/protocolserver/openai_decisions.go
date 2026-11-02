package protocolserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/decision"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HandleOpenAIDecisions serves OpenAI Decisions API requests
// (.design/decision-protocol.md).
//
// Decisions are a native OpenAI endpoint ({APIBase}/decisions), not a new
// provider style: the request rides the OpenAI client, and the only special
// handling identifies and, where supported, converts decision dialects. Like
// embeddings it is non-streaming, skips the chat transform chain, and is
// reachable from any scenario that declares TransportOpenAI or
// TransportDecisions (the dedicated `decisions` scenario).
func (ph *ProtocolHandler) HandleOpenAIDecisions(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	if !IsValidRuleScenario(scenarioType) || (!typ.ScenarioSupportsTransport(scenarioType, typ.TransportOpenAI) &&
		!typ.ScenarioSupportsTransport(scenarioType, typ.TransportDecisions)) {
		decisionBadRequest(c, fmt.Sprintf("scenario %s does not support decisions", scenario))
		return
	}

	body, err := c.GetRawData()
	if err != nil {
		decisionBadRequest(c, "Failed to read request body: "+err.Error())
		return
	}
	dialect, err := decision.Request(body)
	if err != nil {
		decisionBadRequest(c, err.Error())
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
	target := decision.ResolveTarget(provider.APIBase).Dialect
	if provider.AuthType == typ.AuthTypeVirtual {
		target = decision.SystemOne
	}
	upstreamBody, conversion, err := decision.ConvertRequest(body, dialect, target)
	if err != nil {
		decisionBadRequest(c, "decision conversion: "+err.Error())
		return
	}
	upstreamBody, err = sjson.SetBytes(upstreamBody, "model", actualModel)
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
	if conversion != nil {
		resp, err = conversion.ConvertResponse(resp)
		if err != nil {
			writeErrorResponse(c, http.StatusBadGateway, ErrorResponse{Error: ErrorDetail{
				Message: "invalid upstream decision response: " + err.Error(), Type: "api_error",
			}})
			return
		}
	}
	responseDialect := decision.Response(resp)
	if dialect != decision.Unknown && responseDialect != decision.Unknown && dialect != responseDialect {
		writeErrorResponse(c, http.StatusBadGateway, ErrorResponse{Error: ErrorDetail{
			Message: "upstream returned an incompatible decision response format; protocol conversion is not configured",
			Type:    "api_error",
		}})
		return
	}
	c.Data(http.StatusOK, "application/json", decisionEchoModel(resp, requestModel))
}

func decisionBadRequest(c *gin.Context, msg string) {
	writeErrorResponse(c, http.StatusBadRequest, ErrorResponse{
		Error: ErrorDetail{Message: msg, Type: "invalid_request_error"},
	})
}

// decisionRequestModel returns the requested model for rule routing.
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
	read := func(keys ...string) int64 {
		for _, key := range keys {
			if value := u.Get(key); value.Type == gjson.Number {
				return value.Int()
			}
		}
		return 0
	}
	in := read("input_tokens", "inputTokens", "prompt_tokens")
	out := read("output_tokens", "outputTokens", "completion_tokens")
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
