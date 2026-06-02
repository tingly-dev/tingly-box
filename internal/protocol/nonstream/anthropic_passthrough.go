package nonstream

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// HandleAnthropicV1NonStream handles Anthropic v1 non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1NonStream(hc *protocol.HandleContext, resp *anthropic.Message) (*protocol.TokenUsage, error) {
	resp.Model = anthropic.Model(hc.ResponseModel)
	hc.GinContext.JSON(http.StatusOK, resp)
	return usage.FromAnthropicMessage(resp.Usage), nil
}

// HandleAnthropicV1BetaNonStream handles Anthropic v1 beta non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1BetaNonStream(hc *protocol.HandleContext, resp *anthropic.BetaMessage) (*protocol.TokenUsage, error) {
	resp.Model = anthropic.Model(hc.ResponseModel)
	hc.GinContext.JSON(http.StatusOK, resp)
	return usage.FromAnthropicBetaMessage(resp.Usage), nil
}
