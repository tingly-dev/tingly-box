package server

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/server/forwarding"
	"github.com/tingly-dev/tingly-box/internal/server/module/mcp"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func (s *Server) streamAnthropicBetaToOpenAIChatWithMCP(
	c *gin.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	actualModel string,
	responseModel string,
	disableStreamUsage bool,
	recorder *ProtocolRecorder,
) {
	for round := 0; round < 3; round++ {
		wrapper := s.clientPool.GetAnthropicClient(c.Request.Context(), provider, actualModel)
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

		hooks := s.buildAnthropicToOpenAIMCPHooks(c.Request.Context(), req)
		inputTokens, outputTokens, err := stream.AnthropicToOpenAIStreamWithMCPHooks(c, req, streamResp, responseModel, disableStreamUsage, hooks)
		if errors.Is(err, stream.ErrMCPStreamContinue) {
			continue
		}
		if err != nil {
			usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
			s.trackUsageWithTokenUsage(c, usage, err)
			stream.SendInternalError(c, err.Error())
			if recorder != nil {
				recorder.RecordError(err)
			}
			return
		}

		usage := protocol.NewTokenUsageWithCache(inputTokens, outputTokens, 0)
		s.trackUsageWithTokenUsage(c, usage, nil)
		return
	}
	stream.SendInternalError(c, "MCP stream continuation exceeded max rounds")
	if recorder != nil {
		recorder.RecordError(stream.ErrMCPStreamContinue)
	}
}

func (s *Server) buildAnthropicToOpenAIMCPHooks(ctx context.Context, req *anthropic.BetaMessageNewParams) *stream.AnthropicToOpenAIMCPHooks {
	if s == nil || s.mcpRuntime == nil || req == nil {
		return nil
	}
	registry := s.mcpRuntime.VirtualRegistry()
	hookMessages := extractAnthropicBetaMessages(req.Messages)
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
				var result string
				var err error
				ctx, result, err = s.callMCPToolWithHooks(ctx, tc.Name, arguments, hookMessages)
				virtualResults = append(virtualResults, mcp.ToolExecutionResult{ToolUseID: tc.ID, Content: result, IsError: err != nil})
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
		resultBlocks = append(resultBlocks, anthropic.NewBetaToolResultBlock(result.ToolUseID, result.Content, result.IsError))
	}
	req.Messages = append(req.Messages,
		anthropic.BetaMessageParam{Role: anthropic.BetaMessageParamRoleAssistant, Content: assistantBlocks},
		anthropic.NewBetaUserMessage(resultBlocks...),
	)
}
