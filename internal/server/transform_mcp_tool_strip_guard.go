package server

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// MCPToolStripGuardTransform strips disabled MCP tool declarations/tool calls when enabled.
type MCPToolStripGuardTransform struct {
	server *Server
}

func NewMCPToolStripGuardTransform(server *Server) *MCPToolStripGuardTransform {
	return &MCPToolStripGuardTransform{server: server}
}

func (t *MCPToolStripGuardTransform) Name() string { return "mcp_tool_strip_guard" }

func (t *MCPToolStripGuardTransform) Apply(ctx *protocoltransform.TransformContext) error {
	if t.server == nil || t.server.mcpRuntime == nil || !t.server.mcpEnabled() {
		return nil
	}

	listCtx := ctx.Context
	if listCtx == nil {
		listCtx = context.Background()
	}
	enabled := t.server.mcpRuntime.ListEnabledServerToolNames(listCtx)
	stripEnabled := t.server.mcpStripDisabledToolsEnabled()

	var hits int
	var removed int

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		hits, removed = stripOpenAIChatDisabledMCP(req, enabled, stripEnabled)
	case *anthropic.MessageNewParams:
		hits, removed = stripAnthropicV1DisabledMCP(req, enabled, stripEnabled)
	case *anthropic.BetaMessageNewParams:
		hits, removed = stripAnthropicBetaDisabledMCP(req, enabled, stripEnabled)
	}

	if hits == 0 {
		return nil
	}
	if stripEnabled {
		logrus.WithFields(logrus.Fields{
			"hits":    hits,
			"removed": removed,
		}).Warn("mcp: stripped disabled MCP declarations/tool calls")
		return nil
	}

	logrus.WithFields(logrus.Fields{
		"hits": hits,
	}).Warn("mcp: disabled MCP declarations/tool calls detected (strip is off)")
	return nil
}

func isEnabledServerMCPTool(name string, enabled map[string]struct{}) bool {
	if !runtime.IsMCPToolName(name) {
		return true
	}
	_, ok := enabled[name]
	return ok
}

func stripOpenAIChatDisabledMCP(req *openai.ChatCompletionNewParams, enabled map[string]struct{}, doStrip bool) (int, int) {
	hits := 0
	removed := 0

	if len(req.Tools) > 0 {
		filtered := make([]openai.ChatCompletionToolUnionParam, 0, len(req.Tools))
		for _, tool := range req.Tools {
			fn := tool.GetFunction()
			if fn != nil && runtime.IsMCPToolName(fn.Name) && !isEnabledServerMCPTool(fn.Name, enabled) {
				hits++
				if doStrip {
					removed++
					continue
				}
			}
			filtered = append(filtered, tool)
		}
		if doStrip {
			req.Tools = filtered
		}
	}

	if len(req.Messages) == 0 {
		return hits, removed
	}

	if !doStrip {
		for _, msg := range req.Messages {
			raw, err := json.Marshal(msg)
			if err != nil {
				continue
			}
			var m map[string]any
			if err := json.Unmarshal(raw, &m); err != nil {
				continue
			}
			calls, _ := m["tool_calls"].([]any)
			for _, c := range calls {
				call, _ := c.(map[string]any)
				function, _ := call["function"].(map[string]any)
				name, _ := function["name"].(string)
				if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
					hits++
				}
			}
		}
		return hits, removed
	}

	blocked := make(map[string]string)
	out := make([]openai.ChatCompletionMessageParamUnion, 0, len(req.Messages))
	for _, msg := range req.Messages {
		raw, err := json.Marshal(msg)
		if err != nil {
			out = append(out, msg)
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			out = append(out, msg)
			continue
		}

		role, _ := m["role"].(string)
		if role == "tool" {
			if toolCallID, _ := m["tool_call_id"].(string); toolCallID != "" {
				if _, ok := blocked[toolCallID]; ok {
					removed++
					continue
				}
			}
		}

		var removedCalls [][2]string
		if role == "assistant" {
			if calls, ok := m["tool_calls"].([]any); ok {
				kept := make([]any, 0, len(calls))
				for _, c := range calls {
					call, _ := c.(map[string]any)
					function, _ := call["function"].(map[string]any)
					name, _ := function["name"].(string)
					id, _ := call["id"].(string)
					if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
						hits++
						removed++
						if id != "" {
							blocked[id] = name
							removedCalls = append(removedCalls, [2]string{id, name})
						}
						continue
					}
					kept = append(kept, c)
				}
				if len(kept) > 0 {
					m["tool_calls"] = kept
				} else {
					delete(m, "tool_calls")
				}
			}
		}

		converted, err := mapToOpenAIMessageParam(m)
		if err != nil {
			out = append(out, msg)
			continue
		}
		out = append(out, converted)
		for _, rc := range removedCalls {
			errPayload, _ := json.Marshal(map[string]string{"error": "calling disabled tools: " + rc[1]})
			out = append(out, openai.ToolMessage(string(errPayload), rc[0]))
		}
	}
	req.Messages = out
	return hits, removed
}

