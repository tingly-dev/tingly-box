package server

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/server/servertool"
	coretool "github.com/tingly-dev/tingly-box/internal/tool"
)

// handlerHookDeps implements servertool.HookDeps using the AIHandler.
type handlerHookDeps struct {
	handler *AIHandler
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

// newServerExecutor creates a DefaultExecutor backed by this AIHandler.
func (ah *AIHandler) newServerExecutor() *servertool.DefaultExecutor {
	var pipeline *servertool.Pipeline
	if ah.deps.GetServertoolPipeline != nil {
		pipeline = ah.deps.GetServertoolPipeline()
	}
	if pipeline != nil {
		return pipeline.NewExecutor(ah.deps.MCPRuntime, &handlerHookDeps{handler: ah})
	}
	return servertool.NewDefaultExecutor(ah.deps.MCPRuntime, &handlerHookDeps{handler: ah})
}

// CallMCPToolWithHooks executes response-phase MCP servertool hooks before the
// runtime call. Returns updated context (with advisor quota decremented),
// result, and error.
func (ah *AIHandler) CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, coretool.ToolResult, error) {
	return ah.newServerExecutor().Execute(ctx, servertool.ToolCall{
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
