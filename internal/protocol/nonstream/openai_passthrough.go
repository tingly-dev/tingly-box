package nonstream

import (
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleOpenAIResponsesPassthroughNonStream handles Responses API passthrough (non-streaming),
// overriding the model field in the response when responseModel differs from the request model.
// Corresponds to stream.HandleOpenAIResponsesStream.
func HandleOpenAIResponsesPassthroughNonStream(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	responseMap, err := wire.OpenAIResponsesMap(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	responseMap["model"] = hc.ResponseModel
	hc.GinContext.JSON(http.StatusOK, responseMap)
	return usage.FromOpenAIResponses(resp.Usage), nil
}

// HandleOpenAIChatNonStream handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChatNonStream(hc *protocol.HandleContext, resp *openai.ChatCompletion) (*protocol.TokenUsage, error) {
	responseMap, err := wire.OpenAIChatCompletionMap(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
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
	responseMap, err := wire.OpenAIResponsesMap(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	hc.GinContext.JSON(http.StatusOK, responseMap)
	return usage.FromOpenAIResponses(resp.Usage), nil
}
