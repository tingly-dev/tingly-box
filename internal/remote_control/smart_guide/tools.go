package smart_guide

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	extTools "github.com/tingly-dev/tingly-agentscope/extension/tools"
	"github.com/tingly-dev/tingly-agentscope/pkg/message"
	"github.com/tingly-dev/tingly-agentscope/pkg/tool"
	"github.com/tingly-dev/tingly-box/agentsec"
)

// ============================================================================
// Re-exports from agentsec for backward compatibility within this package
// ============================================================================

// ToolExecutor is re-exported from agentsec so the rest of the smart_guide
// package and its callers (agent.go, agent_smart_guide.go) can reference it
// without importing agentsec directly.
type ToolExecutor = agentsec.ToolExecutor

// ApprovalRequest is re-exported from agentsec.
type ApprovalRequest = agentsec.ApprovalRequest

// ApprovalCallback is re-exported from agentsec.
type ApprovalCallback = agentsec.ApprovalCallback

// AllowRule is re-exported from agentsec.
type AllowRule = agentsec.AllowRule

// PermissionRule is re-exported from agentsec.
type PermissionRule = agentsec.PermissionRule

// DefaultBashAllowlist re-exports agentsec.DefaultBashAllowlist.
var DefaultBashAllowlist = agentsec.DefaultBashAllowlist

// MergeAllowlists re-exports agentsec.MergeAllowlists.
var MergeAllowlists = agentsec.MergeAllowlists

// NewToolExecutor re-exports agentsec.NewToolExecutor.
var NewToolExecutor = agentsec.NewToolExecutor

// ============================================================================
// Tool Context
// ============================================================================

// ToolContext provides context for tool execution
type ToolContext struct {
	ChatID      string
	ProjectPath string
	SessionID   string

	// SendFile sends a local file to the user via the IM bot.
	// Injected by the bot layer; nil if file sending is not available.
	SendFile func(ctx context.Context, filePath, caption string) error

	// RequestApproval requests explicit user approval for sensitive operations
	// (e.g. sending files outside the project path). This callback must NOT be
	// bypassed by yolo mode — it is distinct from the bash approval callback.
	// Returns (false, nil) if denied. Returns (false, err) on failure.
	RequestApproval func(ctx context.Context, prompt string) (approved bool, err error)
}

// ============================================================================
// StatusInfo
// ============================================================================

// StatusInfo holds bot status information
type StatusInfo struct {
	CurrentAgent   string `json:"current_agent"`
	SessionID      string `json:"session_id"`
	ProjectPath    string `json:"project_path"`
	WorkingDir     string `json:"working_dir"`
	HasRunningTask bool   `json:"has_running_task"`
	Whitelisted    bool   `json:"whitelisted"`
}

// ============================================================================
// Unified Bash Tool
// ============================================================================

// BashParams defines the parameters for bash tool
type BashParams struct {
	Command string `json:"command" required:"true" jsonschema:"description=The bash command to execute (e.g., 'ls -la', 'git status')"`
}

// BashTool wraps the agentsec policy layer with SmartGuide-specific execution:
// the { cmd; pwd; } composite pattern that tracks the working directory.
type BashTool struct {
	Executor *ToolExecutor
	policy   *agentsec.BashPolicy
}

// NewBashTool creates a new bash tool wrapper
func NewBashTool(executor *ToolExecutor, allowlist []string) *BashTool {
	return &BashTool{
		Executor: executor,
		policy:   agentsec.NewBashPolicy(allowlist),
	}
}

// AllowedCommands returns the allowlist rules for display purposes.
// It delegates to the executor's allowlist map.
func (t *BashTool) AllowedCommands() []string {
	return t.Executor.GetAllowedCommands()
}

// Description returns the bash tool description
func (t *BashTool) Description() string {
	return `Execute bash commands for file system operations and git.

Allowed commands: ls, pwd, cat, mkdir, rm, cp, mv, git, curl, wget, and more.
Commands containing pipes (|), && chains, ; sequences, or $(...) substitutions
always require explicit user approval.

Examples:
- List files: ls -la
- Show current directory: pwd
- Clone repository: git clone https://github.com/user/repo.git
- Check git status: git status
- Change directory temporarily: cd /path/to/dir && ls`
}

// Name returns the bash tool name
func (t *BashTool) Name() string {
	return "bash"
}

