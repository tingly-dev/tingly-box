package server

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

func (s *Server) injectMCPToolsIntoOpenAIRequest(ctx context.Context, req *openai.ChatCompletionNewParams) *openai.ChatCompletionNewParams {
	if s.mcpRuntime == nil {
		return req
	}
	mcpTools := s.mcpRuntime.ListOpenAITools(ctx)
	if len(mcpTools) == 0 {
		return req
	}
	out := *req
	out.Tools = append(append([]openai.ChatCompletionToolUnionParam{}, req.Tools...), mcpTools...)
	return &out
}

func (s *Server) injectMCPToolsIntoAnthropicV1Request(ctx context.Context, req *anthropic.MessageNewParams) *anthropic.MessageNewParams {
	if s.mcpRuntime == nil {
		return req
	}
	mcpTools := s.mcpRuntime.ListOpenAITools(ctx)
	if len(mcpTools) == 0 {
		return req
	}
	betaTools := request.ConvertOpenAIToAnthropicTools(mcpTools)
	if len(betaTools) == 0 {
		return req
	}

	var toolsV1 []anthropic.ToolUnionParam
	if b, err := json.Marshal(betaTools); err == nil {
		_ = json.Unmarshal(b, &toolsV1)
	}
	if len(toolsV1) == 0 {
		return req
	}

	out := *req
	out.Tools = append(append([]anthropic.ToolUnionParam{}, req.Tools...), toolsV1...)
	return &out
}

func (s *Server) injectMCPToolsIntoAnthropicBetaRequest(ctx context.Context, req *anthropic.BetaMessageNewParams) *anthropic.BetaMessageNewParams {
	if s.mcpRuntime == nil {
		return req
	}
	mcpTools := s.mcpRuntime.ListOpenAITools(ctx)
	if len(mcpTools) == 0 {
		return req
	}
	tools := request.ConvertOpenAIToAnthropicTools(mcpTools)
	if len(tools) == 0 {
		return req
	}
	out := *req
	out.Tools = append(append([]anthropic.BetaToolUnionParam{}, req.Tools...), tools...)
	return &out
}
