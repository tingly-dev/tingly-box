package server

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol/nonstream"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform/ops"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

const responseRoundtripHeader = "X-Tingly-Response-Roundtrip"

func ShouldRoundtripResponse(c *gin.Context, target string) bool {
	return strings.EqualFold(strings.TrimSpace(c.GetHeader(responseRoundtripHeader)), target)
}

func RoundtripOpenAIResponseViaAnthropic(openaiResp *openai.ChatCompletion, responseModel string, provider *typ.Provider, actualModel string) (map[string]interface{}, error) {
	anthropicResp := nonstream.ConvertOpenAIToAnthropicResponse(openaiResp, responseModel)
	return ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel), nil
}

func RoundtripOpenAIMapViaAnthropic(openaiResp map[string]interface{}, responseModel string, provider *typ.Provider, actualModel string) (map[string]interface{}, error) {
	raw, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, err
	}
	var parsed openai.ChatCompletion
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	return RoundtripOpenAIResponseViaAnthropic(&parsed, responseModel, provider, actualModel)
}

func RoundtripAnthropicBetaResponseViaOpenAI(anthropicResp *anthropic.BetaMessage, responseModel string, provider *typ.Provider, actualModel string) (*anthropic.BetaMessage, error) {
	openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
	raw, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, err
	}
	var parsed openai.ChatCompletion
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	roundtrip := nonstream.ConvertOpenAIToAnthropicResponse(&parsed, responseModel)
	return roundtrip, nil
}

// ConvertAnthropicToOpenAIResponseWithProvider converts an Anthropic response to OpenAI format
// and applies provider-specific transformations to the response
func ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp *anthropic.BetaMessage, responseModel string, provider *typ.Provider, model string) map[string]interface{} {
	// Base conversion
	openaiResp := nonstream.ConvertAnthropicToOpenAIResponse(anthropicResp, responseModel)

	// Apply provider-specific transformations using the transform system
	return ops.ApplyResponseTransforms(openaiResp, provider.APIBase, model)
}
