package server

import (
	"context"
	"fmt"
)

func (s *Server) isEnabledMCPToolName(ctx context.Context, toolName string) bool {
	if s == nil || s.mcpRuntime == nil {
		return false
	}
	enabled := s.mcpRuntime.ListEnabledServerToolNames(ctx)
	_, ok := enabled[toolName]
	return ok
}

func disabledMCPToolErrorPayload(toolName string) string {
	return fmt.Sprintf(`{"error":"calling disabled tools: %s"}`, toolName)
}

func normalizeMCPToolCallError(err error) string {
	if err == nil {
		return ""
	}
	return fmt.Sprintf(`{"error":"%s"}`, err.Error())
}

func (s *Server) callMCPToolWithGuard(ctx context.Context, toolName, arguments string) (string, error) {
	if !s.isEnabledMCPToolName(ctx, toolName) {
		return disabledMCPToolErrorPayload(toolName), fmt.Errorf("calling disabled tools: %s", toolName)
	}

	result, err := s.mcpRuntime.CallTool(ctx, toolName, arguments)
	if err != nil {
		return normalizeMCPToolCallError(err), err
	}

	return result, nil
}
