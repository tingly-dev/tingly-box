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
)

// ============================================================================
// Tool Context & Executor
// ============================================================================

// ToolContext provides context for tool execution
type ToolContext struct {
	ChatID      string
	ProjectPath string
	SessionID   string
}

// ApprovalRequest represents a request for user approval
type ApprovalRequest struct {
	Command string   // Command to execute
	Args    []string // Command arguments
	Reason  string   // Reason for approval request
}

// ApprovalCallback is called when a command requires user approval
// Returns (approved, error) - if error is non-nil, the approval process failed
type ApprovalCallback func(ctx context.Context, req ApprovalRequest) (approved bool, err error)

// ToolExecutor handles tool execution with proper context
type ToolExecutor struct {
	BashAllowlist   map[string]struct{}
	BashCwd         string           // Per-execution working directory
	onApproval      ApprovalCallback // Approval callback for non-allowlisted commands
	approvalTimeout time.Duration    // Timeout for approval requests
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(allowlist []string) *ToolExecutor {
	allowlistMap := make(map[string]struct{})
	for _, cmd := range allowlist {
		allowlistMap[strings.ToLower(cmd)] = struct{}{}
	}

	return &ToolExecutor{
		BashAllowlist:   allowlistMap,
		BashCwd:         "",              // Start in current directory
		approvalTimeout: 5 * time.Minute, // Default 5 minute timeout
	}
}

// SetApprovalCallback sets the approval callback for non-allowlisted commands
func (e *ToolExecutor) SetApprovalCallback(callback ApprovalCallback) {
	e.onApproval = callback
}

// SetApprovalTimeout sets the timeout for approval requests
func (e *ToolExecutor) SetApprovalTimeout(timeout time.Duration) {
	e.approvalTimeout = timeout
}

// SetWorkingDirectory sets the current working directory
func (e *ToolExecutor) SetWorkingDirectory(cwd string) {
	e.BashCwd = cwd
}

// GetWorkingDirectory returns the current working directory
func (e *ToolExecutor) GetWorkingDirectory() string {
	if e.BashCwd == "" {
		return "" // Return empty string if not explicitly set
	}
	return e.BashCwd
}

// ResolvePath resolves a path to an absolute path
// If the path is relative, it's joined with the current working directory
func (e *ToolExecutor) ResolvePath(path string) string {
	if !filepath.IsAbs(path) {
		currentDir := e.GetWorkingDirectory()
		if currentDir == "" {
			// If no working directory is set, use os.Getwd() as a fallback for resolution
			if wd, err := os.Getwd(); err == nil {
				currentDir = wd
			} else {
				currentDir = "/" // Fallback to root if os.Getwd fails
			}
		}
		return filepath.Join(currentDir, path)
	}
	return path
}

// ExecuteBash executes a bash command with allowlist checking
func (e *ToolExecutor) ExecuteBash(ctx context.Context, cmd string, args ...string) (string, error) {
	// Check if command is in allowlist
	cmdLower := strings.ToLower(cmd)
	if _, allowed := e.BashAllowlist[cmdLower]; !allowed {
		// Command not in allowlist - request approval if callback is available
		if e.onApproval != nil {
			logrus.WithFields(logrus.Fields{
				"command": cmd,
				"args":    args,
			}).Info("Command not in allowlist, requesting approval")

			approved, err := e.onApproval(ctx, ApprovalRequest{
				Command: cmd,
				Args:    args,
				Reason:  fmt.Sprintf("Command '%s' is not in the allowlist", cmd),
			})
			if err != nil {
				return "", fmt.Errorf("approval request failed: %w", err)
			}
			if !approved {
				return "", fmt.Errorf("command '%s' was not approved by user", cmd)
			}
			logrus.WithField("command", cmd).Info("Command approved by user")
		} else {
			// No approval callback available - deny with error
			return "", fmt.Errorf("command '%s' is not allowed. Allowed commands: %v",
				cmd, e.GetAllowedCommands())
		}
	}

	// Build full command
	fullCmd := append([]string{cmd}, args...)

	// Create command execution
	execCmd := exec.CommandContext(ctx, fullCmd[0], fullCmd[1:]...)

	// Set working directory
	if e.BashCwd != "" {
		execCmd.Dir = e.BashCwd
	}

	// Execute and capture output
	output, err := execCmd.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("command failed: %w", err)
	}

	return string(output), nil
}

