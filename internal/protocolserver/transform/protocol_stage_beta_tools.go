package transform

import (
	"context"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// ProtocolStageBetaToolProvider prepares the Beta working request with the
// exact MCP server tools owned by the Protocol Stage ToolLoop. It reuses the
// existing merge and Advisor-system behavior without invoking a generic LLM
// tool representation.
type ProtocolStageBetaToolProvider struct {
	runtime          *runtime.Runtime
	isAdvisorRequest bool
	hasNativeAdvisor bool
}

func NewProtocolStageBetaToolProvider(
	rt *runtime.Runtime,
	isAdvisorRequest bool,
	hasNativeAdvisor bool,
) *ProtocolStageBetaToolProvider {
	return &ProtocolStageBetaToolProvider{
		runtime:          rt,
		isAdvisorRequest: isAdvisorRequest,
		hasNativeAdvisor: hasNativeAdvisor,
	}
}

func (p *ProtocolStageBetaToolProvider) PrepareRequest(
	ctx context.Context,
	request *anthropic.BetaMessageNewParams,
) ([]string, error) {
	if p == nil || p.runtime == nil || request == nil || p.isAdvisorRequest {
		return nil, nil
	}
	tools := p.runtime.ListServerToolsForAnthropicBetaInjection(ctx)
	if len(tools) == 0 {
		return nil, nil
	}
	if p.hasNativeAdvisor {
		filtered := make([]anthropic.BetaToolUnionParam, 0, len(tools))
		for _, tool := range tools {
			if tool.OfTool != nil && tool.OfTool.Name == advisorInjectedToolName {
				continue
			}
			filtered = append(filtered, tool)
		}
		tools = filtered
	}
	if len(tools) == 0 {
		return nil, nil
	}
	request.Tools = mergeUniqueAnthropicBetaTools(request.Tools, tools)
	names := extractAnthropicBetaToolNames(tools)
	if containsString(names, advisorInjectedToolName) {
		request.System = appendAdvisorBehaviorToAnthropicBetaSystem(request.System)
	}
	return names, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
