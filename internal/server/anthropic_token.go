package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/token"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type anthropicCountTokensVersion int

const (
	anthropicCountTokensV1 anthropicCountTokensVersion = iota
	anthropicCountTokensBeta
)

// AnthropicCountTokens handles Anthropic v1 count_tokens endpoint
// This is the entry point that delegates to the appropriate implementation (v1 or beta)
func (s *Server) AnthropicCountTokens(c *gin.Context) {
	// Check if beta parameter is set to true
	beta := c.Query("beta") == "true"
	logrus.Debugf("scenario: %s beta: %v", c.Query("scenario"), beta)

	// Read the raw request body first for debugging purposes
	bodyBytes, err := c.GetRawData()
	if err != nil {
		logrus.Debugf("Failed to read request body: %v", err)
	} else {
		// Store the body back for parsing
		c.Request.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	}

	// Parse the request to check if streaming is requested
	var rawReq map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &rawReq); err != nil {
		logrus.Debugf("Invalid JSON in request body: %v", err)
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Invalid JSON: " + err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Check if streaming is requested
	isStreaming := false
	if stream, ok := rawReq["stream"].(bool); ok {
		isStreaming = stream
	}
	logrus.Debugf("Stream requested for AnthropicMessages: %v", isStreaming)

	// Get model from request
	model := rawReq["model"].(string)
	if model == "" {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: "Model is required",
				Type:    "invalid_request_error",
			},
		})
		return
	}

	// Determine provider and model based on request
	// Check if this is the request model name first
	rule, err := s.determineRule(model)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}
	provider, service, err := s.DetermineProviderAndModel(rule)
	if err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error: ErrorDetail{
				Message: err.Error(),
				Type:    "invalid_request_error",
			},
		})
		return
	}

	useModel := service.Model
	// Delegate to the appropriate implementation based on beta parameter
	if beta {
		s.anthropicCountTokens(c, provider, useModel, anthropicCountTokensBeta)
	} else {
		s.anthropicCountTokens(c, provider, useModel, anthropicCountTokensV1)
	}
}

// anthropicCountTokens unified token counting implementation
func (s *Server) anthropicCountTokens(c *gin.Context, provider *typ.Provider, model string, version anthropicCountTokensVersion) {
	c.Set("provider", provider.UUID)
	c.Set("model", model)

	apiStyle := provider.APIStyle
	wrapper := s.clientPool.GetAnthropicClient(provider, model)
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
		s.anthropicCountTokensViaAPI(c, ctx, wrapper, model, version)
	case protocol.APIStyleOpenAI:
		s.anthropicCountTokensViaTiktoken(c, version)
	}
}

func (s *Server) anthropicCountTokensViaAPI(c *gin.Context, ctx context.Context, wrapper interface{}, model string, version anthropicCountTokensVersion) {
	switch version {
	case anthropicCountTokensBeta:
		var req anthropic.BetaMessageCountTokensParams
		if err := c.ShouldBindJSON(&req); err != nil {
			logrus.Debugf("Invalid JSON request received: %v", err)
			SendInvalidRequestBodyError(c, err)
			return
		}
		req.Model = anthropic.Model(model)
		message, err := wrapper.(*client.AnthropicClient).BetaMessagesCountTokens(ctx, req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, message)
	case anthropicCountTokensV1:
		var req anthropic.MessageCountTokensParams
		if err := c.ShouldBindJSON(&req); err != nil {
			logrus.Debugf("Invalid JSON request received: %v", err)
			SendInvalidRequestBodyError(c, err)
			return
		}
		req.Model = anthropic.Model(model)
		message, err := wrapper.(*client.AnthropicClient).MessagesCountTokens(ctx, req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, message)
	}
}

func (s *Server) anthropicCountTokensViaTiktoken(c *gin.Context, version anthropicCountTokensVersion) {
	switch version {
	case anthropicCountTokensBeta:
		var req anthropic.BetaMessageCountTokensParams
		if err := c.ShouldBindJSON(&req); err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		count, err := token.CountBetaTokensViaTiktoken(&req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	case anthropicCountTokensV1:
		var req anthropic.MessageCountTokensParams
		if err := c.ShouldBindJSON(&req); err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		count, err := token.CountTokensViaTiktoken(&req)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}