func mapToOpenAIMessageParam(m map[string]any) (openai.ChatCompletionMessageParamUnion, error) {
	var out openai.ChatCompletionMessageParamUnion
	b, err := json.Marshal(m)
	if err != nil {
		return out, err
	}
	if err := json.Unmarshal(b, &out); err != nil {
		return out, err
	}
	return out, nil
}

func stripAnthropicV1DisabledMCP(req *anthropic.MessageNewParams, enabled map[string]struct{}, doStrip bool) (int, int) {
	hits := 0
	removed := 0

	if len(req.Tools) > 0 {
		filtered := make([]anthropic.ToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.OfTool != nil && runtime.IsMCPToolName(t.OfTool.Name) && !isEnabledServerMCPTool(t.OfTool.Name, enabled) {
				hits++
				if doStrip {
					removed++
					continue
				}
			}
			filtered = append(filtered, t)
		}
		if doStrip {
			req.Tools = filtered
		}
	}

	return stripAnthropicMessagesV1(&req.Messages, enabled, doStrip, hits, removed)
}

func stripAnthropicBetaDisabledMCP(req *anthropic.BetaMessageNewParams, enabled map[string]struct{}, doStrip bool) (int, int) {
	hits := 0
	removed := 0

	if len(req.Tools) > 0 {
		filtered := make([]anthropic.BetaToolUnionParam, 0, len(req.Tools))
		for _, t := range req.Tools {
			if t.OfTool != nil && runtime.IsMCPToolName(t.OfTool.Name) && !isEnabledServerMCPTool(t.OfTool.Name, enabled) {
				hits++
				if doStrip {
					removed++
					continue
				}
			}
			filtered = append(filtered, t)
		}
		if doStrip {
			req.Tools = filtered
		}
	}

	return stripAnthropicMessagesBeta(&req.Messages, enabled, doStrip, hits, removed)
}

func stripAnthropicMessagesV1(messages *[]anthropic.MessageParam, enabled map[string]struct{}, doStrip bool, hitsBase, removedBase int) (int, int) {
	hits := hitsBase
	removed := removedBase
	if len(*messages) == 0 {
		return hits, removed
	}

	if !doStrip {
		for _, m := range *messages {
			raw, err := json.Marshal(m)
			if err != nil {
				continue
			}
			var msgMap map[string]any
			if err := json.Unmarshal(raw, &msgMap); err != nil {
				continue
			}
			content, _ := msgMap["content"].([]any)
			for _, c := range content {
				block, _ := c.(map[string]any)
				if block["type"] == "tool_use" {
					name, _ := block["name"].(string)
					if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
						hits++
					}
				}
			}
		}
		return hits, removed
	}

	blocked := make(map[string]string)
	out := make([]anthropic.MessageParam, 0, len(*messages))
	for _, m := range *messages {
		raw, err := json.Marshal(m)
		if err != nil {
			out = append(out, m)
			continue
		}
		var msgMap map[string]any
		if err := json.Unmarshal(raw, &msgMap); err != nil {
			out = append(out, m)
			continue
		}

		content, ok := msgMap["content"].([]any)
		if !ok {
			out = append(out, m)
			continue
		}

		kept := make([]any, 0, len(content))
		var removedCalls [][2]string
		for _, c := range content {
			block, _ := c.(map[string]any)
			blockType, _ := block["type"].(string)
			switch blockType {
			case "tool_use":
				name, _ := block["name"].(string)
				id, _ := block["id"].(string)
				if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
					hits++
					removed++
					if id != "" {
						blocked[id] = name
						removedCalls = append(removedCalls, [2]string{id, name})
					}
					continue
				}
				kept = append(kept, c)
			case "tool_result":
				toolUseID, _ := block["tool_use_id"].(string)
				if toolUseID != "" {
					if _, blockedResult := blocked[toolUseID]; blockedResult {
						removed++
						continue
					}
				}
				kept = append(kept, c)
			default:
				kept = append(kept, c)
			}
		}

		if len(kept) == 0 {
			continue
		}
		msgMap["content"] = kept

		var rebuilt anthropic.MessageParam
		b, err := json.Marshal(msgMap)
		if err != nil || json.Unmarshal(b, &rebuilt) != nil {
			out = append(out, m)
		} else {
			out = append(out, rebuilt)
		}

		if len(removedCalls) > 0 {
			toolResults := make([]anthropic.ContentBlockParamUnion, 0, len(removedCalls))
			for _, rc := range removedCalls {
				errPayload, _ := json.Marshal(map[string]string{"error": "calling disabled tools: " + rc[1]})
				toolResults = append(toolResults, anthropic.NewToolResultBlock(rc[0], string(errPayload), true))
			}
			out = append(out, anthropic.NewUserMessage(toolResults...))
		}
	}
	*messages = out
	return hits, removed
}

