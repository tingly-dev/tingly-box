package adaptor

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"google.golang.org/genai"
)

// ConvertGoogleToOpenAIResponse converts Google GenerateContentResponse to OpenAI format
func ConvertGoogleToOpenAIResponse(googleResp *genai.GenerateContentResponse, responseModel string) map[string]interface{} {
	if googleResp == nil {
		return nil
	}

	// Get first candidate's content
	var textContent string
	var toolCalls []map[string]interface{}
	finishReason := "stop"

	if len(googleResp.Candidates) > 0 {
		candidate := googleResp.Candidates[0]

		// Extract text content
		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					textContent += part.Text
				}

				// Handle function calls
				if part.FunctionCall != nil {
					toolCall := map[string]interface{}{
						"id":   part.FunctionCall.ID,
						"type": "function",
						"function": map[string]interface{}{
							"name": part.FunctionCall.Name,
						},
					}
					// Marshal args to JSON string
					if argsBytes, err := json.Marshal(part.FunctionCall.Args); err == nil {
						toolCall["function"].(map[string]interface{})["arguments"] = string(argsBytes)
					}
					toolCalls = append(toolCalls, toolCall)
				}
			}
		}

		// Map finish reason
		finishReason = mapGoogleFinishReasonToOpenAI(candidate.FinishReason)
	}

	// Build message
	message := make(map[string]interface{})
	message["role"] = "assistant"

	if textContent != "" {
		message["content"] = textContent
	}
	if len(toolCalls) > 0 {
		message["tool_calls"] = toolCalls
		if finishReason == "stop" {
			finishReason = "tool_calls"
		}
	}

	// Build response
	response := map[string]interface{}{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().Unix()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   responseModel,
		"choices": []map[string]interface{}{
			{
				"index":         0,
				"message":       message,
				"finish_reason": finishReason,
			},
		},
		"usage": map[string]interface{}{
			"prompt_tokens":     0,
			"completion_tokens": 0,
			"total_tokens":      0,
		},
	}

	// Add usage info if available
	if googleResp.UsageMetadata != nil {
		response["usage"] = map[string]interface{}{
			"prompt_tokens":     googleResp.UsageMetadata.PromptTokenCount,
			"completion_tokens": googleResp.UsageMetadata.CandidatesTokenCount,
			"total_tokens":      googleResp.UsageMetadata.TotalTokenCount,
		}
	}

	return response
}

func mapGoogleFinishReasonToOpenAI(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "stop"
	case genai.FinishReasonMaxTokens:
		return "length"
	case genai.FinishReasonSafety:
		return "content_filter"
	case genai.FinishReasonRecitation, genai.FinishReasonLanguage, genai.FinishReasonOther,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent, genai.FinishReasonSPII,
		genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall,
		genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation, genai.FinishReasonImageOther, genai.FinishReasonNoImage:
		return "stop"
	default:
		return "stop"
	}
}

