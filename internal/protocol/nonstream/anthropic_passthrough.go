package nonstream

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleAnthropicV1NonStream handles Anthropic v1 non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1NonStream(hc *protocol.HandleContext, resp *anthropic.Message) (*protocol.TokenUsage, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)
	cacheTokens := int(resp.Usage.CacheReadInputTokens)

	resp.Model = anthropic.Model(hc.ResponseModel)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}

// HandleAnthropicV1BetaNonStream handles Anthropic v1 beta non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaNonStream(hc *protocol.HandleContext, resp *anthropic.BetaMessage) (*protocol.TokenUsage, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)
	cacheTokens := int(resp.Usage.CacheReadInputTokens)

	resp.Model = anthropic.Model(hc.ResponseModel)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewTokenUsageWithCache(inputTokens, outputTokens, cacheTokens), nil
}
