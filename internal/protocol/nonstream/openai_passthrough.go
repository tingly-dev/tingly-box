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
func HandleOpenAIChatNonStream(hc *protocol.HandleContext, resp *openai.ChatCompletion) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.PromptTokens)
	outputTokens := int(resp.Usage.CompletionTokens)

	// Convert response to JSON map for modification
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroUsageStat(), err
	}

	var responseMap map[string]interface{}
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		hc.SendError(err, "api_error", "unmarshal_failed")
		return protocol.ZeroUsageStat(), err
	}

	// Update response model
	responseMap["model"] = hc.ResponseModel

	hc.GinContext.JSON(http.StatusOK, responseMap)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesNonStream(hc *protocol.HandleContext, resp *responses.Response) (protocol.UsageStat, error) {
	inputTokens := int(resp.Usage.InputTokens)
	outputTokens := int(resp.Usage.OutputTokens)

	hc.GinContext.JSON(http.StatusOK, resp)
	return protocol.NewUsageStat(inputTokens, outputTokens), nil
}
