package transform

import (
	"context"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/sirupsen/logrus"
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
		logrus.Debug("[MCP-DEBUG] No MCP tools to inject")
		return nil
	}

	// Log injected tools for debugging
	injectedToolNames := make([]string, 0, len(mcpTools))
	for _, tool := range mcpTools {
		if fn := tool.GetFunction(); fn != nil {
			injectedToolNames = append(injectedToolNames, fn.Name)
		}
	}
	logrus.Debugf("[MCP-DEBUG] Injecting %d MCP tools: %v", len(injectedToolNames), injectedToolNames)

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		originalCount := 0
		if req.Tools != nil {
			originalCount = len(req.Tools)
		}
		req.Tools = mergeUniqueOpenAITools(req.Tools, mcpTools)
		logrus.Debugf("[MCP-DEBUG] OpenAI tools: original=%d, injected=%d, total=%d",
			originalCount, len(mcpTools), len(req.Tools))
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
			originalCount := 0
			if req.Tools != nil {
				originalCount = len(req.Tools)
			}
			req.Tools = mergeUniqueAnthropicV1Tools(req.Tools, toolsV1)
			logrus.Debugf("[MCP-DEBUG] Anthropic V1 tools: original=%d, injected=%d, total=%d",
				originalCount, len(toolsV1), len(req.Tools))
		}
	case *anthropic.BetaMessageNewParams:
		tools := request.ConvertOpenAIToAnthropicTools(mcpTools)
		if len(tools) > 0 {
			originalCount := 0
			if req.Tools != nil {
				originalCount = len(req.Tools)
			}
			req.Tools = mergeUniqueAnthropicBetaTools(req.Tools, tools)
			logrus.Debugf("[MCP-DEBUG] Anthropic Beta tools: original=%d, injected=%d, total=%d",
				originalCount, len(tools), len(req.Tools))
		}
	}

	return nil
}