// Call executes a bash command with SmartGuide-specific enhancements.
// Policy evaluation (allowlist check, pipe detection) is delegated to agentsec.BashPolicy.
func (t *BashTool) Call(ctx context.Context, params BashParams) (*tool.ToolResponse, error) {
	command := params.Command
	if command == "" {
		return tool.TextResponse("Error: 'command' parameter is required"), nil
	}

	decision := t.policy.Evaluate(command)

	switch {
	case decision == agentsec.PolicyAllow:
		return t.executeCommand(ctx, command)

	case decision.NeedsApproval():
		return t.requestApproval(ctx, command, decision.IsChained())

	default:
		return tool.TextResponse(fmt.Sprintf("Error: unexpected policy decision for command %q", command)), nil
	}
}

// executeCommand runs the command via the extension BashTool using the
// SmartGuide-specific { cmd; pwd; } composite to track working directory.
func (t *BashTool) executeCommand(ctx context.Context, command string) (*tool.ToolResponse, error) {
	oldDir := t.Executor.GetWorkingDirectory()
	cwd := oldDir

	// Build a composite command: { command; pwd; }
	// Using { } ensures cd affects the current shell context; pwd captures new dir.
	compositeCommand := fmt.Sprintf("{ %s; pwd; }", command)

	extBash := extTools.NewBashTool(
		extTools.BashOptions([]string{}, nil, 120*time.Second, cwd),
		extTools.BashAllowChaining(true),
	)

	result, err := extBash.Call(ctx, extTools.BashParams{Command: compositeCommand})
	if err != nil {
		return result, err
	}

	// Extract new working directory from the trailing pwd output
	newDir := t.extractDirectoryFromResult(result)
	if newDir != "" && newDir != oldDir {
		t.Executor.SetWorkingDirectory(newDir)
		logrus.WithFields(logrus.Fields{
			"oldDir": oldDir,
			"newDir": newDir,
		}).Info("BashTool: Working directory changed after command execution")
		cwd = newDir
	}

	// Annotate output with cwd, removing the trailing pwd line
	if result != nil && len(result.Content) > 0 {
		if textBlock, ok := result.Content[0].(*message.TextBlock); ok {
			displayText := t.removeTrailingPwd(textBlock.Text)
			result.Content[0] = message.Text(fmt.Sprintf("(cwd: %s)\n%s", cwd, displayText))
		}
	}

	return result, nil
}

// requestApproval sends the command for user approval via the executor callback.
// isChained is true for commands containing shell operators; in that case the
// approval result's Remember flag must be ignored (not stored in allowlist).
func (t *BashTool) requestApproval(ctx context.Context, command string, isChained bool) (*tool.ToolResponse, error) {
	baseCmd := agentsec.ExtractBaseCommand(command)

	if t.Executor.HasApprovalCallback() {
		logrus.WithFields(logrus.Fields{
			"command":   baseCmd,
			"full":      command,
			"isChained": isChained,
		}).Info("BashTool: Requesting approval for command")

		parts := strings.Fields(command)
		var cmd string
		var args []string
		if len(parts) > 0 {
			cmd = parts[0]
			args = parts[1:]
		}

		approved, err := t.Executor.CallApproval(ctx, agentsec.ApprovalRequest{
			Command:   cmd,
			Args:      args,
			Reason:    fmt.Sprintf("Command %q is not in the allowlist", baseCmd),
			IsChained: isChained,
		})

		logrus.WithFields(logrus.Fields{
			"command":   cmd,
			"approved":  approved,
			"isChained": isChained,
			"error":     err,
		}).Info("BashTool: Approval callback returned")

		if err != nil {
			logrus.WithError(err).WithField("command", cmd).Error("BashTool: Approval callback failed")
			return tool.TextResponse(fmt.Sprintf("Error: approval request failed: %v", err)), nil
		}
		if !approved {
			logrus.WithField("command", cmd).Warn("BashTool: Command was NOT approved by user")
			return tool.TextResponse(fmt.Sprintf("Error: command %q was not approved by user", baseCmd)), nil
		}
		logrus.WithField("command", baseCmd).Info("BashTool: Command approved, executing")
		return t.executeCommand(ctx, command)
	}

	// No approval callback — deny
	logrus.WithField("command", baseCmd).Warn("BashTool: Command not in allowlist and no approval callback set - denying")
	return tool.TextResponse(fmt.Sprintf("Error: command %q is not allowed. Allowed commands: %s",
		baseCmd, strings.Join(t.AllowedCommands(), ", "))), nil
}

