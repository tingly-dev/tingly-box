package server

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// buildOpenAIToAnthropicMCPHooks builds the stream hooks that intercept
// virtual (in-process) MCP tool calls during an OpenAI Chat -> Anthropic
// stream conversion, executing them locally instead of forwarding them
// upstream. Moved here (from root's protocol_dispatch.go, still Step 8
// territory) alongside its sole caller, openai_mcp.go — it depends only on
// h.deps.MCPRuntime and h.CallMCPToolWithHooks, both already available here.
func (ph *ProtocolHandler) buildOpenAIToAnthropicMCPHooks(
	ctx context.Context,
	providerUUID string,
	req *openai.ChatCompletionNewParams,
) *stream.OpenAIToAnthropicMCPHooks {
	if ph == nil || ph.deps.MCPRuntime == nil || req == nil {
		return nil
	}

	registry := ph.deps.MCPRuntime.VirtualRegistry()
	hookMessages := ExtractOpenAIMessages(req.Messages)
	return &stream.OpenAIToAnthropicMCPHooks{
		ShouldSuppressTool: func(name string) bool {
			return mcp.IsVirtualTool(name, registry)
		},
		OnToolCallsFinal: func(calls []stream.OpenAIToAnthropicToolCall) error {
			if len(calls) == 0 {
				return nil
			}

			externalIDs := make([]string, 0, len(calls))
			virtualResults := make([]mcp.ToolExecutionResult, 0, len(calls))

			for _, tc := range calls {
				if !mcp.IsVirtualTool(tc.Name, registry) {
					if tc.ID != "" {
						externalIDs = append(externalIDs, tc.ID)
					}
					continue
				}

				arguments := tc.Arguments
				if arguments == "" {
					arguments = "{}"
				}
				// CallMCPToolWithHooks updates context (e.g., advisor quota), so we must propagate it
				var toolResult coretool.ToolResult
				var err error
				ctx, toolResult, err = ph.CallMCPToolWithHooks(ctx, tc.Name, arguments, hookMessages)
				if err != nil {
					logrus.WithError(err).Warnf("mcp: tool call failed name=%s arguments=%s", tc.Name, arguments)
				}
				virtualResults = append(virtualResults, mcp.ToolExecutionResult{
					ToolUseID: tc.ID,
					Contents:  toolResult.Contents,
					IsError:   err != nil,
				})
			}

			if len(virtualResults) == 0 {
				return nil
			}

			segment := BuildOpenAIContinuationSegment(calls, virtualResults)
			if len(segment) == 0 {
				return nil
			}

			if len(externalIDs) == 0 {
				req.Messages = append(req.Messages, segment...)
				return stream.ErrMCPStreamContinue
			}

			mcp.StoreOpenAIContinuationSegment(typ.GetSessionID(ctx), providerUUID, segment)
			return nil
		},
	}
}

// ExtractOpenAIMessages JSON-roundtrips an OpenAI message list into the
// generic map shape MCP tool hooks pass through as message history.
func ExtractOpenAIMessages(messages []openai.ChatCompletionMessageParamUnion) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, _ := json.Marshal(messages)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// ExtractAnthropicBetaMessages JSON-roundtrips an Anthropic beta message list
// into the generic map shape MCP tool hooks pass through as message history.
func ExtractAnthropicBetaMessages(messages []anthropic.BetaMessageParam) []map[string]any {
	if len(messages) == 0 {
		return nil
	}
	b, _ := json.Marshal(messages)
	var out []map[string]any
	_ = json.Unmarshal(b, &out)
	return out
}

// BuildOpenAIContinuationSegment builds the assistant tool_calls + tool
// result messages appended to the OpenAI message history after a round of
// virtual MCP tool execution.
func BuildOpenAIContinuationSegment(
	calls []stream.OpenAIToAnthropicToolCall,
	virtualResults []mcp.ToolExecutionResult,
) []openai.ChatCompletionMessageParamUnion {
	if len(calls) == 0 || len(virtualResults) == 0 {
		return nil
	}
	toolCalls := make([]map[string]any, 0, len(calls))
	for _, tc := range calls {
		toolCalls = append(toolCalls, map[string]any{
			"id":   tc.ID,
			"type": "function",
			"function": map[string]any{
				"name":      tc.Name,
				"arguments": tc.Arguments,
			},
		})
	}
	assistantMsg := map[string]any{
		"role":       "assistant",
		"content":    "",
		"tool_calls": toolCalls,
	}
	b, err := json.Marshal(assistantMsg)
	if err != nil {
		return nil
	}
	var assistantUnion openai.ChatCompletionMessageParamUnion
	if err := json.Unmarshal(b, &assistantUnion); err != nil {
		return nil
	}
	segment := []openai.ChatCompletionMessageParamUnion{assistantUnion}
	for _, r := range virtualResults {
		if r.ToolUseID == "" {
			continue
		}
		segment = append(segment, openai.ToolMessage(r.TextContent(), r.ToolUseID))
	}
	return segment
}
