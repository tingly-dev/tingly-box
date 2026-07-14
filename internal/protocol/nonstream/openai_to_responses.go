package nonstream

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
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
	return ConvertChatToResponsesWire(resp, responseModel, actualModel).ToMap()
}

// ConvertChatToResponsesWire builds the typed Responses API output contract
// without routing protocol conversion through generic maps or JSON decoding.
func ConvertChatToResponsesWire(resp *openai.ChatCompletion, responseModel, actualModel string) wire.ResponsesWireResponse {
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

	status, incompleteReason := chatFinishReasonToResponsesStatus(finishReason)
	itemStatus := status

	output := []wire.ResponsesOutputItemWire{}
	if messageContent != "" {
		output = append(output, wire.ResponsesOutputItemWire{
			// The real Responses API always assigns output items an id;
			// strict clients (AI SDK zod) require it.
			ID:     "msg_" + resp.ID,
			Type:   "message",
			Role:   "assistant",
			Status: itemStatus,
			Content: []wire.ResponsesContentPartWire{
				// The real Responses API always includes annotations on
				// output_text; strict clients (AI SDK zod) require it.
				{Type: "output_text", Text: messageContent, Annotations: []any{}},
			},
		})
	}

	usageWire := &wire.ResponsesUsageWire{
		InputTokens:  resp.Usage.PromptTokens,
		OutputTokens: resp.Usage.CompletionTokens,
		TotalTokens:  resp.Usage.PromptTokens + resp.Usage.CompletionTokens,
	}
	// Carry cache-read and reasoning detail so a client reading usage off the
	// Responses-API body sees them instead of zeros.
	if cached := resp.Usage.PromptTokensDetails.CachedTokens; cached > 0 {
		usageWire.InputTokensDetails.CachedTokens = cached
	}
	if reasoning := resp.Usage.CompletionTokensDetails.ReasoningTokens; reasoning > 0 {
		usageWire.OutputTokensDetails.ReasoningTokens = reasoning
	}

	result := wire.ResponsesWireResponse{
		ID:        resp.ID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Model:     model,
		Status:    status,
		Output:    output,
		Usage:     usageWire,
	}
	if incompleteReason != "" {
		result.IncompleteDetails = &wire.ResponsesIncompleteDetailsWire{
			Reason: incompleteReason,
		}
	}
	return result
}

func chatFinishReasonToResponsesStatus(finishReason string) (status, incompleteReason string) {
	switch finishReason {
	case "length":
		return "incomplete", "max_output_tokens"
	case "content_filter":
		return "incomplete", "content_filter"
	default:
		return "completed", ""
	}
}

// BuildResponsesPayloadFromAnthropicBeta converts an Anthropic Beta message response to Responses API format.
func BuildResponsesPayloadFromAnthropicBeta(resp *anthropic.BetaMessage, responseModel, actualModel string) map[string]any {
	return ConvertAnthropicBetaToResponsesWire(resp, responseModel, actualModel).ToMap()
}

// ConvertAnthropicBetaToResponsesWire builds the typed Responses API output
// contract without coupling the Bridge to transport or SDK JSON internals.
func ConvertAnthropicBetaToResponsesWire(resp *anthropic.BetaMessage, responseModel, actualModel string) wire.ResponsesWireResponse {
	model := responseModel
	if model == "" {
		model = actualModel
	}

	status, incompleteReason := anthropicStopReasonToResponsesStatus(string(resp.StopReason))

	output := []wire.ResponsesOutputItemWire{}
	outputIndex := 0

	var textParts []wire.ResponsesContentPartWire
	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			if block.Text == "" {
				continue
			}
			textParts = append(textParts, wire.ResponsesContentPartWire{
				Type:        "output_text",
				Text:        block.Text,
				Annotations: []any{},
			})
		case "tool_use":
			argsJSON := "{}"
			if block.Input != nil {
				if raw, err := json.Marshal(block.Input); err == nil {
					argsJSON = string(raw)
				}
			}
			index := outputIndex
			output = append(output, wire.ResponsesOutputItemWire{
				Type:        "function_call",
				ID:          block.ID,
				Name:        block.Name,
				Arguments:   &argsJSON,
				OutputIndex: &index,
			})
			outputIndex++
		}
	}

	if len(textParts) > 0 {
		msgItem := wire.ResponsesOutputItemWire{
			ID:      "msg_" + resp.ID,
			Type:    "message",
			Role:    "assistant",
			Status:  status,
			Content: textParts,
		}
		output = append([]wire.ResponsesOutputItemWire{msgItem}, output...)
	}

	// Responses-API input_tokens is the TOTAL prompt cost. Anthropic reports
	// uncached input separately from cache-read/creation, so fold them in here
	// (matching the Chat conversion) instead of reporting only the uncached slice.
	totalInput := resp.Usage.InputTokens + resp.Usage.CacheReadInputTokens + resp.Usage.CacheCreationInputTokens
	usageWire := &wire.ResponsesUsageWire{
		InputTokens:  totalInput,
		OutputTokens: resp.Usage.OutputTokens,
		TotalTokens:  totalInput + resp.Usage.OutputTokens,
	}
	if cached := resp.Usage.CacheReadInputTokens; cached > 0 {
		usageWire.InputTokensDetails.CachedTokens = cached
	}

	result := wire.ResponsesWireResponse{
		ID:        resp.ID,
		Object:    "response",
		CreatedAt: time.Now().Unix(),
		Model:     model,
		Status:    status,
		Output:    output,
		Usage:     usageWire,
	}
	if incompleteReason != "" {
		result.IncompleteDetails = &wire.ResponsesIncompleteDetailsWire{
			Reason: incompleteReason,
		}
	}
	return result
}

func anthropicStopReasonToResponsesStatus(stopReason string) (status, incompleteReason string) {
	switch stopReason {
	case "max_tokens":
		return "incomplete", "max_output_tokens"
	default:
		return "completed", ""
	}
}
