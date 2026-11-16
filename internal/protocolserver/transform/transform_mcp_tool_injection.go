package transform

import (
	"encoding/json"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

const injectedServerToolsKey = "injected_server_tools"

// MCPToolInjectionTransform injects enabled server-side MCP tools into requests.
type MCPToolInjectionTransform struct {
	runtime *runtime.Runtime
	// connectedTools says whether connected MCP tools may be injected as well.
	// Nil means yes; when it reports false only Tingly Box's own Server Tools are.
	connectedTools func() bool
}

func NewMCPToolInjectionTransform(rt *runtime.Runtime) *MCPToolInjectionTransform {
	return &MCPToolInjectionTransform{runtime: rt}
}

// WithConnectedTools makes injection of connected MCP tools conditional.
func (t *MCPToolInjectionTransform) WithConnectedTools(on func() bool) *MCPToolInjectionTransform {
	t.connectedTools = on
	return t
}

func (t *MCPToolInjectionTransform) Name() string { return "mcp_tool_injection" }

func (t *MCPToolInjectionTransform) Apply(ctx *protocoltransform.TransformContext) error {
	if t.runtime == nil {
		return nil
	}

	// Skip injection for advisor loopback requests (X-Tingly-Advisor-Depth header
	// present). advisor_call.go marks its outbound context via
	// client.WithAdvisorLoopback, and the generic transport chains stamp the
	// header (client/advisor_loopback.go) — so any advisor call that looped back
	// through tingly-box has IsAdvisorRequest=true here.
	if ctx.IsAdvisorRequest {
		return nil
	}

	var mcpTools []openai.ChatCompletionToolUnionParam
	if t.connectedTools == nil || t.connectedTools() {
		mcpTools = t.runtime.ListServerToolsForInjection(ctx.Context)
	} else {
		mcpTools = t.runtime.ListBuiltinServerToolsForInjection(ctx.Context)
	}
	if len(mcpTools) == 0 {
		return nil
	}

	if ctx.HasNativeAdvisor {
		filtered := make([]openai.ChatCompletionToolUnionParam, 0, len(mcpTools))
		for _, tool := range mcpTools {
			fn := tool.GetFunction()
			if fn != nil && fn.Name == advisorInjectedToolName {
				continue
			}
			filtered = append(filtered, tool)
		}
		mcpTools = filtered
		if len(mcpTools) == 0 {
			return nil
		}
	}

	advisorInjected := containsAdvisorTool(mcpTools)

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		req.Tools = mergeUniqueOpenAITools(req.Tools, mcpTools)
		if advisorInjected {
			req.Messages = appendAdvisorBehaviorToOpenAISystem(req.Messages)

		}
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
			if advisorInjected {
				req.System = appendAdvisorBehaviorToAnthropicV1System(req.System)
			}
		}
	case *anthropic.BetaMessageNewParams:
		tools := request.ConvertOpenAIToAnthropicTools(mcpTools)
		if len(tools) > 0 {
			req.Tools = mergeUniqueAnthropicBetaTools(req.Tools, tools)
			if advisorInjected {
				req.System = appendAdvisorBehaviorToAnthropicBetaSystem(req.System)
			}
		}
	default:
		return nil // Responses and other protocols are deliberately not supported.
	}
	if ctx.Extra == nil {
		ctx.Extra = map[string]any{}
	}
	ctx.Extra[injectedServerToolsKey] = extractOpenAIToolNames(mcpTools)

	return nil
}

func extractOpenAIToolNames(tools []openai.ChatCompletionToolUnionParam) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if fn := t.GetFunction(); fn != nil && strings.TrimSpace(fn.Name) != "" {
			out = append(out, fn.Name)
		}
	}
	return out
}

func extractAnthropicV1ToolNames(tools []anthropic.ToolUnionParam) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t.OfTool != nil && strings.TrimSpace(t.OfTool.Name) != "" {
			out = append(out, t.OfTool.Name)
		}
	}
	return out
}

func extractAnthropicBetaToolNames(tools []anthropic.BetaToolUnionParam) []string {
	if len(tools) == 0 {
		return nil
	}
	out := make([]string, 0, len(tools))
	for _, t := range tools {
		if t.OfTool != nil && strings.TrimSpace(t.OfTool.Name) != "" {
			out = append(out, t.OfTool.Name)
		}
	}
	return out
}
