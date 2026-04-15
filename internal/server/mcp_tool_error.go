package server

import (
	"context"
	"encoding/json"
	"fmt"
)

func (s *Server) advisorMaxUses() int {
	if s == nil || s.mcpRuntime == nil {
		return 0
	}
	cfg := s.mcpRuntime.GetConfig()
	if cfg == nil {
		return 0
	}
	for _, source := range cfg.Sources {
		if source.Advisor != nil && source.Advisor.MaxUsesPerRequest > 0 {
			return source.Advisor.MaxUsesPerRequest
		}
	}
	return 0
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
