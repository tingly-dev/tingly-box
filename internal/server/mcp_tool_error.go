package server

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// handlerHookDeps implements servertool.HookDeps using the ProtocolHandler.
type handlerHookDeps struct {
	handler *ProtocolHandler
}

func (d *handlerHookDeps) GetAdvisorMaxUses() int {
	if d.handler == nil || d.handler.deps.MCPRuntime == nil {
		return 0
	}
	return d.handler.deps.MCPRuntime.GetAdvisorMaxUses()
}

func (d *handlerHookDeps) GetScenarioSink(ctx context.Context) *obs.Sink {
	if d.handler == nil || d.handler.deps.GetOrCreateScenarioSink == nil {
		return nil
	}
	scenario, ok := servertool.ScenarioFromContext(ctx)
	if !ok {
		return nil
	}
	sink := d.handler.deps.GetOrCreateScenarioSink(scenario)
	if sink == nil || !sink.IsEnabled() {
		return nil
	}
	return sink
}

// newServerExecutor creates a DefaultExecutor backed by this ProtocolHandler.
func (ph *ProtocolHandler) newServerExecutor() *servertool.DefaultExecutor {
	var pipeline *servertool.Pipeline
	if ph.deps.GetServertoolPipeline != nil {
		pipeline = ph.deps.GetServertoolPipeline()
	}
	if pipeline != nil {
		return pipeline.NewExecutor(ph.deps.MCPRuntime, &handlerHookDeps{handler: ph})
	}
	return servertool.NewDefaultExecutor(ph.deps.MCPRuntime, &handlerHookDeps{handler: ph})
}

// CallMCPToolWithHooks executes response-phase MCP servertool hooks before the
// runtime call. Returns updated context (with advisor quota decremented),
// result, and error.
func (ph *ProtocolHandler) CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, coretool.ToolResult, error) {
	return ph.newServerExecutor().Execute(ctx, servertool.ToolCall{
		NormalizedName: toolName,
		Arguments:      arguments,
		Messages:       messages,
	})
}

// advisorResponseHook is kept for test compatibility (TestAdvisorResponseHook_Match*).
type advisorResponseHook = servertool.AdvisorHook

// remapLegacyAdvisorToolName is kept for test compatibility (TestRemapLegacyAdvisorToolName).
func remapLegacyAdvisorToolName(toolName string) string {
	return servertool.RemapLegacyAdvisorToolName(toolName)
}
