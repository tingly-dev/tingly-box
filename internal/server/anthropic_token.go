package server

import (
	"context"
	"fmt"
	"net/http"
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

// anthropicCountTokensV1Beta implements beta count_tokens
func (s *Server) anthropicCountTokensV1Beta(c *gin.Context, provider *typ.Provider, model string) {
	s.anthropicCountTokens(c, provider, model, anthropicCountTokensBeta)
}

// anthropicCountTokensV1 implements standard v1 count_tokens
func (s *Server) anthropicCountTokensV1(c *gin.Context, provider *typ.Provider, model string) {
	s.anthropicCountTokens(c, provider, model, anthropicCountTokensV1)
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
		count, err := token.CountBetaTokensWithTiktoken(string(req.Model), req.Messages, req.System)
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
		count, err := token.CountTokensWithTiktoken(string(req.Model), req.Messages, req.System.OfTextBlockArray)
		if err != nil {
			SendInvalidRequestBodyError(c, err)
			return
		}
		c.JSON(http.StatusOK, anthropic.MessageTokensCount{
			InputTokens: int64(count),
		})
	}
}
