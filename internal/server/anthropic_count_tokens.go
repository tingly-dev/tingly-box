package server

import (
	"context"
	"encoding/json"
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
	logrus.Debugf("scenario: %s beta: %v", c.Query("scenario"), beta)

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Debugf("Failed to read request body: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
			},
		})
		return
	}

	var requestModel string

	// always use beta for token count
	var params anthropic.BetaMessageCountTokensParams
	if err := json.Unmarshal(bodyBytes, &params); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Message error: %s", err.Error()),
				Type:    "invalid_request_error",
			},
		})
		logrus.WithError(err).Errorf("Anthropic beta decode error")
		c.Abort()
		return
	}

	requestModel = params.Model
	if requestModel == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if this is the request requestModel name first
	rule, err := ph.determineRuleWithScenario(c, scenarioType, requestModel)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	provider, selectedService, err := ph.deps.RoutingSelector.SelectService(c, scenarioType, rule, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	useModel := selectedService.Model
	params.Model = useModel
	ph.anthropicCountTokens(c, provider, useModel, params)
}

// anthropicCountTokens unified token counting implementation
func (ph *ProtocolHandler) anthropicCountTokens(c *gin.Context, provider *typ.Provider, model string, req anthropic.BetaMessageCountTokensParams) {
	// Resolve dual endpoint: when the provider has an Anthropic-compatible
	// dual URL configured, route there natively to avoid a transform.
	provider = provider.ResolveStyle(protocol.APIStyleAnthropic)

	c.Set("provider", provider.UUID)
	c.Set("model", model)

	apiStyle := provider.APIStyle
	wrapper := ph.deps.ClientPool.GetAnthropicClient(context.Background(), provider, model)
	timeout := time.Duration(provider.Timeout) * time.Second
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	switch apiStyle {
	default:
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: fmt.Sprintf("Unsupported API style: %s %s", provider.Name, apiStyle),
				Type:    "invalid_request_error",
			},
		})
		return
	case protocol.APIStyleAnthropic:
		ph.anthropicCountTokensViaAPI(c, ctx, wrapper, req)
	case protocol.APIStyleOpenAI, protocol.APIStyleGoogle:
		ph.anthropicCountTokensViaTiktoken(c, req)
	}
}

func (ph *ProtocolHandler) anthropicCountTokensViaAPI(c *gin.Context, ctx context.Context, wrapper client.AnthropicClientInterface, req anthropic.BetaMessageCountTokensParams) {
	message, err := wrapper.BetaMessagesCountTokens(ctx, &req)
	if err != nil {
		stream.SendInvalidRequestBodyError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (ph *ProtocolHandler) anthropicCountTokensViaTiktoken(c *gin.Context, req anthropic.BetaMessageCountTokensParams) {
	count, err := token.CountBetaTokensViaTiktoken(&req)
	if err != nil {
		stream.SendInvalidRequestBodyError(c, err)
		return
	}
	c.JSON(http.StatusOK, anthropic.MessageTokensCount{
		InputTokens: int64(count),
	})
}
