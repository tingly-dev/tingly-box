package mcp

import (
	"context"
)

// ServerToolExecutor implements ToolExecutor by wrapping server's MCP tool execution
type ServerToolExecutor struct {
	server ToolExecutorServer
}

// ToolExecutorServer defines the server methods needed for tool execution
type ToolExecutorServer interface {
	CallMCPTool(ctx context.Context, toolName, arguments string, messages []map[string]any) (string, error)
}

func NewServerToolExecutor(s ToolExecutorServer) *ServerToolExecutor {
	return &ServerToolExecutor{
		server: s,
	}
}

func (e *ServerToolExecutor) ExecuteTool(
	ctx context.Context,
	tool Tool,
	messages []map[string]any,
) (ToolExecutionResult, error) {
	result, err := e.server.CallMCPTool(
		ctx,
		tool.Name(),
		tool.Arguments(),
		messages,
	)

	return ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   result,
		IsError:   err != nil,
	}, err
}

func (e *ServerToolExecutor) ExecuteTools(
	ctx context.Context,
	tools []Tool,
	messages []map[string]any,
) ([]ToolExecutionResult, error) {
	results := make([]ToolExecutionResult, len(tools))

	for i, tool := range tools {
		result, err := e.ExecuteTool(ctx, tool, messages)
		results[i] = result

		// Log errors but continue with other tools
		if err != nil {
			// Error is already captured in result.IsError
		}
	}

	return results, nil
}
