package servertool

import (
	"context"

	coretool "github.com/tingly-dev/tingly-box/internal/tool"
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
}

// AdvisorHook injects AdvisorContext before an advisor tool call.
// Exported for test compatibility in packages that alias this type.
type AdvisorHook struct{}

func (h AdvisorHook) Match(toolName string) bool {
	sourceID, toolNameOnly, ok := coretool.ParseNormalizedToolName(toolName)
	if !ok {
		return false
	}
	isAdvisorSource := sourceID == coretool.BuiltinAdvisorSourceID || sourceID == "builtin"
	return isAdvisorSource && toolNameOnly == coretool.BuiltinAdvisorToolName
}

func (h AdvisorHook) PrepareContext(deps HookDeps, ctx context.Context, messages []map[string]any) context.Context {
	if actx, ok := coretool.GetAdvisorContext(ctx); ok {
		actx.Messages = messages
		return ctx
	}

	maxUses := deps.GetAdvisorMaxUses()
	if maxUses <= 0 {
		maxUses = 3
	}

	return coretool.WithAdvisorContext(ctx, &coretool.AdvisorContext{
		Messages:      messages,
		UsesRemaining: &maxUses,
	})
}

// applyHooks runs all matching hooks for the given tool name and returns the updated context.
func applyHooks(ctx context.Context, toolName string, messages []map[string]any, deps HookDeps, hooks []Hook) context.Context {
	for _, hook := range hooks {
		if hook.Match(toolName) {
			// Increment depth to prevent advisor recursion.
			depth := coretool.GetAdvisorDepth(ctx)
			ctx = coretool.WithAdvisorDepth(ctx, depth+1)
			ctx = hook.PrepareContext(deps, ctx, messages)
		}
	}
	return ctx
}
