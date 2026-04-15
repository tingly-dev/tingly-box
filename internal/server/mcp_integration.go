package server

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/request"
)

func mergeUniqueOpenAITools(existing []openai.ChatCompletionToolUnionParam, mcpTools []openai.ChatCompletionToolUnionParam) []openai.ChatCompletionToolUnionParam {
	out := make([]openai.ChatCompletionToolUnionParam, 0, len(existing)+len(mcpTools))
	seen := make(map[string]struct{}, len(existing)+len(mcpTools))

	for _, t := range existing {
		out = append(out, t)
		if fn := t.GetFunction(); fn != nil && fn.Name != "" {
			seen[fn.Name] = struct{}{}
		}
	}
	for _, t := range mcpTools {
		fn := t.GetFunction()
		if fn == nil || fn.Name == "" {
			out = append(out, t)
			continue
		}
		if _, ok := seen[fn.Name]; ok {
			continue
		}
		seen[fn.Name] = struct{}{}
		out = append(out, t)
	}
	return out
}

func mergeUniqueAnthropicV1Tools(existing []anthropic.ToolUnionParam, mcpTools []anthropic.ToolUnionParam) []anthropic.ToolUnionParam {
	out := make([]anthropic.ToolUnionParam, 0, len(existing)+len(mcpTools))
	seen := make(map[string]struct{}, len(existing)+len(mcpTools))

	for _, t := range existing {
		out = append(out, t)
		if t.OfTool != nil && t.OfTool.Name != "" {
			seen[t.OfTool.Name] = struct{}{}
		}
	}
	for _, t := range mcpTools {
		if t.OfTool == nil || t.OfTool.Name == "" {
			out = append(out, t)
			continue
		}
		if _, ok := seen[t.OfTool.Name]; ok {
			continue
		}
		seen[t.OfTool.Name] = struct{}{}
		out = append(out, t)
	}
	return out
}

func mergeUniqueAnthropicBetaTools(existing []anthropic.BetaToolUnionParam, mcpTools []anthropic.BetaToolUnionParam) []anthropic.BetaToolUnionParam {
	out := make([]anthropic.BetaToolUnionParam, 0, len(existing)+len(mcpTools))
	seen := make(map[string]struct{}, len(existing)+len(mcpTools))

	for _, t := range existing {
		out = append(out, t)
		if t.OfTool != nil && t.OfTool.Name != "" {
			seen[t.OfTool.Name] = struct{}{}
		}
	}
	for _, t := range mcpTools {
		if t.OfTool == nil || t.OfTool.Name == "" {
			out = append(out, t)
			continue
		}
		if _, ok := seen[t.OfTool.Name]; ok {
			continue
		}
		seen[t.OfTool.Name] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (s *Server) injectMCPToolsIntoOpenAIRequest(ctx context.Context, req *openai.ChatCompletionNewParams) *openai.ChatCompletionNewParams {
	if s.mcpRuntime == nil {
		logrus.Debugf("mcp: inject - mcpRuntime is nil")
		return req
	}
	mcpTools := s.mcpRuntime.ListOpenAITools(ctx)
	if len(mcpTools) == 0 {
		logrus.Debugf("mcp: inject - no tools returned")
		return req
	}
	logrus.Debugf("mcp: inject - injecting %d MCP tools", len(mcpTools))
	out := *req
	out.Tools = mergeUniqueOpenAITools(req.Tools, mcpTools)
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
	out.Tools = mergeUniqueAnthropicV1Tools(req.Tools, toolsV1)
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
	out.Tools = mergeUniqueAnthropicBetaTools(req.Tools, tools)
	return &out
}
