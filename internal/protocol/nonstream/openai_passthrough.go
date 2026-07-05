package nonstream

import (
	"encoding/json"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// HandleOpenAIResponses handles Responses API passthrough (non-streaming),
// overriding the model field in the response when responseModel differs from the request model.
// Corresponds to stream.HandleOpenAIResponsesStream.
func HandleOpenAIResponses(hc *protocol.HandleContext, resp *responses.Response) (*protocol.TokenUsage, error) {
	// Prefer the upstream's actual body: re-marshaling the SDK struct emits
	// every union field with its zero value (e.g. output[].phase: "",
	// content[].annotations: null), which strict clients like the AI SDK's
	// zod schemas reject. RawJSON is empty only for locally-built responses.
	responseJSON := []byte(resp.RawJSON())
	if len(responseJSON) == 0 {
		var err error
		responseJSON, err = json.Marshal(resp)
		if err != nil {
			hc.SendError(err, "api_error", "marshal_failed")
			return protocol.ZeroTokenUsage(), err
		}
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

// HandleOpenAIChat handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChat(hc *protocol.HandleContext, resp *openai.ChatCompletion) (*protocol.TokenUsage, error) {
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
