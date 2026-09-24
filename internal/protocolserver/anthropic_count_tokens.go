package protocolserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// AnthropicCountTokens handles Anthropic v1 count_tokens endpoint
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (ph *ProtocolHandler) AnthropicCountTokens(c *gin.Context) {
	scenario := c.Param("scenario")
	scenarioType := typ.RuleScenario(scenario)

	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.WithContext(c.Request.Context()).Debugf("scenario: %s beta: %v", scenario, beta)

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		rejectRequestWithStatus(c, http.StatusInternalServerError, "inbound", "", "", err)
		return
	}

	var requestModel string

	// always use beta for token count
	var params anthropic.BetaMessageCountTokensParams
	if err := json.Unmarshal(bodyBytes, &params); err != nil {
		rejectRequest(c, "inbound", "", fmt.Errorf("Message error: %w", err))
		return
	}

	requestModel = params.Model
	if requestModel == "" {
		rejectRequest(c, "inbound", "", errors.New("Model is required"))
		return
	}

	// Check if this is the request requestModel name first
	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		rejectRequest(c, "routing", requestModel, err)
		return
	}

	provider, selectedService, err := ph.selectService(c, scenarioType, rule, nil)
	if err != nil {
		rejectRequest(c, "routing", requestModel, err)
		return
	}

	// count_tokens never goes through SetTrackingContext (it records no
	// usage), so label the access log here or the Requests view shows the
	// row without scenario/model.
	c.Set(ContextKeyScenario, ExtractScenarioFromPath(c.Request.URL.Path))
	c.Set(ContextKeyRequestModel, requestModel)

	useModel := selectedService.Model
	params.Model = useModel
	ph.anthropicCountTokens(c, provider, useModel, params)
}

// anthropicCountTokens unified token counting implementation
func (ph *ProtocolHandler) anthropicCountTokens(c *gin.Context, provider *typ.Provider, model string, req anthropic.BetaMessageCountTokensParams) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleAnthropic)
	c.Set(ContextKeyProvider, provider)
	c.Set(ContextKeyModel, model)

	apiStyle := provider.APIStyle
	timeout := time.Duration(provider.Timeout) * time.Second
	// Derived from the request context, not Background: it carries the
	// request_id the upstream log line correlates by, and a client hanging
	// up cancels the upstream call.
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()

	switch apiStyle {
	default:
		rejectRequest(c, "routing", model, fmt.Errorf("Unsupported API style: %s %s", provider.Name, apiStyle))
		return
	case protocol.APIStyleAnthropic:
		// Backends without a count_tokens endpoint (Bedrock) get the local
		// estimate, same as the openai/google styles.
		if !client.SupportsAnthropicCountTokens(provider) {
			ph.anthropicCountTokensViaTiktoken(c, req)
			return
		}
		wrapper := ph.deps.ClientPool.GetAnthropicClient(c.Request.Context(), provider, model)
		if wrapper == nil {
			// Client construction failed (e.g. a malformed stored credential);
			// fall back to local estimation rather than panicking.
			ph.anthropicCountTokensViaTiktoken(c, req)
			return
		}
		ph.anthropicCountTokensViaAPI(c, ctx, wrapper, req)
	case protocol.APIStyleOpenAI, protocol.APIStyleGoogle:
		ph.anthropicCountTokensViaTiktoken(c, req)
	}
}

func (ph *ProtocolHandler) anthropicCountTokensViaAPI(c *gin.Context, ctx context.Context, wrapper client.AnthropicClientInterface, req anthropic.BetaMessageCountTokensParams) {
	message, err := wrapper.BetaMessagesCountTokens(ctx, &req)
	if err != nil {
		// An upstream failure: classified status + redacted message like
		// every other forward, and logged on the request's timeline.
		stream.SendForwardingError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (ph *ProtocolHandler) anthropicCountTokensViaTiktoken(c *gin.Context, req anthropic.BetaMessageCountTokensParams) {
	count, err := token.CountBetaTokensViaTiktoken(&req)
	if err != nil {
		rejectRequest(c, "transform", req.Model, fmt.Errorf("Invalid request body: %w", err))
		return
	}
	c.JSON(http.StatusOK, anthropic.MessageTokensCount{
		InputTokens: int64(count),
	})
}
