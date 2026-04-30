package smart_guide

import (
	"fmt"

	"github.com/sirupsen/logrus"
	extTools "github.com/tingly-dev/tingly-agentscope/extension/tools"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
)

// Tool Registration
// ============================================================================

// RegisterTools registers all smart guide tools with a toolkit
func RegisterTools(
	toolkit *tool.Toolkit, executor *ToolExecutor, chatID string,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	updateProjectFunc func(chatID string, projectPath string) error,
	toolCtx *ToolContext,
) error {

	// Create tool groups
	if err := toolkit.CreateToolGroup("bash", "Bash commands for file system and git operations", true, ""); err != nil {
		return fmt.Errorf("failed to create bash tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("project", "Project and directory management tools", true, ""); err != nil {
		return fmt.Errorf("failed to create project tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("file_ops", "File reading, writing, and editing tools", true, ""); err != nil {
		return fmt.Errorf("failed to create file_ops tool group: %w", err)
	}

	// Register bash tool
	bashTool := NewBashTool(executor, DefaultBashAllowlist)
	if err := toolkit.RegisterAll(bashTool); err != nil {
		return fmt.Errorf("failed to register bash tool: %w", err)
	}

	// Register get_status tool
	getStatusTool := NewGetStatusTool(executor, getStatusFunc)
	if err := toolkit.RegisterAll(getStatusTool); err != nil {
		return fmt.Errorf("failed to register get_status tool: %w", err)
	}

	// Register change_workdir tool
	changeDirTool := NewChangeDirTool(executor, chatID, updateProjectFunc)
	if err := toolkit.RegisterAll(changeDirTool); err != nil {
		return fmt.Errorf("failed to register change_workdir tool: %w", err)
	}

	// Register read tool (from extension)
	if err := extTools.RegisterReadTool(toolkit,
		extTools.ReadOptions(nil, 10*1024*1024)); err != nil {
		return fmt.Errorf("failed to register read tool: %w", err)
	}

	// Register write tool (from extension)
	if err := extTools.RegisterWriteTool(toolkit,
		extTools.WriteOptions(nil, true),
		extTools.WriteMaxSize(10*1024*1024)); err != nil {
		return fmt.Errorf("failed to register write tool: %w", err)
	}

	// Register edit tool (from extension)
	if err := extTools.RegisterEditTool(toolkit,
		extTools.EditOptions(nil)); err != nil {
		return fmt.Errorf("failed to register edit tool: %w", err)
	}

	// Register send_file tool (if SendFile callback is available)
	if toolCtx != nil && toolCtx.SendFile != nil {
		sendFileTool := NewSendFileTool(executor, toolCtx)
		if err := toolkit.RegisterAll(sendFileTool); err != nil {
			return fmt.Errorf("failed to register send_file tool: %w", err)
		}
	}

	// Note: handoff_to_cc is not registered for now

	logrus.Info("Smart guide tools registered successfully (all tools now use standard pattern)")
	return nil
}
