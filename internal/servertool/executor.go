package servertool

import (
	"context"
	"encoding/json"
	"fmt"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
)

// ToolCall describes a single MCP tool invocation.
type ToolCall struct {
	NormalizedName string
	Arguments      string
	Messages       []map[string]any
}

// RuntimeCaller is the subset of mcpruntime.Runtime used by DefaultExecutor.
type RuntimeCaller interface {
	CallTool(ctx context.Context, normalizedName, arguments string) (mcpruntime.ToolResult, error)
	ListCallableServerToolNames(ctx context.Context) map[string]struct{}
	GetAdvisorMaxUses() int
}

// Executor executes a single MCP tool call through the full pipeline:
// name validation, legacy remapping, callable guard, hooks, and dispatch.
type Executor interface {
	Execute(ctx context.Context, call ToolCall) (context.Context, mcpruntime.ToolResult, error)
}

// DefaultExecutor is the standard implementation of Executor.
type DefaultExecutor struct {
	runtime RuntimeCaller
	deps    HookDeps
	hooks   []Hook
}

// NewDefaultExecutor creates a DefaultExecutor with the default hook set.
func NewDefaultExecutor(rt RuntimeCaller, deps HookDeps) *DefaultExecutor {
	return &DefaultExecutor{
		runtime: rt,
		deps:    deps,
		hooks:   defaultHooks(),
	}
}

// Execute runs the full tool-call pipeline and returns the updated context, result, and error.
func (e *DefaultExecutor) Execute(ctx context.Context, call ToolCall) (context.Context, mcpruntime.ToolResult, error) {
	toolName := call.NormalizedName

	// 1. Validate that the name looks like an MCP tool name.
	if !IsMCPToolName(toolName) {
		return ctx,
			disabledToolErrorPayload(toolName),
			fmt.Errorf("non-MCP tool routed to MCP executor: %s", toolName)
	}

	// 2. Remap legacy source IDs (e.g. "advisor" → "builtin").
	resolvedName := RemapLegacyAdvisorToolName(toolName)

	// 3. Guard: reject disabled / uncallable tools.
	if !e.isCallable(ctx, resolvedName) {
		return ctx,
			disabledToolErrorPayload(toolName),
			fmt.Errorf("calling disabled tools: %s", toolName)
	}

	// 4. Apply pre-call hooks (depth increment, advisor context, scenario sink).
	prevDepth := mcpruntime.GetAdvisorDepth(ctx)
	ctx = applyHooks(ctx, toolName, call.Messages, e.deps, e.hooks)

	// 5. Dispatch to the runtime.
	result, err := e.runtime.CallTool(ctx, resolvedName, call.Arguments)
	if err != nil {
		result = normalizeError(err)
	}

	// 6. Restore depth so it doesn't accumulate across sequential tool calls.
	ctx = mcpruntime.WithAdvisorDepth(ctx, prevDepth)

	return ctx, result, err
}

func (e *DefaultExecutor) isCallable(ctx context.Context, toolName string) bool {
	if e.runtime == nil {
		return false
	}
	callable := e.runtime.ListCallableServerToolNames(ctx)
	_, ok := callable[toolName]
	return ok
}

func disabledToolErrorPayload(toolName string) mcpruntime.ToolResult {
	payload, _ := json.Marshal(map[string]string{"error": "calling disabled tools: " + toolName})
	return mcpruntime.ErrorToolResult(string(payload))
}

func normalizeError(err error) mcpruntime.ToolResult {
	if err == nil {
		return mcpruntime.ToolResult{}
	}
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	return mcpruntime.ErrorToolResult(string(payload))
}
