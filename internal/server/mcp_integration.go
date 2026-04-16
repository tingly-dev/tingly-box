package server

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
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
