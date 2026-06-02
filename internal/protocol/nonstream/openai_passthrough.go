package nonstream

import (
	"encoding/json"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// HandleOpenAIResponsesPassthroughNonStream handles Responses API passthrough (non-streaming),
// overriding the model field in the response when responseModel differs from the request model.
// Corresponds to stream.HandleOpenAIResponsesStream.
func HandleOpenAIResponsesPassthroughNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	var responseMap map[string]any
	if err := json.Unmarshal(responseJSON, &responseMap); err != nil {
		hc.SendError(err, "api_error", "unmarshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	responseMap["model"] = hc.ResponseModel
	hc.GinContext.JSON(http.StatusOK, responseMap)
	return usage.FromOpenAIResponses(resp.Usage), nil
}

// HandleOpenAIChatNonStream handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatNonStream(hc *protocol.HandleContext, resp *openai.ChatCompletion) (*protocol.TokenUsage, error) {
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
	return usage.FromOpenAIChatCompletion(resp.Usage), nil
}

// HandleOpenAIResponsesNonStream handles OpenAI Responses API non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIResponsesNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	hc.GinContext.JSON(http.StatusOK, resp)
	return usage.FromOpenAIResponses(resp.Usage), nil
}