// extractDirectoryFromResult reads the trailing pwd line from the composite command output.
func (t *BashTool) extractDirectoryFromResult(result *tool.ToolResponse) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	if textBlock, ok := result.Content[0].(*message.TextBlock); ok {
		text := strings.TrimRight(textBlock.Text, "\n\r\t ")
		lines := strings.Split(text, "\n")
		if len(lines) > 0 {
			potentialDir := strings.TrimSpace(lines[len(lines)-1])
			if filepath.IsAbs(potentialDir) {
				return potentialDir
			}
		}
	}
	return ""
}

// removeTrailingPwd strips the last line if it is an absolute path (the pwd output).
func (t *BashTool) removeTrailingPwd(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		if filepath.IsAbs(strings.TrimSpace(lines[len(lines)-1])) {
			return strings.Join(lines[:len(lines)-1], "\n")
		}
	}
	return text
}

// ============================================================================
// Get Status Tool
// ============================================================================

// GetStatusParams defines the parameters for get_status tool
type GetStatusParams struct {
	ChatID string `json:"chat_id,omitempty" jsonschema:"description=Chat ID to get status for"`
}

// GetStatusTool returns current bot status
type GetStatusTool struct {
	executor      *ToolExecutor
	getStatusFunc func(chatID string) (*StatusInfo, error)
}

// NewGetStatusTool creates a new GetStatusTool
func NewGetStatusTool(executor *ToolExecutor, getStatusFunc func(chatID string) (*StatusInfo, error)) *GetStatusTool {
	return &GetStatusTool{
		executor:      executor,
		getStatusFunc: getStatusFunc,
	}
}

// Description returns the get_status tool description
func (t *GetStatusTool) Description() string {
	return "Get the current bot status including agent, session, project path, and working directory."
}

// Name returns the get_status tool name
func (t *GetStatusTool) Name() string {
	return "get_status"
}

// Call returns the current bot status
func (t *GetStatusTool) Call(ctx context.Context, params GetStatusParams) (*tool.ToolResponse, error) {
	chatID := params.ChatID
	cwd := t.executor.GetWorkingDirectory()

	if t.getStatusFunc == nil {
		return tool.TextResponse(fmt.Sprintf("Current working directory: %s", cwd)), nil
	}

	status, err := t.getStatusFunc(chatID)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error getting status: %v", err)), nil
	}

	if status != nil {
		status.WorkingDir = cwd
	}

	response := fmt.Sprintf("**Current Status:**\n"+
		"• Agent: %s\n"+
		"• Session: %s\n"+
		"• Project: %s\n"+
		"• Working Directory: %s\n"+
		"• Whitelisted: %v",
		status.CurrentAgent,
		status.SessionID,
		status.ProjectPath,
		status.WorkingDir,
		status.Whitelisted,
	)

	return tool.TextResponse(response), nil
}

// ============================================================================
// Change Directory Tool
// ============================================================================

// ChangeDirParams defines the parameters for change_workdir tool
type ChangeDirParams struct {
	Path   string `json:"path" jsonschema:"description=The directory path to change to (absolute or relative to current directory)"`
	ChatID string `json:"chat_id,omitempty" jsonschema:"description=(internal) Chat ID for persistence"`
}

// ChangeDirTool changes the bound project directory
type ChangeDirTool struct {
	executor          *ToolExecutor
	chatID            string
	updateProjectFunc func(chatID string, projectPath string) error
}

// NewChangeDirTool creates a new ChangeDirTool
func NewChangeDirTool(executor *ToolExecutor, chatID string, updateProjectFunc func(chatID string, projectPath string) error) *ChangeDirTool {
	return &ChangeDirTool{
		executor:          executor,
		chatID:            chatID,
		updateProjectFunc: updateProjectFunc,
	}
}

// Description returns the change_workdir tool description
func (t *ChangeDirTool) Description() string {
	return "Change the bound project directory. This updates both the current working directory and the persisted project path."
}

// Name returns the change_workdir tool name
func (t *ChangeDirTool) Name() string {
	return "change_workdir"
}

