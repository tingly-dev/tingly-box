package smartguide

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-agentscope/pkg/model"
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

// ToolExecutor handles tool execution with proper context
type ToolExecutor struct {
	BashAllowlist map[string]struct{}
	BashCwd       string // Per-execution working directory
}

// NewToolExecutor creates a new tool executor
func NewToolExecutor(allowlist []string) *ToolExecutor {
	allowlistMap := make(map[string]struct{})
	for _, cmd := range allowlist {
		allowlistMap[strings.ToLower(cmd)] = struct{}{}
	}

	return &ToolExecutor{
		BashAllowlist: allowlistMap,
		BashCwd:       "", // Start in current directory
	}
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
	// Check if command is allowed
	cmdLower := strings.ToLower(cmd)
	if _, allowed := e.BashAllowlist[cmdLower]; !allowed {
		return "", fmt.Errorf("command '%s' is not allowed. Allowed commands: %v",
			cmd, e.GetAllowedCommands())
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

// BashTool is a unified bash execution tool
type BashTool struct {
	Executor        *ToolExecutor
	AllowedCommands map[string]struct{}
}

// NewBashTool creates a new unified bash tool
func NewBashTool(executor *ToolExecutor, allowlist []string) *BashTool {
	allowed := make(map[string]struct{})
	for _, cmd := range allowlist {
		allowed[strings.ToLower(cmd)] = struct{}{}
	}
	return &BashTool{
		Executor:        executor,
		AllowedCommands: allowed,
	}
}

// Name returns the tool name
func (t *BashTool) Name() string {
	return "bash"
}

// Description returns the tool description
func (t *BashTool) Description() string {
	return `Execute bash commands for file system operations and git.

Allowed commands: ls, pwd, cat, mkdir, rm, cp, mv, git, curl, wget, and more.

Note: Use 'change_workdir' tool instead of 'cd' to change working directory.

Examples:
- List files: ls -la
- Show current directory: pwd
- Clone repository: git clone https://github.com/user/repo.git
- Check git status: git status`
}

// Parameters returns the tool parameters schema
func (t *BashTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"command": map[string]any{
				"type":        "string",
				"description": "The bash command to execute (e.g., 'ls -la', 'cd /path')",
			},
		},
		"required": []string{"command"},
	}
}

// Call executes the bash command
func (t *BashTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	command, ok := kwargs["command"].(string)
	if !ok || command == "" {
		return tool.TextResponse("Error: 'command' parameter is required"), nil
	}

	// Parse the command to get the base command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return tool.TextResponse("Error: empty command"), nil
	}

	baseCmd := strings.ToLower(parts[0])

	// Check if command is allowed
	if _, allowed := t.AllowedCommands[baseCmd]; !allowed {
		allowedList := make([]string, 0, len(t.AllowedCommands))
		for cmd := range t.AllowedCommands {
			allowedList = append(allowedList, cmd)
		}
		return tool.TextResponse(fmt.Sprintf("Error: command '%s' is not allowed. Allowed commands: %s",
			baseCmd, strings.Join(allowedList, ", "))), nil
	}

	// cd is not allowed - use change_workdir tool instead
	if baseCmd == "cd" {
		return tool.TextResponse("Error: 'cd' is not available in bash. Use the 'change_workdir' tool to change working directory."), nil
	}

	// Execute command
	return t.executeCommand(ctx, command)
}

// executeCommand executes a general bash command
func (t *BashTool) executeCommand(ctx context.Context, command string) (*tool.ToolResponse, error) {
	// Parse command
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return tool.TextResponse("Error: empty command"), nil
	}

	cmdName := parts[0]
	args := parts[1:]

	// Create command
	cmd := exec.CommandContext(ctx, cmdName, args...)
	cmd.Dir = t.Executor.GetWorkingDirectory()

	// Execute and capture output
	output, err := cmd.CombinedOutput()

	result := string(output)
	if err != nil {
		// Include error in output
		result = fmt.Sprintf("%s\nError: %v", result, err)
	}

	// Add working directory context
	cwd := t.Executor.GetWorkingDirectory()
	if result != "" {
		result = fmt.Sprintf("(cwd: %s)\n%s", cwd, result)
	}

	return tool.TextResponse(result), nil
}

// ============================================================================
// Get Status Tool
// ============================================================================

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

// Name returns the tool name
func (t *GetStatusTool) Name() string {
	return "get_status"
}

// Description returns the tool description
func (t *GetStatusTool) Description() string {
	return "Get the current bot status including agent, session, project path, and working directory."
}

// Parameters returns the tool parameters schema
func (t *GetStatusTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"chat_id": map[string]any{
				"type":        "string",
				"description": "Chat ID to get status for",
			},
		},
		"required": []string{},
	}
}

// Call implements the tool interface
func (t *GetStatusTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	chatID, _ := kwargs["chat_id"].(string)

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

// ChangeDirTool changes the bound project directory
type ChangeDirTool struct {
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

// Name returns the tool name
func (t *ChangeDirTool) Name() string {
	return "change_workdir"
}

// Description returns the tool description
func (t *ChangeDirTool) Description() string {
	return `Change the bound project directory. This updates both the current working directory and the persisted project path.

Use this when:
- User wants to switch to a different project
- User provides a new path to work on
- Setting up initial project location`
}

// Parameters returns the tool parameters schema
func (t *ChangeDirTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"path": map[string]any{
				"type":        "string",
				"description": "The directory path to change to (absolute or relative to current directory)",
			},
			"chat_id": map[string]any{
				"type":        "string",
				"description": "(internal) Chat ID for persistence",
			},
		},
		"required": []string{"path"},
	}
}

// Call implements the tool interface
func (t *ChangeDirTool) Call(ctx context.Context, kwargs map[string]any) (*tool.ToolResponse, error) {
	path, ok := kwargs["path"].(string)
	if !ok || path == "" {
		return tool.TextResponse("Error: 'path' parameter is required"), nil
	}

	chatID, _ := kwargs["chat_id"].(string)

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
type HandoffToCCTool struct{}

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
	tool.ToolCallable
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

	// Register unified bash tool
	bashTool := NewBashTool(executor, DefaultBashAllowlist)
	if err := registerToolWithSchema(toolkit, bashTool, "bash"); err != nil {
		return fmt.Errorf("failed to register bash tool: %w", err)
	}

	// Register get_status tool (in project group for visibility)
	getStatusTool := NewGetStatusTool(executor, getStatusFunc)
	if err := registerToolWithSchema(toolkit, getStatusTool, "project"); err != nil {
		return fmt.Errorf("failed to register get_status tool: %w", err)
	}

	// Register change_workdir tool
	changeDirTool := NewChangeDirTool(executor, updateProjectFunc)
	if err := registerToolWithSchema(toolkit, changeDirTool, "project"); err != nil {
		return fmt.Errorf("failed to register change_workdir tool: %w", err)
	}

	// Note: handoff_to_cc is not registered for now

	logrus.Info("Smart guide tools registered successfully")
	return nil
}

// registerToolWithSchema registers a tool that implements ToolWithSchema interface
func registerToolWithSchema(toolkit *tool.Toolkit, t ToolWithSchema, groupName string) error {
	return toolkit.Register(t, &tool.RegisterOptions{
		GroupName: groupName,
		JSONSchema: &model.ToolDefinition{
			Type: "function",
			Function: model.FunctionDefinition{
				Name:        t.Name(),
				Description: t.Description(),
				Parameters:  t.Parameters(),
			},
		},
	})
}
