package smart_guide

import (
	tbanthropic "github.com/tingly-dev/tingly-box/internal/anthropic"
)

// BuildTools assembles the Smart Guide toolset for the ReAct engine.
//
// The set mirrors the previous agentscope registration: bash, get_status,
// change_workdir, native read/write/edit, and (when a SendFile callback is
// available) send_file.
func BuildTools(
	executor *ToolExecutor,
	chatID string,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	updateProjectFunc func(chatID string, projectPath string) error,
	toolCtx *ToolContext,
) []tbanthropic.Tool {
	tools := []tbanthropic.Tool{
		NewBashTool(executor, DefaultBashAllowlist),
		NewGetStatusTool(executor, chatID, getStatusFunc),
		NewChangeDirTool(executor, chatID, updateProjectFunc),
		NewReadFileTool(executor),
		NewWriteFileTool(executor),
		NewEditFileTool(executor),
	}

	if toolCtx != nil && toolCtx.SendFile != nil {
		tools = append(tools, NewSendFileTool(executor, toolCtx))
	}

	return tools
}
