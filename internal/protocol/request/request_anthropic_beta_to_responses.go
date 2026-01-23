package request

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ConvertAnthropicBetaToResponsesRequestWithProvider converts Anthropic beta request to OpenAI Responses API format
// and applies provider-specific transformations
func ConvertAnthropicBetaToResponsesRequestWithProvider(
	anthropicReq *anthropic.BetaMessageNewParams,
	provider *typ.Provider,
	model string,
) responses.ResponseNewParams {
	// Base conversion
	responsesReq := ConvertAnthropicBetaToResponsesRequest(anthropicReq)

	// Set the model via raw JSON approach (similar to adaptive_probe.go)
	paramsJSON, err := json.Marshal(responsesReq)
	if err != nil {
		return responsesReq
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(paramsJSON, &raw); err != nil {
		return responsesReq
	}

	raw["model"] = model

	modifiedJSON, err := json.Marshal(raw)
	if err != nil {
		return responsesReq
	}

	json.Unmarshal(modifiedJSON, &responsesReq)

	return responsesReq
}

// ConvertAnthropicBetaToResponsesRequest converts Anthropic beta request to OpenAI Responses API format
// This is a simplified conversion that focuses on the core message content
// The Responses API has a different structure than Chat Completions
func ConvertAnthropicBetaToResponsesRequest(anthropicReq *anthropic.BetaMessageNewParams) responses.ResponseNewParams {
	// Extract the user message content from Anthropic format
	// For now, we'll use a simple approach - concatenate user messages
	var userContent string

	// Process messages to extract user content
	for _, msg := range anthropicReq.Messages {
		if msg.Role == "user" {
			// Handle the union type properly
			for _, block := range msg.Content {
				// Check if this is a text block
				if block.OfText != nil {
					userContent += block.OfText.Text
				}
			}
		}
	}

	// Create minimal Responses API request
	// The Responses API uses a simpler input structure
	params := responses.ResponseNewParams{
		Input: responses.ResponseNewParamsInputUnion{
			OfString: param.NewOpt(userContent),
		},
	}

	return params
}

// ConvertAnthropicBetaToResponsesRequestWithStreaming converts Anthropic beta request to OpenAI Responses API format
// for streaming requests
func ConvertAnthropicBetaToResponsesRequestWithStreaming(
	anthropicReq *anthropic.BetaMessageNewParams,
	provider *typ.Provider,
	model string,
) responses.ResponseNewParams {
	return ConvertAnthropicBetaToResponsesRequestWithProvider(anthropicReq, provider, model)
}
