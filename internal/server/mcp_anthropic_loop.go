package server

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/mcpruntime"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

func hasOnlyMCPToolUsesV1(content []anthropic.ContentBlockUnion) ([]anthropic.ToolUseBlock, bool) {
	toolUses := make([]anthropic.ToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			if !mcpruntime.IsMCPToolName(v.Name) {
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
			if !mcpruntime.IsMCPToolName(v.Name) {
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

func (s *Server) handleAnthropicV1MCPToolCalls(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.MessageNewParams,
	resp *anthropic.Message,
) (*anthropic.Message, *anthropic.MessageNewParams, error) {
	if s.mcpRuntime == nil {
		return resp, req, nil
	}

	currentReq := req
	currentResp := resp
	const maxRounds = 6

	for round := 0; round < maxRounds; round++ {
		toolUses, ok := hasOnlyMCPToolUsesV1(currentResp.Content)
		if !ok {
			return currentResp, currentReq, nil
		}

		toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(toolUses))
		for _, tu := range toolUses {
			arguments := string(tu.Input)
			if arguments == "" {
				arguments = "{}"
			}
			result, err := s.mcpRuntime.CallTool(ctx, tu.Name, arguments)
			if err != nil {
				result = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}
			toolResults = append(toolResults, anthropic.NewToolResultBlock(tu.ID, result, err != nil))
		}

		nextReq := *currentReq
		nextReq.Messages = append(append([]anthropic.MessageParam{}, currentReq.Messages...), currentResp.ToParam(), anthropic.NewUserMessage(toolResults...))
		nextReq = *s.injectMCPToolsIntoAnthropicV1Request(ctx, &nextReq)

		wrapper := s.clientPool.GetAnthropicClient(provider, string(nextReq.Model))
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

func (s *Server) handleAnthropicBetaMCPToolCalls(
	ctx context.Context,
	provider *typ.Provider,
	req *anthropic.BetaMessageNewParams,
	resp *anthropic.BetaMessage,
) (*anthropic.BetaMessage, *anthropic.BetaMessageNewParams, error) {
	if s.mcpRuntime == nil {
		return resp, req, nil
	}

	currentReq := req
	currentResp := resp
	const maxRounds = 6

	for round := 0; round < maxRounds; round++ {
		toolUses, ok := hasOnlyMCPToolUsesBeta(currentResp.Content)
		if !ok {
			return currentResp, currentReq, nil
		}

		toolResults := make([]anthropic.BetaContentBlockParamUnion, 0, len(toolUses))
		for _, tu := range toolUses {
			arguments := "{}"
			if b, err := json.Marshal(tu.Input); err == nil && len(b) > 0 {
				arguments = string(b)
			}
			result, err := s.mcpRuntime.CallTool(ctx, tu.Name, arguments)
			if err != nil {
				result = fmt.Sprintf(`{"error":"%s"}`, err.Error())
			}
			toolResults = append(toolResults, anthropic.NewBetaToolResultBlock(tu.ID, result, err != nil))
		}

		nextReq := *currentReq
		nextReq.Messages = append(append([]anthropic.BetaMessageParam{}, currentReq.Messages...), currentResp.ToParam(), anthropic.NewBetaUserMessage(toolResults...))
		nextReq = *s.injectMCPToolsIntoAnthropicBetaRequest(ctx, &nextReq)

		wrapper := s.clientPool.GetAnthropicClient(provider, string(nextReq.Model))
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