// Call changes the working directory and persists the change
func (t *ChangeDirTool) Call(ctx context.Context, params ChangeDirParams) (*tool.ToolResponse, error) {
	path := params.Path
	if path == "" {
		return tool.TextResponse("Error: 'path' parameter is required"), nil
	}

	resolvedPath := t.executor.ResolvePath(path)

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: %v", err)), nil
	}
	if !info.IsDir() {
		return tool.TextResponse(fmt.Sprintf("Error: '%s' is not a directory", resolvedPath)), nil
	}

	t.executor.SetWorkingDirectory(resolvedPath)

	if t.updateProjectFunc != nil && t.chatID != "" {
		if err := t.updateProjectFunc(t.chatID, resolvedPath); err != nil {
			logrus.WithError(err).WithField("chatID", t.chatID).Warn("Failed to update project path in chat store")
			return tool.TextResponse(fmt.Sprintf("Warning: directory changed but persistence failed: %v\nNew directory: %s", err, resolvedPath)), nil
		}
	}

	lsCmd := exec.CommandContext(ctx, "ls", "-la")
	lsCmd.Dir = resolvedPath
	output, _ := lsCmd.CombinedOutput()

	return tool.TextResponse(fmt.Sprintf("✅ Changed directory to: %s\n\nDirectory contents:\n%s", resolvedPath, string(output))), nil
}

// ============================================================================
// Handoff Tool (hidden, reserved for future use)
// ============================================================================

// HandoffParams defines the parameters for handoff_to_cc tool
type HandoffParams struct{}

// HandoffToCCTool provides handoff to Claude Code
type HandoffToCCTool struct{}

// NewHandoffToCCTool creates a new handoff tool
func NewHandoffToCCTool() *HandoffToCCTool { return &HandoffToCCTool{} }

// Name returns the tool name
func (t *HandoffToCCTool) Name() string { return "handoff_to_cc" }

// Description returns the tool description
func (t *HandoffToCCTool) Description() string {
	return "Hand off control to Claude Code (@cc) for coding tasks. Use this when the user is ready to start coding."
}

// Call implements the tool interface
func (t *HandoffToCCTool) Call(ctx context.Context, params HandoffParams) (*tool.ToolResponse, error) {
	return tool.TextResponse("HANDOFF_TO_CC"), nil
}

// ============================================================================
// Tool Registration
// ============================================================================

// RegisterTools registers all smart guide tools with a toolkit
func RegisterTools(
	toolkit *tool.Toolkit, executor *ToolExecutor, chatID string,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	updateProjectFunc func(chatID string, projectPath string) error,
	toolCtx *ToolContext,
	bashAllowlist []string,
) error {

	if err := toolkit.CreateToolGroup("bash", "Bash commands for file system and git operations", true, ""); err != nil {
		return fmt.Errorf("failed to create bash tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("project", "Project and directory management tools", true, ""); err != nil {
		return fmt.Errorf("failed to create project tool group: %w", err)
	}
	if err := toolkit.CreateToolGroup("file_ops", "File reading, writing, and editing tools", true, ""); err != nil {
		return fmt.Errorf("failed to create file_ops tool group: %w", err)
	}

	bashTool := NewBashTool(executor, bashAllowlist)
	if err := toolkit.RegisterAll(bashTool); err != nil {
		return fmt.Errorf("failed to register bash tool: %w", err)
	}

	getStatusTool := NewGetStatusTool(executor, getStatusFunc)
	if err := toolkit.RegisterAll(getStatusTool); err != nil {
		return fmt.Errorf("failed to register get_status tool: %w", err)
	}

	changeDirTool := NewChangeDirTool(executor, chatID, updateProjectFunc)
	if err := toolkit.RegisterAll(changeDirTool); err != nil {
		return fmt.Errorf("failed to register change_workdir tool: %w", err)
	}

	if err := extTools.RegisterReadTool(toolkit, extTools.ReadOptions(nil, 10*1024*1024)); err != nil {
		return fmt.Errorf("failed to register read tool: %w", err)
	}
	if err := extTools.RegisterWriteTool(toolkit, extTools.WriteOptions(nil, true), extTools.WriteMaxSize(10*1024*1024)); err != nil {
		return fmt.Errorf("failed to register write tool: %w", err)
	}
	if err := extTools.RegisterEditTool(toolkit, extTools.EditOptions(nil)); err != nil {
		return fmt.Errorf("failed to register edit tool: %w", err)
	}

	if toolCtx != nil && toolCtx.SendFile != nil {
		sendFileTool := NewSendFileTool(executor, toolCtx)
		if err := toolkit.RegisterAll(sendFileTool); err != nil {
			return fmt.Errorf("failed to register send_file tool: %w", err)
		}
	}

	logrus.Info("Smart guide tools registered successfully")
	return nil
}