func stripAnthropicMessagesBeta(messages *[]anthropic.BetaMessageParam, enabled map[string]struct{}, doStrip bool, hitsBase, removedBase int) (int, int) {
	hits := hitsBase
	removed := removedBase
	if len(*messages) == 0 {
		return hits, removed
	}

	if !doStrip {
		for _, m := range *messages {
			raw, err := json.Marshal(m)
			if err != nil {
				continue
			}
			var msgMap map[string]any
			if err := json.Unmarshal(raw, &msgMap); err != nil {
				continue
			}
			content, _ := msgMap["content"].([]any)
			for _, c := range content {
				block, _ := c.(map[string]any)
				if block["type"] == "tool_use" {
					name, _ := block["name"].(string)
					if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
						hits++
					}
				}
			}
		}
		return hits, removed
	}

	blocked := make(map[string]string)
	out := make([]anthropic.BetaMessageParam, 0, len(*messages))
	for _, m := range *messages {
		raw, err := json.Marshal(m)
		if err != nil {
			out = append(out, m)
			continue
		}
		var msgMap map[string]any
		if err := json.Unmarshal(raw, &msgMap); err != nil {
			out = append(out, m)
			continue
		}

		content, ok := msgMap["content"].([]any)
		if !ok {
			out = append(out, m)
			continue
		}

		kept := make([]any, 0, len(content))
		var removedCalls [][2]string
		for _, c := range content {
			block, _ := c.(map[string]any)
			blockType, _ := block["type"].(string)
			switch blockType {
			case "tool_use":
				name, _ := block["name"].(string)
				id, _ := block["id"].(string)
				if runtime.IsMCPToolName(name) && !isEnabledServerMCPTool(name, enabled) {
					hits++
					removed++
					if id != "" {
						blocked[id] = name
						removedCalls = append(removedCalls, [2]string{id, name})
					}
					continue
				}
				kept = append(kept, c)
			case "tool_result":
				toolUseID, _ := block["tool_use_id"].(string)
				if toolUseID != "" {
					if _, blockedResult := blocked[toolUseID]; blockedResult {
						removed++
						continue
					}
				}
				kept = append(kept, c)
			default:
				kept = append(kept, c)
			}
		}

		if len(kept) == 0 {
			continue
		}
		msgMap["content"] = kept

		var rebuilt anthropic.BetaMessageParam
		b, err := json.Marshal(msgMap)
		if err != nil || json.Unmarshal(b, &rebuilt) != nil {
			out = append(out, m)
		} else {
			out = append(out, rebuilt)
		}

		if len(removedCalls) > 0 {
			toolResults := make([]anthropic.BetaContentBlockParamUnion, 0, len(removedCalls))
			for _, rc := range removedCalls {
				errPayload, _ := json.Marshal(map[string]string{"error": "calling disabled tools: " + rc[1]})
				toolResults = append(toolResults, anthropic.NewBetaToolResultBlock(rc[0], string(errPayload), true))
			}
			out = append(out, anthropic.NewBetaUserMessage(toolResults...))
		}
	}
	*messages = out
	return hits, removed
}