// ConvertGoogleToAnthropicResponse converts Google GenerateContentResponse to Anthropic format
func ConvertGoogleToAnthropicResponse(googleResp *genai.GenerateContentResponse, responseModel string) anthropic.Message {
	if googleResp == nil {
		return anthropic.Message{}
	}

	// Build response JSON
	responseJSON := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().Unix()),
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         responseModel,
		"stop_reason":   "end_turn",
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	}

	var contentBlocks []map[string]interface{}

	// Process first candidate
	if len(googleResp.Candidates) > 0 {
		candidate := googleResp.Candidates[0]

		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "text",
						"text": part.Text,
					})
				}

				if part.FunctionCall != nil {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type":  "tool_use",
						"id":    part.FunctionCall.ID,
						"name":  part.FunctionCall.Name,
						"input": part.FunctionCall.Args,
					})
				}
			}
		}

		// Map stop reason
		responseJSON["stop_reason"] = mapGoogleFinishReasonToAnthropic(candidate.FinishReason)
	}

	responseJSON["content"] = contentBlocks

	// Add usage info if available
	if googleResp.UsageMetadata != nil {
		responseJSON["usage"] = map[string]interface{}{
			"input_tokens":  googleResp.UsageMetadata.PromptTokenCount,
			"output_tokens": googleResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	// Marshal and unmarshal to create proper Message struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.Message
	json.Unmarshal(jsonBytes, &msg)

	return msg
}

func mapGoogleFinishReasonToAnthropic(reason genai.FinishReason) string {
	switch reason {
	case genai.FinishReasonStop:
		return "end_turn"
	case genai.FinishReasonMaxTokens:
		return "max_tokens"
	case genai.FinishReasonSafety:
		return "content_filter"
	case genai.FinishReasonRecitation, genai.FinishReasonLanguage, genai.FinishReasonOther,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent, genai.FinishReasonSPII,
		genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall,
		genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation, genai.FinishReasonImageOther, genai.FinishReasonNoImage:
		return "end_turn"
	default:
		return "end_turn"
	}
}

// ConvertGoogleToAnthropicBetaResponse converts Google GenerateContentResponse to Anthropic beta format
func ConvertGoogleToAnthropicBetaResponse(googleResp *genai.GenerateContentResponse, responseModel string) anthropic.BetaMessage {
	if googleResp == nil {
		return anthropic.BetaMessage{}
	}

	// Build response JSON
	responseJSON := map[string]interface{}{
		"id":            fmt.Sprintf("msg_%d", time.Now().Unix()),
		"type":          "message",
		"role":          "assistant",
		"content":       []map[string]interface{}{},
		"model":         responseModel,
		"stop_reason":   anthropic.BetaStopReasonEndTurn,
		"stop_sequence": "",
		"usage": map[string]interface{}{
			"input_tokens":  0,
			"output_tokens": 0,
		},
	}

	var contentBlocks []map[string]interface{}

	// Process first candidate
	if len(googleResp.Candidates) > 0 {
		candidate := googleResp.Candidates[0]

		if candidate.Content != nil {
			for _, part := range candidate.Content.Parts {
				if part.Text != "" {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type": "text",
						"text": part.Text,
					})
				}

				if part.FunctionCall != nil {
					contentBlocks = append(contentBlocks, map[string]interface{}{
						"type":  "tool_use",
						"id":    part.FunctionCall.ID,
						"name":  part.FunctionCall.Name,
						"input": part.FunctionCall.Args,
					})
				}
			}
		}

		// Map stop reason
		responseJSON["stop_reason"] = mapGoogleFinishReasonToAnthropicBeta(candidate.FinishReason)
	}

	responseJSON["content"] = contentBlocks

	// Add usage info if available
	if googleResp.UsageMetadata != nil {
		responseJSON["usage"] = map[string]interface{}{
			"input_tokens":  googleResp.UsageMetadata.PromptTokenCount,
			"output_tokens": googleResp.UsageMetadata.CandidatesTokenCount,
		}
	}

	// Marshal and unmarshal to create proper BetaMessage struct
	jsonBytes, _ := json.Marshal(responseJSON)
	var msg anthropic.BetaMessage
	json.Unmarshal(jsonBytes, &msg)

	return msg
}

func mapGoogleFinishReasonToAnthropicBeta(reason genai.FinishReason) anthropic.BetaStopReason {
	switch reason {
	case genai.FinishReasonStop:
		return anthropic.BetaStopReasonEndTurn
	case genai.FinishReasonMaxTokens:
		return anthropic.BetaStopReasonMaxTokens
	case genai.FinishReasonSafety:
		return anthropic.BetaStopReasonRefusal
	case genai.FinishReasonRecitation, genai.FinishReasonLanguage, genai.FinishReasonOther,
		genai.FinishReasonBlocklist, genai.FinishReasonProhibitedContent, genai.FinishReasonSPII,
		genai.FinishReasonMalformedFunctionCall, genai.FinishReasonUnexpectedToolCall,
		genai.FinishReasonImageSafety, genai.FinishReasonImageProhibitedContent,
		genai.FinishReasonImageRecitation, genai.FinishReasonImageOther, genai.FinishReasonNoImage:
		return anthropic.BetaStopReasonEndTurn
	default:
		return anthropic.BetaStopReasonEndTurn
	}
}
