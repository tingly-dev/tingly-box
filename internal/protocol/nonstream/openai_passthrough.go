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
	inputTokens := int(resp.Usage.PromptTokens)
	outputTokens := int(resp.Usage.CompletionTokens)
	cacheTokens := int(resp.Usage.PromptTokensDetails.CachedTokens)
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

	hc.RunNonStreamResponseHooks(responseMap)
	hc.GinContext.JSON(http.StatusOK, responseMap)
	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)
	cacheTokens := int(resp.Usage.InputTokensDetails.CachedTokens)
	reasoningTokens := int(resp.Usage.OutputTokensDetails.ReasoningTokens)

	hc.RunNonStreamResponseHooks(resp)
	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewTokenUsageFull(inputTokens, outputTokens, cacheTokens, reasoningTokens), nil
}
