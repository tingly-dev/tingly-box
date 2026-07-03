package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/recording"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	mcp "github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (ph *ProtocolHandler) StreamAnthropicBetaToOpenAIChatWithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	actualModel string,
	responseModel string,
	disableStreamUsage bool,
	recorder *recording.ProtocolRecorder,
) {
	for round := 0; round < 3; round++ {
		wrapper := ph.deps.ClientPool.GetAnthropicClient(c.Request.Context(), provider, actualModel)
		fc := forwarding.NewForwardContext(c.Request.Context(), provider)
		streamResp, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, req)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			stream.SendStreamingError(c, err)
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		hooks := ph.buildAnthropicToOpenAIMCPHooks(c.Request.Context(), req)
		hc := protocol.NewHandleContext(c, responseModel)
		usage, err := stream.AnthropicToOpenAIStreamWithMCPHooks(hc, req, streamResp, responseModel, disableStreamUsage, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			ph.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		ph.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if recorder != nil {
		recorder.RecordError(stream.ErrMCPStreamContinue)
	}
}

func (ph *ProtocolHandler) buildAnthropicToOpenAIMCPHooks(ctx context.Context, req *anthropic.BetaMessageNewParams) *stream.AnthropicToOpenAIMCPHooks {
	if ph == nil || ph.deps.MCPRuntime == nil || req == nil {
		return nil
	}
	registry := ph.deps.MCPRuntime.VirtualRegistry()
	hookMessages := ExtractAnthropicBetaMessages(req.Messages)
	return &stream.AnthropicToOpenAIMCPHooks{
		ShouldSuppressTool: func(name string) bool {
			return mcp.IsVirtualToolName(name, registry)
		},
		OnToolCallsFinal: func(calls []stream.AnthropicToOpenAIToolCall) error {
			if len(calls) == 0 {
				return nil
			}
			virtualResults := make([]mcp.ToolExecutionResult, 0, len(calls))
			for _, tc := range calls {
				if !mcp.IsVirtualToolName(tc.Name, registry) {
					return nil
				}
				arguments := tc.Arguments
				if arguments == "" {
					arguments = "{}"
				}
				var toolResult coretool.ToolResult
				var err error
				ctx, toolResult, err = ph.CallMCPToolWithHooks(ctx, tc.Name, arguments, hookMessages)
				virtualResults = append(virtualResults, mcp.ToolExecutionResult{ToolUseID: tc.ID, Contents: toolResult.Contents, IsError: err != nil})
			}
			appendAnthropicBetaToolContinuation(req, calls, virtualResults)
			return stream.ErrMCPStreamContinue
		},
	}
}

func appendAnthropicBetaToolContinuation(req *anthropic.BetaMessageNewParams, calls []stream.AnthropicToOpenAIToolCall, results []mcp.ToolExecutionResult) {
	if req == nil || len(calls) == 0 || len(results) == 0 {
		return
	}
	assistantBlocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(calls))
	for _, call := range calls {
		input := map[string]any{}
		_ = json.Unmarshal([]byte(call.Arguments), &input)
		assistantBlocks = append(assistantBlocks, anthropic.NewBetaToolUseBlock(call.ID, input, call.Name))
	}
	resultBlocks := make([]anthropic.BetaContentBlockParamUnion, 0, len(results))
	for _, result := range results {
		resultBlocks = append(resultBlocks, anthropic.BetaContentBlockParamUnion{
			OfToolResult: &anthropic.BetaToolResultBlockParam{
				ToolUseID: result.ToolUseID,
				Content:   mcp.ToolContentsToAnthropicBeta(result.Contents),
				IsError:   anthropic.Bool(result.IsError),
			},
		})
	}
	req.Messages = append(req.Messages,
		anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: assistantBlocks},
		anthropic.NewBetaUserMessage(resultBlocks...),
	)
}
