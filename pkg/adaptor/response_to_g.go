package adaptor

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// ConvertOpenAIToGoogleResponse converts OpenAI ChatCompletion to Google format
func ConvertOpenAIToGoogleResponse(openaiResp *openai.ChatCompletion) *genai.GenerateContentResponse {
	if openaiResp == nil {
		return nil
	}

	googleResp := &genai.GenerateContentResponse{
		Candidates:    []*genai.Candidate{},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{},
	}

	// Build candidate from first choice
	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		candidate := &genai.Candidate{
			Content: &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{},
			},
			FinishReason: mapOpenAIFinishReasonToGoogle(choice.FinishReason),
			Index:        0,
		}

		// Add text content
		if choice.Message.Content != "" {
			candidate.Content.Parts = append(candidate.Content.Parts, genai.NewPartFromText(choice.Message.Content))
		}

		// Add tool calls
		if len(choice.Message.ToolCalls) > 0 {
			for _, toolCall := range choice.Message.ToolCalls {
				var argsInput map[string]interface{}
				if toolCall.Function.Arguments != "" {
					_ = json.Unmarshal([]byte(toolCall.Function.Arguments), &argsInput)
				}

				candidate.Content.Parts = append(candidate.Content.Parts, &genai.Part{
					FunctionCall: &genai.FunctionCall{
						ID:   toolCall.ID,
						Name: toolCall.Function.Name,
						Args: argsInput,
					},
				})
			}
		}

		googleResp.Candidates = append(googleResp.Candidates, candidate)
	}

	// Add usage metadata
	googleResp.UsageMetadata.PromptTokenCount = int32(openaiResp.Usage.PromptTokens)
	googleResp.UsageMetadata.CandidatesTokenCount = int32(openaiResp.Usage.CompletionTokens)
	googleResp.UsageMetadata.TotalTokenCount = int32(openaiResp.Usage.TotalTokens)

	return googleResp
}

func mapOpenAIFinishReasonToGoogle(reason string) genai.FinishReason {
	switch reason {
	case "stop":
		return genai.FinishReasonStop
	case "length":
		return genai.FinishReasonMaxTokens
	case "content_filter":
		return genai.FinishReasonSafety
	case "tool_calls":
		return genai.FinishReasonStop
	default:
		return genai.FinishReasonOther
	}
}

// ConvertAnthropicToGoogleResponse converts Anthropic Message to Google format
func ConvertAnthropicToGoogleResponse(anthropicResp *anthropic.Message) *genai.GenerateContentResponse {
	if anthropicResp == nil {
		return nil
	}

	googleResp := &genai.GenerateContentResponse{
		Candidates:    []*genai.Candidate{},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{},
	}

	candidate := &genai.Candidate{
		Content: &genai.Content{
			Role:  "model",
			Parts: []*genai.Part{},
		},
		FinishReason: mapAnthropicFinishReasonToGoogle(string(anthropicResp.StopReason)),
		Index:        0,
	}

	// Process content blocks
	for _, block := range anthropicResp.Content {
		if block.Type == "text" {
			candidate.Content.Parts = append(candidate.Content.Parts, genai.NewPartFromText(block.Text))
		} else if block.Type == "tool_use" {
			var argsInput map[string]interface{}
			_ = json.Unmarshal(block.Input, &argsInput)

			candidate.Content.Parts = append(candidate.Content.Parts, &genai.Part{
				FunctionCall: &genai.FunctionCall{
					ID:   block.ID,
					Name: block.Name,
					Args: argsInput,
				},
			})
		}
	}

	googleResp.Candidates = append(googleResp.Candidates, candidate)

	// Add usage metadata
	googleResp.UsageMetadata.PromptTokenCount = int32(anthropicResp.Usage.InputTokens)
	googleResp.UsageMetadata.CandidatesTokenCount = int32(anthropicResp.Usage.OutputTokens)
	googleResp.UsageMetadata.TotalTokenCount = int32(anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens)

	return googleResp
}

func mapAnthropicFinishReasonToGoogle(reason string) genai.FinishReason {
	switch reason {
	case "end_turn":
		return genai.FinishReasonStop
	case "max_tokens":
		return genai.FinishReasonMaxTokens
	case "tool_use":
		return genai.FinishReasonStop
	case "content_filter":
		return genai.FinishReasonSafety
	default:
		return genai.FinishReasonOther
	}
}
