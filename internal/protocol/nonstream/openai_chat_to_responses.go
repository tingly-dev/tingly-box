package nonstream

import (
	"net/http"
	"time"

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

// BuildResponsesPayloadFromChat converts a Chat completion response to Responses API format.
func BuildResponsesPayloadFromChat(resp *openai.ChatCompletion, responseModel, actualModel string) map[string]any {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	finishReason := ""
	messageContent := ""
	var toolCalls []openai.ChatCompletionMessageToolCallUnion
	if len(resp.Choices) > 0 {
		messageContent = resp.Choices[0].Message.Content
		finishReason = string(resp.Choices[0].FinishReason)
		toolCalls = resp.Choices[0].Message.ToolCalls
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
	// Mirror the tool_use handling BuildResponsesPayloadFromAnthropicBeta
	// (anthropic_beta_to_openai_responses.go) already has: a Chat message's
	// tool_calls must surface as function_call output items, or the
	// Responses-formatted response silently loses every tool call the model made.
	for _, tc := range toolCalls {
		if tc.Type != "" && tc.Type != "function" {
			continue
		}
		callID := tc.ID
		itemID := callID
		if itemID == "" {
			itemID = "fc_" + resp.ID
		}
		output = append(output, map[string]any{
			"type":      "function_call",
			"id":        itemID,
			"call_id":   callID,
			"name":      tc.Function.Name,
			"arguments": tc.Function.Arguments,
			"status":    itemStatus,
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
