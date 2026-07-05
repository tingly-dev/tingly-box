package nonstream

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
)

// HandleOpenAIChatToResponses writes an OpenAI Chat response as Responses API format.
// Corresponds to stream.HandleOpenAIChatToResponsesStream.
func HandleOpenAIChatToResponses(hc *protocol.HandleContext, resp *openai.ChatCompletion, requestModel string) (*protocol.TokenUsage, error) {
	payload := BuildResponsesPayloadFromChat(resp, hc.ResponseModel, requestModel)
	hc.GinContext.JSON(http.StatusOK, payload)
	return usage.FromOpenAIChatCompletion(resp.Usage), nil
}

// HandleAnthropicBetaToResponses writes an Anthropic Beta response as Responses API format.
// Corresponds to stream.HandleAnthropicBetaToOpenAIResponsesStream.
func HandleAnthropicBetaToResponses(hc *protocol.HandleContext, resp *anthropic.BetaMessage, requestModel string) (*protocol.TokenUsage, error) {
	payload := BuildResponsesPayloadFromAnthropicBeta(resp, hc.ResponseModel, requestModel)
	hc.GinContext.JSON(http.StatusOK, payload)
	return usage.FromAnthropicBetaMessage(resp.Usage), nil
}

// BuildResponsesPayloadFromChat converts a Chat completion response to Responses API format.
func BuildResponsesPayloadFromChat(resp *openai.ChatCompletion, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	finishReason := ""
	messageContent := ""
	if len(resp.Choices) > 0 {
		messageContent = resp.Choices[0].Message.Content
		finishReason = string(resp.Choices[0].FinishReason)
	}

	status, incompleteDetails := chatFinishReasonToResponsesStatus(finishReason)
	itemStatus := status

	output := []map[string]any{}
	if messageContent != "" {
		output = append(output, map[string]any{
			// The real Responses API always assigns output items an id;
			// strict clients (AI SDK zod) require it.
			"id":     "msg_" + resp.ID,
			"type":   "message",
			"role":   "assistant",
			"status": itemStatus,
			"content": []map[string]any{
				// The real Responses API always includes annotations on
				// output_text; strict clients (AI SDK zod) require it.
				{"type": "output_text", "text": messageContent, "annotations": []any{}},
			},
		})
	}

	usageMap := map[string]any{
		"input_tokens":  resp.Usage.PromptTokens,
		"output_tokens": resp.Usage.CompletionTokens,
		"total_tokens":  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
	}
	// Carry cache-read and reasoning detail so a client reading usage off the
	// Responses-API body sees them instead of zeros.
	if cached := resp.Usage.PromptTokensDetails.CachedTokens; cached > 0 {
		usageMap["input_tokens_details"] = map[string]any{"cached_tokens": cached}
	}
	if reasoning := resp.Usage.CompletionTokensDetails.ReasoningTokens; reasoning > 0 {
		usageMap["output_tokens_details"] = map[string]any{"reasoning_tokens": reasoning}
	}

	result := map[string]any{
		"id":         resp.ID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      model,
		"status":     status,
		"output":     output,
		"usage":      usageMap,
	}
	if incompleteDetails != nil {
		result["incomplete_details"] = incompleteDetails
	}
	return result
}

func chatFinishReasonToResponsesStatus(finishReason string) (string, map[string]any) {
	switch finishReason {
	case "length":
		return "incomplete", map[string]any{"reason": "max_output_tokens"}
	case "content_filter":
		return "incomplete", map[string]any{"reason": "content_filter"}
	default:
		return "completed", nil
	}
}

// BuildResponsesPayloadFromAnthropicBeta converts an Anthropic Beta message response to Responses API format.
func BuildResponsesPayloadFromAnthropicBeta(resp *anthropic.BetaMessage, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	status, incompleteDetails := anthropicStopReasonToResponsesStatus(string(resp.StopReason))

	output := []map[string]any{}
	outputIndex := 0

	var textParts []map[string]any
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			textParts = append(textParts, map[string]any{
				"type":        "output_text",
				"text":        block.Text,
				"annotations": []any{},
			})
		case "tool_use":
			argsJSON := "{}"
			if block.Input != nil {
				if raw, err := json.Marshal(block.Input); err == nil {
					argsJSON = string(raw)
				}
			}
			output = append(output, map[string]any{
				"type":         "function_call",
				"id":           block.ID,
				"name":         block.Name,
				"arguments":    argsJSON,
				"output_index": outputIndex,
			})
			outputIndex++
		}
	}

	if len(textParts) > 0 {
		msgItem := map[string]any{
			"id":      "msg_" + resp.ID,
			"type":    "message",
			"role":    "assistant",
			"status":  status,
			"content": textParts,
		}
		output = append([]map[string]any{msgItem}, output...)
	}

	// Responses-API input_tokens is the TOTAL prompt cost. Anthropic reports
	// uncached input separately from cache-read/creation, so fold them in here
	// (matching the Chat conversion) instead of reporting only the uncached slice.
	totalInput := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens
	usageMap := map[string]any{
		"input_tokens":  totalInput,
		"output_tokens": resp.Usage.OutputTokens,
		"total_tokens":  totalInput + resp.Usage.OutputTokens,
	}
	if cached := resp.Usage.CacheReadInputTokens; cached > 0 {
		usageMap["input_tokens_details"] = map[string]any{"cached_tokens": cached}
	}

	result := map[string]any{
		"id":         resp.ID,
		"object":     "response",
		"created_at": time.Now().Unix(),
		"model":      model,
		"status":     status,
		"output":     output,
		"usage":      usageMap,
	}
	if incompleteDetails != nil {
		result["incomplete_details"] = incompleteDetails
	}
	return result
}

func anthropicStopReasonToResponsesStatus(stopReason string) (string, map[string]any) {
	switch stopReason {
	case "max_tokens":
		return "incomplete", map[string]any{"reason": "max_output_tokens"}
	default:
		return "completed", nil
	}
}
