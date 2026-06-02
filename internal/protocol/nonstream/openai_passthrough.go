package nonstream

import (
	"encoding/json"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// HandleOpenAIChatNonStream handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatNonStream(hc *protocol.HandleContext, resp *openai.ChatCompletion) (*protocol.TokenUsage, error) {
	cacheTokens := int(resp.Usage.PromptTokensDetails.CachedTokens)
	inputTokens := int(resp.Usage.PromptTokens) - cacheTokens
	outputTokens := int(resp.Usage.CompletionTokens)
	reasoningTokens := int(resp.Usage.CompletionTokensDetails.ReasoningTokens)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		hc.SendError(err, "api_error", "unmarshal_failed")
		return protocol.ZeroTokenUsage(), err
	}

	// Update response model
	responseMap["model"] = hc.ResponseModel

	hc.GinContext.JSON(http.StatusOK, responseMap)
	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	cacheTokens := int(resp.Usage.InputTokensDetails.CachedTokens)
	inputTokens := int(resp.Usage.InputTokens) - cacheTokens
	outputTokens := int(resp.Usage.OutputTokens)
	reasoningTokens := int(resp.Usage.OutputTokensDetails.ReasoningTokens)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}
