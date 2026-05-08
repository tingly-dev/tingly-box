package server

import (
	"context"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/servertool"
)

// serverHookDeps implements servertool.HookDeps using the Server.
type serverHookDeps struct {
	server *Server
}

func (d *serverHookDeps) GetAdvisorMaxUses() int {
	if d.server == nil || d.server.mcpRuntime == nil {
		return 0
	}
	return d.server.mcpRuntime.GetAdvisorMaxUses()
}

func (d *serverHookDeps) GetScenarioSink(ctx context.Context) *obs.Sink {
	if d.server == nil {
		return nil
	}
	scenario, ok := servertool.ScenarioFromContext(ctx)
	if !ok {
		return nil
	}
	sink := d.server.GetOrCreateScenarioSink(scenario)
	if sink == nil || !sink.IsEnabled() {
		return nil
	}
	return sink
}

// newServerExecutor creates a DefaultExecutor backed by this Server.
func (s *Server) newServerExecutor() *servertool.DefaultExecutor {
	return servertool.NewDefaultExecutor(s.mcpRuntime, &serverHookDeps{server: s})
}

// callMCPToolWithHooks executes response-phase MCP servertool hooks before the runtime call.
// Returns updated context (with advisor quota decremented), result, and error.
func (s *Server) callMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, mcpruntime.ToolResult, error) {
	return s.newServerExecutor().Execute(ctx, servertool.ToolCall{
		NormalizedName: toolName,
		Arguments:      arguments,
		Messages:       messages,
	})
}

// CallMCPToolWithHooks is the exported variant of callMCPToolWithHooks.
func (s *Server) CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, mcpruntime.ToolResult, error) {
	return s.callMCPToolWithHooks(ctx, toolName, arguments, messages)
}

// callMCPToolWithGuard is kept for test compatibility.
func (s *Server) callMCPToolWithGuard(ctx context.Context, toolName, arguments string) (mcpruntime.ToolResult, error) {
	_, result, err := s.callMCPToolWithHooks(ctx, toolName, arguments, nil)
	return result, err
}

// advisorResponseHook is kept for test compatibility (TestAdvisorResponseHook_Match*).
type advisorResponseHook = servertool.AdvisorHook

// remapLegacyAdvisorToolName is kept for test compatibility (TestRemapLegacyAdvisorToolName).
func remapLegacyAdvisorToolName(toolName string) string {
	return servertool.RemapLegacyAdvisorToolName(toolName)
}
