package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func hasOnlyMCPToolUsesV1(content []anthropic.ContentBlockUnion) ([]anthropic.ToolUseBlock, bool) {
	toolUses := make([]anthropic.ToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			if !runtime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

func hasOnlyMCPToolUsesBeta(content []anthropic.BetaContentBlockUnion) ([]anthropic.BetaToolUseBlock, bool) {
	toolUses := make([]anthropic.BetaToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaToolUseBlock:
			if !runtime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

// handleAnthropicV1MCPToolCalls executes MCP tool calls in a loop until no more MCP tools
// are returned. Returns the final (possibly modified) response and request.
func (s *Server) handleAnthropicV1MCPToolCalls(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.MessageNewParams,
	resp *anthropic.Message,
) (*anthropic.Message, *anthropic.MessageNewParams, error) {
	if s.mcpRuntime == nil || !s.mcpEnabled() {
		return resp, req, nil
	}

	currentReq := req
	currentResp := resp
	const maxRounds = 6

	for round := 0; round < maxRounds; round++ {
		toolUses, ok := hasOnlyMCPToolUsesV1(currentResp.Content)
		if !ok {
			logrus.Debugf("[MCP-DEBUG] V1 round %d: no MCP tool uses found, exiting loop", round)
			return currentResp, currentReq, nil
		}

		// Log all tool uses for debugging
		toolNames := make([]string, 0, len(toolUses))
		for _, tu := range toolUses {
			toolNames = append(toolNames, tu.Name)
		}
		logrus.Debugf("[MCP-DEBUG] V1 round %d: executing %d MCP tools: %v", round, len(toolUses), toolNames)

		toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(toolUses))
		hookMessages := extractAnthropicV1Messages(append(append([]anthropic.MessageParam{}, currentReq.Messages...), currentResp.ToParam()))
		for _, tu := range toolUses {
			arguments := string(tu.Input)
			if arguments == "" {
				arguments = "{}"
			}
			result, err := s.callMCPToolWithHooks(ctx, tu.Name, arguments, hookMessages)
			if err != nil {
				logrus.WithError(err).Warnf("mcp: tool call failed name=%s arguments=%s", tu.Name, arguments)
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, result, err != nil))
		}

		nextReq := *currentReq
		nextReq.Messages = append(append([]anthropic.MessageParam{}, currentReq.Messages...), currentResp.ToParam(), anthropic.NewUserMessage(toolResults...))

		wrapper := s.clientPool.GetAnthropicClient(ctx, provider, nextReq.Model)
		fc := NewForwardContext(nil, provider)
		nextResp, cancel, err := ForwardAnthropicV1(fc, wrapper, &nextReq)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get follow-up response after mcp tool execution: %w", err)
		}

		currentReq = &nextReq
		currentResp = nextResp
	}

	return currentResp, currentReq, nil
}

// respondMCPError writes a JSON error response for non-streaming MCP tool call failures.
// This consolidates the ~10-line error block repeated across dispatch paths.
func respondMCPError(s *Server, c *gin.Context, recorder *ProtocolRecorder, err error, msg string) {
	s.trackUsageFromContext(c, 0, 0, err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: msg + ": " + err.Error(),
			Type:    "api_error",
		},
	})
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// recordMCPError sends a streaming error response for streaming MCP tool call failures.
func recordMCPError(s *Server, c *gin.Context, err error, recorder *ProtocolRecorder) {
	s.trackUsageFromContext(c, 0, 0, err)
	stream.SendStreamingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// recordMCPForwardingError handles MCP errors in non-streaming forward paths.
func recordMCPForwardingError(s *Server, c *gin.Context, err error, recorder *ProtocolRecorder) {
	s.trackUsageFromContext(c, 0, 0, err)
	stream.SendForwardingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// handleAnthropicBetaMCPToolCalls executes MCP tool calls in a loop until no more MCP tools
// are returned. Returns the final (possibly modified) response and request.
func (s *Server) handleAnthropicBetaMCPToolCalls(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	resp *anthropic.BetaMessage,
) (*anthropic.BetaMessage, *anthropic.BetaMessageNewParams, error) {
	if s.mcpRuntime == nil || !s.mcpEnabled() {
		return resp, req, nil
	}

	currentReq := req
	currentResp := resp
	const maxRounds = 6

	for round := 0; round < maxRounds; round++ {
		toolUses, ok := hasOnlyMCPToolUsesBeta(currentResp.Content)
		if !ok {
			logrus.Debugf("[MCP-DEBUG] Beta round %d: no MCP tool uses found, exiting loop", round)
			return currentResp, currentReq, nil
		}

		// Log all tool uses for debugging
		toolNames := make([]string, 0, len(toolUses))
		for _, tu := range toolUses {
			toolNames = append(toolNames, tu.Name)
		}
		logrus.Debugf("[MCP-DEBUG] Beta round %d: executing %d MCP tools: %v", round, len(toolUses), toolNames)

		toolResults := make([]anthropic.BetaContentBlockParamUnion, 0, len(toolUses))
		hookMessages := extractAnthropicBetaMessages(append(append([]anthropic.BetaMessageParam{}, currentReq.Messages...), currentResp.ToParam()))
		for _, tu := range toolUses {
			arguments := "{}"
			if b, err := json.Marshal(tu.Input); err == nil && len(b) > 0 {
				arguments = string(b)
			}
			result, err := s.callMCPToolWithHooks(ctx, tu.Name, arguments, hookMessages)
			if err != nil {
				logrus.WithError(err).Warnf("mcp: beta tool call failed name=%s arguments=%s", tu.Name, arguments)
			}
			toolResults = append(toolResults, anthropic.NewBetaToolResultBlock(tu.ID, result, err != nil))
		}

		nextReq := *currentReq
		nextReq.Messages = append(append([]anthropic.BetaMessageParam{}, currentReq.Messages...), currentResp.ToParam(), anthropic.NewBetaUserMessage(toolResults...))
		wrapper := s.clientPool.GetAnthropicClient(ctx, provider, nextReq.Model)
		fc := NewForwardContext(nil, provider)
		nextResp, cancel, err := ForwardAnthropicV1Beta(fc, wrapper, &nextReq)
		if cancel != nil {
			defer cancel()
		}
		if err != nil {
			return nil, nil, fmt.Errorf("failed to get follow-up beta response after mcp tool execution: %w", err)
		}

		currentReq = &nextReq
		currentResp = nextResp
	}

	return currentResp, currentReq, nil
}
