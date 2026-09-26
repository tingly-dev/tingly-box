package nonstream

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// HandleAnthropicBetaToOpenAIResponse preserves the legacy conversion entry
// point while delegating to the transport-neutral value converter.
func HandleAnthropicBetaToOpenAIResponse(bm *anthropic.BetaMessage, responseModel string) wire.ChatCompletionWire {
	return ConvertAnthropicBetaToOpenAIChat(bm, responseModel)
}

// ConvertAnthropicBetaToOpenAIChat converts an Anthropic Beta message to the
// transport-neutral OpenAI Chat Completions wire value.
func ConvertAnthropicBetaToOpenAIChat(bm *anthropic.BetaMessage, responseModel string) wire.ChatCompletionWire {
	var toolCalls []wire.ChatCompletionToolCallWire
	var textContent string
	var thinking string

	for _, block := range bm.Content {
		switch block.Type {
		case "text":
			textContent += block.Text
		case "tool_use":
			argsJSON, _ := json.Marshal(block.Input)
			toolCalls = append(toolCalls, wire.ChatCompletionToolCallWire{
				ID:   block.ID,
				Type: "function",
				Function: wire.ChatCompletionFunctionWire{
					Name:      block.Name,
					Arguments: string(argsJSON),
				},
			})
		case "thinking":
			thinking += block.Text
		}
	}

	finishReason := "stop"
	switch bm.StopReason {
	case "tool_use":
		finishReason = "tool_calls"
	case "max_tokens":
		finishReason = "length"
	}

	msg := wire.ChatCompletionMessageWire{
		Role:             string(bm.Role),
		Content:          textContent,
		ReasoningContent: thinking,
	}
	if len(toolCalls) > 0 {
		msg.ToolCalls = toolCalls
	}

	// OpenAI wire: prompt_tokens = total (uncached + cache_read + cache_creation).
	usage := usage.ToChatUsageWire(usage.FromAnthropicBetaMessage(bm.Usage))

	return wire.ChatCompletionWire{
		ID:      bm.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   responseModel,
		Choices: []wire.ChatCompletionChoiceWire{
			{
				Index:        0,
				Message:      msg,
				FinishReason: finishReason,
			},
		},
		Usage: usage,
	}
}

// HandleAnthropicV1 handles Anthropic v1 non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1(hc *protocol.HandleContext, m *anthropic.Message) (*protocol.TokenUsage, error) {
	m.Model = anthropic.Model(hc.ResponseModel)
	hc.GinContext.JSON(http.StatusOK, m)
	return usage.FromAnthropicMessage(m.Usage), nil
}

// HandleAnthropicV1Beta handles Anthropic v1 beta non-streaming response.
// Returns (UsageStat, error)
func HandleAnthropicV1Beta(hc *protocol.HandleContext, bm *anthropic.BetaMessage) (*protocol.TokenUsage, error) {
	bm.Model = anthropic.Model(hc.ResponseModel)
	hc.GinContext.JSON(http.StatusOK, bm)
	return usage.FromAnthropicBetaMessage(bm.Usage), nil
}

// WriteAnthropicMessage writes a non-streaming Anthropic message response.
//
// It prefers the SDK message's RawJSON, which is clean by construction — the
// upstream's original body on passthrough, and the converter's wire bytes on
// converted paths (the converters build a wire DTO, so RawJSON carries no
// zero-value noise like content[].citations: null that a plain struct marshal
// would emit and that strict clients reject). It falls back to a marshal only
// when RawJSON is empty.
//
// Callers that mutate the message after receiving it (e.g. the MCP tool loop
// filtering virtual tool_use blocks) must NOT use this helper: RawJSON would
// be stale. They marshal the struct directly so the wire reflects the mutation.
//
// The public model is the one exception: callers set msg.Model to the model
// the client asked for, and the raw body still carries the provider's model
// id, so the writer patches that single field.
func WriteAnthropicMessage(c *gin.Context, msg any) {
	if r, ok := msg.(interface{ RawJSON() string }); ok {
		if raw := strings.Clone(r.RawJSON()); raw != "" {
			if model := anthropicMessageModel(msg); model != "" && gjson.Get(raw, "model").String() != model {
				if patched, err := sjson.Set(raw, "model", model); err == nil {
					raw = patched
				}
			}
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(raw))
			return
		}
	}
	c.JSON(http.StatusOK, msg)
}

// anthropicMessageModel returns the model set on an Anthropic message struct.
func anthropicMessageModel(msg any) string {
	switch m := msg.(type) {
	case *anthropic.Message:
		return string(m.Model)
	case *anthropic.BetaMessage:
		return string(m.Model)
	}
	return ""
}
