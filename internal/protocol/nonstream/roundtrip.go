package nonstream

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

const responseRoundtripHeader = "X-Tingly-Response-Roundtrip"

func ShouldRoundtripResponse(c *gin.Context, target string) bool {
	return strings.EqualFold(strings.TrimSpace(c.GetHeader(responseRoundtripHeader)), target)
}

func RoundtripOpenAIResponseViaAnthropic(openaiResp *openai.ChatCompletion, responseModel string, provider *typ.Provider, actualModel string) (map[string]interface{}, error) {
	anthropicResp := ConvertOpenAIToAnthropicResponse(openaiResp, responseModel)
	return ConvertAnthropicToOpenAIResponseWithProvider(&anthropicResp, responseModel, provider, actualModel), nil
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

func RoundtripAnthropicResponseViaOpenAI(anthropicResp *anthropic.Message, responseModel string, provider *typ.Provider, actualModel string) (*anthropic.Message, error) {
	openaiResp := ConvertAnthropicToOpenAIResponseWithProvider(anthropicResp, responseModel, provider, actualModel)
	raw, err := json.Marshal(openaiResp)
	if err != nil {
		return nil, err
	}
	var parsed openai.ChatCompletion
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}
	roundtrip := ConvertOpenAIToAnthropicResponse(&parsed, responseModel)
	return &roundtrip, nil
}
