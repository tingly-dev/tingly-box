package nonstream

import (
	"net/http"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	usageconv "github.com/tingly-dev/tingly-box/internal/protocol/usage"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

type responsesToChatNonStreamState struct {
	content   strings.Builder
	refusal   strings.Builder
	toolCalls []wire.ChatCompletionToolCallWire
}

// HandleResponsesToOpenAIChat writes a Responses API response as OpenAI Chat format.
// Corresponds to stream.HandleResponsesToOpenAIChatStream.
func HandleResponsesToOpenAIChat(hc *protocol.HandleContext, rs *responses.Response) (map[string]any, *protocol.TokenUsage, error) {
	chatResp := BuildOpenAIChatPayloadFromResponses(rs, hc.ResponseModel)
	if hc.StripUsage {
		delete(chatResp, "usage")
	}
	hc.GinContext.JSON(http.StatusOK, chatResp)
	return chatResp, usageconv.FromOpenAIResponses(rs.Usage), nil
}

// BuildOpenAIChatPayloadFromResponses converts a complete Responses result to
// the minimal Chat Completions wire shape without writing an HTTP response.
func BuildOpenAIChatPayloadFromResponses(rs *responses.Response, responseModel string) map[string]any {
	return ConvertResponsesToOpenAIChat(rs, responseModel).ToMap()
}

// ConvertResponsesToOpenAIChat builds the typed Chat Completions wire
// contract without coupling protocol conversion to HTTP or generic maps.
func ConvertResponsesToOpenAIChat(rs *responses.Response, responseModel string) wire.ChatCompletionWire {
	state := buildResponsesToChatNonStreamState(rs)
	message := wire.ChatCompletionMessageWire{
		Role:      "assistant",
		Content:   state.content.String(),
		Refusal:   state.refusal.String(),
		ToolCalls: state.toolCalls,
	}

	// The canonical wire type owns the Chat usage shape; only total_tokens is
	// overridden with the value the upstream Responses API actually reported
	// (per .design/usage-tracking.md §"wire usage map" — cover one key,
	// do not fork the constructor). Falls back to the computed value when the
	// upstream did not report one.
	usage := usageconv.ToChatUsageWire(usageconv.FromOpenAIResponses(rs.Usage))
	if reported := rs.Usage.TotalTokens; reported != 0 {
		usage.TotalTokens = reported
	}

	return wire.ChatCompletionWire{
		ID:      rs.ID,
		Object:  "chat.completion",
		Created: int64(rs.CreatedAt),
		Model:   responseModel,
		Choices: []wire.ChatCompletionChoiceWire{{
			Index:        0,
			Message:      message,
			FinishReason: mapResponsesFinishReason(rs, len(state.toolCalls) > 0),
		}},
		Usage:       usage,
		ServiceTier: protocol.ChatServiceTierFromResponses(rs.ServiceTier),
		Moderation:  protocol.ChatModerationFromResponses(rs.Moderation),
	}
}

func buildResponsesToChatNonStreamState(rs *responses.Response) *responsesToChatNonStreamState {
	state := &responsesToChatNonStreamState{
		toolCalls: make([]wire.ChatCompletionToolCallWire, 0),
	}
	if rs == nil {
		return state
	}

	for _, output := range rs.Output {
		switch output.Type {
		case "message":
			for _, contentItem := range output.Content {
				switch contentItem.Type {
				case "output_text", "text":
					state.content.WriteString(contentItem.Text)
				case "refusal":
					state.refusal.WriteString(contentItem.Refusal)
				}
			}
		case "function_call", "custom_tool_call", "mcp_call":
			state.toolCalls = append(state.toolCalls, wire.ChatCompletionToolCallWire{
				ID:   firstNonEmpty(output.CallID, output.ID),
				Type: "function",
				Function: wire.ChatCompletionFunctionWire{
					Name:      output.Name,
					Arguments: output.Arguments.OfString,
				},
			})
		}
	}

	return state
}

func mapResponsesFinishReason(rs *responses.Response, hasToolCalls bool) string {
	if hasToolCalls {
		return "tool_calls"
	}

	if rs != nil && rs.Status == "incomplete" {
		switch rs.IncompleteDetails.Reason {
		case "max_output_tokens":
			return "length"
		case "content_filter":
			return "content_filter"
		}
	}

	return "stop"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