// GetAllowedCommands returns the list of allowed commands
func (e *ToolExecutor) GetAllowedCommands() []string {
	cmds := make([]string, 0, len(e.BashAllowlist))
	for cmd := range e.BashAllowlist {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// ============================================================================
// Default Configuration
// ============================================================================

// DefaultBashAllowlist defines the default allowed bash commands
// These are commands useful for preparation/guide work
var DefaultBashAllowlist = []string{
	// File navigation
	"ls", "pwd", "cd", "cat", "tree",
	// File operations
	"mkdir", "rm", "cp", "mv", "touch", "chmod",
	// Git operations
	"git",
	// Network (for setup)
	"curl", "wget",
	// Utility
	"echo", "which", "env", "head", "tail", "wc", "find", "grep",
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
	Command string `json:"command" jsonschema:"description=The bash command to execute (e.g., 'ls -la', 'git status')"`
}

// BashTool wraps extension's BashTool with Smart Guide specific behavior
type BashTool struct {
	tool.DescriptiveTool
	Executor        *ToolExecutor
	AllowedCommands []string
}

// NewBashTool creates a new bash tool wrapper
func NewBashTool(executor *ToolExecutor, allowlist []string) *BashTool {
	return &BashTool{
		Executor:        executor,
		AllowedCommands: allowlist,
	}
}

// Description returns the bash tool description
func (t *BashTool) Description() string {
	return `Execute bash commands for file system operations and git.

Allowed commands: ls, pwd, cat, mkdir, rm, cp, mv, git, curl, wget, and more.

Supports command chaining with &&, ||, |, ;, etc.

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

// Call executes a bash command with Smart Guide specific enhancements
func (t *BashTool) Call(ctx context.Context, params BashParams) (*tool.ToolResponse, error) {
	command := params.Command
	if command == "" {
		return tool.TextResponse("Error: 'command' parameter is required"), nil
	}

	// Extract base command for allowlist checking
	baseCmd := t.extractBaseCommand(command)

	// Check if command is in allowlist
	// Note: isCommandAllowed returns true when command should be BLOCKED (not in allowlist)
	if !t.isCommandAllowed(baseCmd) {
		// Command is in allowlist - execute directly
		return t.executeCommand(ctx, command, false)
	}

	// Command is NOT in allowlist - request approval
	if t.Executor.onApproval != nil {
		logrus.WithFields(logrus.Fields{
			"command": baseCmd,
			"full":    command,
		}).Info("Command not in allowlist, requesting approval")

		// Parse command into base command and args
		parts := strings.Fields(command)
		var cmd string
		var args []string
		if len(parts) > 0 {
			cmd = parts[0]
			args = parts[1:]
		}

		approved, err := t.Executor.onApproval(ctx, ApprovalRequest{
			Command: cmd,
			Args:    args,
			Reason:  fmt.Sprintf("Command '%s' is not in the allowlist", baseCmd),
		})
		if err != nil {
			return tool.TextResponse(fmt.Sprintf("Error: approval request failed: %v", err)), nil
		}
		if !approved {
			return tool.TextResponse(fmt.Sprintf("Error: command '%s' was not approved by user", baseCmd)), nil
		}
		logrus.WithField("command", baseCmd).Info("Command approved by user")
		// Execute approved command without allowlist restriction
		return t.executeCommand(ctx, command, true)
	}

	// No approval callback - deny with error
	allowedList := strings.Join(t.AllowedCommands, ", ")
	return tool.TextResponse(fmt.Sprintf("Error: command '%s' is not allowed. Allowed commands: %s", baseCmd, allowedList)), nil
}

// executeCommand executes a bash command using the extension tool
// If skipAllowlist is true, the command is executed without allowlist restriction
func (t *BashTool) executeCommand(ctx context.Context, command string, skipAllowlist bool) (*tool.ToolResponse, error) {
	// Create extension bash tool with current working directory
	cwd := t.Executor.GetWorkingDirectory()

	// For approved commands not in allowlist, use empty allowlist to allow execution
	allowedCommands := t.AllowedCommands
	if skipAllowlist {
		allowedCommands = []string{} // Empty allowlist means allow all
	}

	extBash := extTools.NewBashTool(
		extTools.BashOptions(allowedCommands, nil, 120*time.Second, cwd),
		extTools.BashAllowChaining(true), // Allow command chaining
	)

	// Execute using extension tool
	result, err := extBash.Bash(ctx, extTools.BashParams{Command: command})
	if err != nil {
		return result, err
	}

	// Add working directory context to response
	if result != nil && len(result.Content) > 0 {
		// Extract text from content block and prepend cwd
		if textBlock, ok := result.Content[0].(*message.TextBlock); ok {
			result.Content[0] = message.Text(fmt.Sprintf("(cwd: %s)\n%s", cwd, textBlock.Text))
		}
	}

	return result, nil
}

// isCommandAllowed checks if a command is in the allowlist
func (t *BashTool) isCommandAllowed(baseCmd string) bool {
	if len(t.AllowedCommands) == 0 {
		return false // Empty allowlist means allow all
	}
	for _, cmd := range t.AllowedCommands {
		if strings.ToLower(cmd) == strings.ToLower(baseCmd) {
			return false // Command is allowed
		}
	}
	return true // Command not found in allowlist
}

// extractBaseCommand extracts the base command name from a command string
func (t *BashTool) extractBaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	for i, r := range trimmed {
		if r == ' ' || r == '\t' {
			return strings.ToLower(trimmed[:i])
		}
	}
	return strings.ToLower(trimmed)
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
	tool.DescriptiveTool
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

	// Add current working directory from executor
	cwd := t.executor.GetWorkingDirectory()

	if t.getStatusFunc == nil {
		return tool.TextResponse(fmt.Sprintf("Current working directory: %s", cwd)), nil
	}

	status, err := t.getStatusFunc(chatID)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error getting status: %v", err)), nil
	}

	// Override working directory with executor's current directory
	if status != nil {
		status.WorkingDir = cwd
	}

	// Format status response
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
	tool.DescriptiveTool
	executor          *ToolExecutor
	updateProjectFunc func(chatID string, projectPath string) error
}

// NewChangeDirTool creates a new ChangeDirTool
func NewChangeDirTool(executor *ToolExecutor, updateProjectFunc func(chatID string, projectPath string) error) *ChangeDirTool {
	return &ChangeDirTool{
		executor:          executor,
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
	chatID := params.ChatID

	if path == "" {
		return tool.TextResponse("Error: 'path' parameter is required"), nil
	}

	// Resolve path (handle relative paths)
	resolvedPath := t.executor.ResolvePath(path)

	// Check if directory exists
	info, err := os.Stat(resolvedPath)
	if err != nil {
		return tool.TextResponse(fmt.Sprintf("Error: %v", err)), nil
	}
	if !info.IsDir() {
		return tool.TextResponse(fmt.Sprintf("Error: '%s' is not a directory", resolvedPath)), nil
	}

	// Update working directory in executor
	t.executor.SetWorkingDirectory(resolvedPath)

	// Persist to chat store
	if t.updateProjectFunc != nil && chatID != "" {
		if err := t.updateProjectFunc(chatID, resolvedPath); err != nil {
			logrus.WithError(err).WithField("chatID", chatID).Warn("Failed to update project path in chat store")
			return tool.TextResponse(fmt.Sprintf("Warning: directory changed but persistence failed: %v\nNew directory: %s", err, resolvedPath)), nil
		}
	}

	// List directory contents to show user where they are
	lsCmd := exec.CommandContext(ctx, "ls", "-la")
	lsCmd.Dir = resolvedPath
	output, _ := lsCmd.CombinedOutput()

	response := fmt.Sprintf("✅ Changed directory to: %s\n\nDirectory contents:\n%s", resolvedPath, string(output))
	return tool.TextResponse(response), nil
}

// ============================================================================
// Handoff Tool (Hidden for now)
// ============================================================================

// HandoffToCCTool provides handoff to Claude Code
// Note: Currently not registered, kept for future use
type HandoffToCCTool struct {
	tool.DescriptiveTool
}

// NewHandoffToCCTool creates a new handoff tool
func NewHandoffToCCTool() *HandoffToCCTool {
	return &HandoffToCCTool{}
}

// Name returns the tool name
func (t *HandoffToCCTool) Name() string {
	return "handoff_to_cc"
}

// Description returns the tool description
func (t *HandoffToCCTool) Description() string {
	return "Hand off control to Claude Code (@cc) for coding tasks. Use this when the user is ready to start coding."
}

// Parameters returns the tool parameters schema
func (t *HandoffToCCTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

// Call implements the tool interface
func (t *HandoffToCCTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	return tool.TextResponse("HANDOFF_TO_CC"), nil
}

// ============================================================================
// Tool Registration
// ============================================================================

// ToolWithSchema is an interface for tools that can provide their own schema
type ToolWithSchema interface {
	tool.DescriptiveTool
	Name() string
	Description() string
	Parameters() map[string]any
}

// RegisterTools registers all smart guide tools with a toolkit
func RegisterTools(toolkit *tool.Toolkit, executor *ToolExecutor,
	getStatusFunc func(chatID string) (*StatusInfo, error),
	updateProjectFunc func(chatID string, projectPath string) error) error {

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

	// Register bash tool (now uses standard pattern with extension wrapper)
	bashTool := NewBashTool(executor, DefaultBashAllowlist)
	if err := toolkit.Register(bashTool.Call, &tool.RegisterOptions{
		GroupName:       "bash",
		FuncName:        bashTool.Name(),
		FuncDescription: bashTool.Description(),
	}); err != nil {
		return fmt.Errorf("failed to register bash tool: %w", err)
	}

	// Register get_status tool (refactored to use standard pattern)
	getStatusTool := NewGetStatusTool(executor, getStatusFunc)
	if err := toolkit.Register(getStatusTool.Call, &tool.RegisterOptions{
		GroupName:       "project",
		FuncName:        getStatusTool.Name(),
		FuncDescription: getStatusTool.Description(),
	}); err != nil {
		return fmt.Errorf("failed to register get_status tool: %w", err)
	}

	// Register change_workdir tool (refactored to use standard pattern)
	changeDirTool := NewChangeDirTool(executor, updateProjectFunc)
	if err := toolkit.Register(changeDirTool.Call, &tool.RegisterOptions{
		GroupName:       "project",
		FuncName:        changeDirTool.Name(),
		FuncDescription: changeDirTool.Description(),
	}); err != nil {
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

	// Note: handoff_to_cc is not registered for now

	logrus.Info("Smart guide tools registered successfully (all tools now use standard pattern)")
	return nil
}
