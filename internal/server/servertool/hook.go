package servertool

import (
	"context"

	"github.com/tingly-dev/tingly-box/internal/client"
	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Hook prepares runtime context for a specific MCP servertool call.
// Hooks run after the worker response arrives and before local tool execution.
type Hook interface {
	Match(toolName string) bool
	PrepareContext(deps HookDeps, ctx context.Context, messages []map[string]any) context.Context
}

// HookDeps provides the server-level dependencies that hooks may need.
type HookDeps interface {
	// GetAdvisorMaxUses returns the configured maximum advisor calls per request.
	GetAdvisorMaxUses() int
	// GetScenarioSink returns an enabled recording sink for the given scenario, or nil.
	GetScenarioSink(ctx context.Context) *obs.Sink
}

// AdvisorHook injects AdvisorContext before an advisor tool call.
// Exported for test compatibility in packages that alias this type.
type AdvisorHook struct{}

func (h AdvisorHook) Match(toolName string) bool {
	sourceID, toolNameOnly, ok := mcpruntime.ParseNormalizedToolName(toolName)
	if !ok {
		return false
	}
	isAdvisorSource := sourceID == mcptools.BuiltinAdvisorSourceID || sourceID == "builtin"
	return isAdvisorSource && toolNameOnly == mcptools.BuiltinAdvisorToolName
}

func (h AdvisorHook) PrepareContext(deps HookDeps, ctx context.Context, messages []map[string]any) context.Context {
	if actx, ok := mcpruntime.GetAdvisorContext(ctx); ok {
		actx.Messages = messages
		return ctx
	}

	maxUses := deps.GetAdvisorMaxUses()
	if maxUses <= 0 {
		maxUses = 3
	}

	return mcpruntime.WithAdvisorContext(ctx, &mcpruntime.AdvisorContext{
		Messages:      messages,
		UsesRemaining: &maxUses,
	})
}

// applyHooks runs all matching hooks for the given tool name and returns the updated context.
func applyHooks(ctx context.Context, toolName string, messages []map[string]any, deps HookDeps, hooks []Hook) context.Context {
	for _, hook := range hooks {
		if hook.Match(toolName) {
			// Increment depth to prevent advisor recursion.
			depth := mcpruntime.GetAdvisorDepth(ctx)
			ctx = mcpruntime.WithAdvisorDepth(ctx, depth+1)
			ctx = hook.PrepareContext(deps, ctx, messages)

			// Inject scenario record sink so advisor HTTP calls get recorded.
			if sink := deps.GetScenarioSink(ctx); sink != nil && sink.IsEnabled() {
				ctx = mcpruntime.WithAdvisorRecordSink(ctx, sink)
			}
		}
	}
	return ctx
}

// ScenarioFromContext reads the scenario stored under client.ScenarioContextKey.
// Useful for HookDeps.GetScenarioSink implementations.
func ScenarioFromContext(ctx context.Context) (typ.RuleScenario, bool) {
	v := ctx.Value(client.ScenarioContextKey)
	if v == nil {
		return "", false
	}
	return typ.RuleScenario(v.(string)), true
}
