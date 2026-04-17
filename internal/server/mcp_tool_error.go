package server

import (
	"context"
	"encoding/json"
	"fmt"

	mcpruntime "github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	mcptools "github.com/tingly-dev/tingly-box/internal/mcp/tools"
)

// mcpResponseToolCallHook prepares runtime context for specific MCP servertool calls.
// Hooks run after worker response arrives and before local tool execution.
type mcpResponseToolCallHook interface {
	Match(toolName string) bool
	PrepareContext(s *Server, ctx context.Context, messages []map[string]any) context.Context
}

type advisorResponseHook struct{}

func (h advisorResponseHook) Match(toolName string) bool {
	sourceID, toolNameOnly, ok := mcpruntime.ParseNormalizedToolName(toolName)
	if !ok {
		return false
	}
	isAdvisorSource := sourceID == mcptools.BuiltinAdvisorSourceID || sourceID == "builtin"
	return isAdvisorSource && toolNameOnly == mcptools.BuiltinAdvisorToolName
}

func (h advisorResponseHook) PrepareContext(s *Server, ctx context.Context, messages []map[string]any) context.Context {
	return s.withAdvisorContext(ctx, messages)
}

func (s *Server) isEnabledMCPToolName(ctx context.Context, toolName string) bool {
	if s == nil || s.mcpRuntime == nil {
		return false
	}
	enabled := s.mcpRuntime.ListEnabledServerToolNames(ctx)
	_, ok := enabled[toolName]
	return ok
}

func disabledMCPToolErrorPayload(toolName string) string {
	payload, _ := json.Marshal(map[string]string{"error": "calling disabled tools: " + toolName})
	return string(payload)
}

func normalizeMCPToolCallError(err error) string {
	if err == nil {
		return ""
	}
	payload, _ := json.Marshal(map[string]string{"error": err.Error()})
	return string(payload)
}

func remapLegacyAdvisorToolName(toolName string) string {
	sourceID, toolNameOnly, ok := mcpruntime.ParseNormalizedToolName(toolName)
	if !ok {
		return toolName
	}
	if sourceID == mcptools.BuiltinAdvisorSourceID && toolNameOnly == mcptools.BuiltinAdvisorToolName {
		return mcpruntime.NormalizeToolName("builtin", mcptools.BuiltinAdvisorToolName)
	}
	return toolName
}

func (s *Server) callMCPToolWithGuard(ctx context.Context, toolName, arguments string) (string, error) {
	resolvedToolName := remapLegacyAdvisorToolName(toolName)
	if !s.isEnabledMCPToolName(ctx, resolvedToolName) {
		return disabledMCPToolErrorPayload(toolName), fmt.Errorf("calling disabled tools: %s", toolName)
	}

	result, err := s.mcpRuntime.CallTool(ctx, resolvedToolName, arguments)
	if err != nil {
		return normalizeMCPToolCallError(err), err
	}

	return result, nil
}

func (s *Server) mcpResponseToolCallHooks() []mcpResponseToolCallHook {
	return []mcpResponseToolCallHook{
		advisorResponseHook{},
	}
}

func (s *Server) withAdvisorContext(ctx context.Context, messages []map[string]any) context.Context {
	if actx, ok := mcpruntime.GetAdvisorContext(ctx); ok {
		actx.Messages = messages
		return mcpruntime.WithAdvisorContext(ctx, actx)
	}

	maxUses := 0
	if s != nil && s.mcpRuntime != nil {
		maxUses = s.mcpRuntime.GetAdvisorMaxUses()
	}
	if maxUses <= 0 {
		maxUses = 3
	}

	return mcpruntime.WithAdvisorContext(ctx, &mcpruntime.AdvisorContext{
		Messages:      messages,
		UsesRemaining: maxUses,
	})
}

func (s *Server) applyMCPResponseToolCallHooks(ctx context.Context, toolName string, messages []map[string]any) context.Context {
	for _, hook := range s.mcpResponseToolCallHooks() {
		if hook.Match(toolName) {
			// Increment depth to prevent adviser recursion
			depth := mcpruntime.GetAdvisorDepth(ctx)
			ctx = mcpruntime.WithAdvisorDepth(ctx, depth+1)
			ctx = hook.PrepareContext(s, ctx, messages)
		}
	}
	return ctx
}

// callMCPToolWithHooks executes response-phase MCP servertool hooks before the runtime call.
// Hooks run when we consume worker-returned tool calls.
func (s *Server) callMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error) {
	ctx = s.applyMCPResponseToolCallHooks(ctx, toolName, messages)
	return s.callMCPToolWithGuard(ctx, toolName, arguments)
}
