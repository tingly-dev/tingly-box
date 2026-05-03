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
	CallMCPToolWithHooks(ctx context.Context, toolName, arguments string, messages []map[string]any) (context.Context, string, error)
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
	_, result, err := e.ExecuteToolWithContext(ctx, tool, messages)
	return result, err
}

func (e *ServerToolExecutor) ExecuteToolWithContext(
	ctx context.Context,
	tool Tool,
	messages []map[string]any,
) (context.Context, ToolExecutionResult, error) {
	nextCtx, result, err := e.server.CallMCPToolWithHooks(
		ctx,
		tool.Name(),
		tool.Arguments(),
		messages,
	)
	if nextCtx == nil {
		nextCtx = ctx
	}

	toolResult := ToolExecutionResult{
		ToolUseID: tool.ID(),
		Content:   result,
		IsError:   err != nil,
	}
	return nextCtx, toolResult, err
}

func (e *ServerToolExecutor) ExecuteTools(
	ctx context.Context,
	tools []Tool,
	messages []map[string]any,
) ([]ToolExecutionResult, error) {
	results := make([]ToolExecutionResult, len(tools))

	for i, tool := range tools {
		nextCtx, result, err := e.ExecuteToolWithContext(ctx, tool, messages)
		ctx = nextCtx
		results[i] = result

		// Log errors but continue with other tools
		if err != nil {
			// Error is already captured in result.IsError
		}
	}

	return results, nil
}
