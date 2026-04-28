package transform

import (
	"encoding/json"
	"strconv"
	"strings"

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

	// Skip injection for advisor loopback requests (X-Tingly-Advisor-Depth header present).
	// advisor_call.go sets this header on all outgoing advisor HTTP requests, so any request
	// that looped back through tingly-box will have IsAdvisorRequest=true here.
	if ctx.IsAdvisorRequest {
		return nil
	}

	mcpTools := t.runtime.ListServerToolsForInjection(ctx.Context)
	if len(mcpTools) == 0 {
		return nil
	}

	advisorInjected := containsAdvisorTool(mcpTools)

	switch req := ctx.Request.(type) {
	case *openai.ChatCompletionNewParams:
		originalCount := 0
		if req.Tools != nil {
			originalCount = len(req.Tools)
		}
		logOriginalToolNames("OpenAI", extractOpenAIToolNames(req.Tools))
		req.Tools = mergeUniqueOpenAITools(req.Tools, mcpTools)
		logrus.Debugf("[MCP-DEBUG] OpenAI tools: original=%d, injected=%d, total=%d",
			originalCount, len(mcpTools), len(req.Tools))
		if advisorInjected {
			originalLen := len(req.Messages)
			req.Messages = appendAdvisorBehaviorToOpenAISystem(req.Messages)
			logrus.Debugf("[MCP-DEBUG] Advisor system-prompt appended (OpenAI Chat): messages %d -> %d", originalLen, len(req.Messages))
		}
	case *anthropic.MessageNewParams:
		logOriginalToolNames("Anthropic V1", extractAnthropicV1ToolNames(req.Tools))
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
			if advisorInjected {
				originalSystemLen := len(req.System)
				req.System = appendAdvisorBehaviorToAnthropicV1System(req.System)
				logrus.Debugf("[MCP-DEBUG] Advisor system-prompt appended (Anthropic V1): blocks %d -> %d", originalSystemLen, len(req.System))
			}
		}
	case *anthropic.BetaMessageNewParams:
		logOriginalToolNames("Anthropic Beta", extractAnthropicBetaToolNames(req.Tools))
		tools := request.ConvertOpenAIToAnthropicTools(mcpTools)
		if len(tools) > 0 {
			originalCount := 0
			if req.Tools != nil {
				originalCount = len(req.Tools)
			}
			req.Tools = mergeUniqueAnthropicBetaTools(req.Tools, tools)
			injectedNames := make([]string, 0, len(tools))
			for _, t := range tools {
				if t.OfTool != nil {
					injectedNames = append(injectedNames, t.OfTool.Name)
				}
			}
			logrus.Debugf("[MCP-DEBUG] Anthropic Beta tools: original=%d, injected=%d, total=%d, names=%v",
				originalCount, len(tools), len(req.Tools), injectedNames)
			if advisorInjected {
				originalSystemLen := len(req.System)
				req.System = appendAdvisorBehaviorToAnthropicBetaSystem(req.System)
				logrus.Debugf("[MCP-DEBUG] Advisor system-prompt appended (Anthropic Beta): blocks %d -> %d", originalSystemLen, len(req.System))
			}
		}
	}

	return nil
}

const mcpOriginalNamesLogLimit = 12

func logOriginalToolNames(api string, names []string) {
	if len(names) == 0 {
		logrus.Debugf("[MCP-DEBUG] %s original tool names: []", api)
		return
	}
	limit := len(names)
	if limit > mcpOriginalNamesLogLimit {
		limit = mcpOriginalNamesLogLimit
	}
	clipped := names[:limit]
	suffix := ""
	if len(names) > limit {
		suffix = " ...+" + strconv.Itoa(len(names)-limit)
	}
	logrus.Debugf("[MCP-DEBUG] %s original tool names: [%s]%s", api, strings.Join(clipped, ", "), suffix)
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
