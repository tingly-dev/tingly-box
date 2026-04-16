package transform

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// MCPToolInjectionTransform injects enabled server-side MCP tools into requests.
type MCPToolInjectionTransform struct {
	runtime *runtime.Runtime
}

func NewMCPToolInjectionTransform(rt *runtime.Runtime) *MCPToolInjectionTransform {
	return &MCPToolInjectionTransform{runtime: rt}
}

func (t *MCPToolInjectionTransform) Name() string { return "mcp_tool_injection" }

func (t *MCPToolInjectionTransform) Apply(ctx *protocoltransform.TransformContext) error {
	if t.runtime == nil {
		return nil
	}

	listCtx := ctx.Context
	if listCtx == nil {
		listCtx = context.Background()
	}
	mcpTools := t.runtime.ListOpenAITools(listCtx)
	if len(mcpTools) == 0 {
		return nil
	}

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		req.Tools = mergeUniqueOpenAITools(req.Tools, mcpTools)
	case *anthropic.MessageNewParams:
		betaTools := request.ConvertOpenAIToAnthropicTools(mcpTools)
		if len(betaTools) == 0 {
			return nil
		}
		var toolsV1 []anthropic.ToolUnionParam
		if b, err := json.Marshal(betaTools); err == nil {
			_ = json.Unmarshal(b, &toolsV1)
		}
		if len(toolsV1) > 0 {
			req.Tools = mergeUniqueAnthropicV1Tools(req.Tools, toolsV1)
		}
	case *anthropic.BetaMessageNewParams:
		tools := request.ConvertOpenAIToAnthropicTools(mcpTools)
		if len(tools) > 0 {
			req.Tools = mergeUniqueAnthropicBetaTools(req.Tools, tools)
		}
	}

	return nil
}
