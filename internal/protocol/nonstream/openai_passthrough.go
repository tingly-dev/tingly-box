package nonstream

import (
	"encoding/json"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

const jsonContentType = "application/json; charset=utf-8"

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
	// Rewrite the single model scalar on the raw bytes instead of decoding
	// the whole (potentially large) body into a map and re-encoding it.
	modified, err := sjson.SetBytes(responseJSON, "model", hc.ResponseModel)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	hc.GinContext.Data(http.StatusOK, jsonContentType, modified)
	return usage.FromOpenAIResponses(resp.Usage), nil
}

// HandleOpenAIChat handles OpenAI chat non-streaming response.
// Returns (UsageStat, error)
func HandleOpenAIChat(hc *protocol.HandleContext, resp *openai.ChatCompletion) (*protocol.TokenUsage, error) {
	responseJSON, err := json.Marshal(resp)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}

	// Rewrite the single model scalar on the raw bytes instead of decoding
	// the whole body into a map and re-encoding it.
	modified, err := sjson.SetBytes(responseJSON, "model", hc.ResponseModel)
	if err != nil {
		hc.SendError(err, "api_error", "marshal_failed")
		return protocol.ZeroTokenUsage(), err
	}
	hc.GinContext.Data(http.StatusOK, jsonContentType, modified)
	return usage.FromOpenAIChatCompletion(resp.Usage), nil
}
