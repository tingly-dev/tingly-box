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

// DefaultBashAllowlist defines the default allowed bash commands
var DefaultBashAllowlist = []string{
	"ls", "pwd", "cd", "cat", "tree",
	"find", "grep", "head", "tail", "wc", "sort", "uniq",
	"mkdir", "cp", "mv", "touch", "echo", "which",
	"git", "go", "npm", "pnpm", "yarn",
	"curl", "wget",
}

// Unified Bash Tool
// ============================================================================

// BashParams defines the parameters for bash tool
type BashParams struct {
	Command string `json:"command" required:"true" jsonschema:"description=The bash command to execute (e.g., 'ls -la', 'git status')"`
}

// BashTool wraps extension's BashTool with Smart Guide specific behavior
type BashTool struct {
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
			"command":         baseCmd,
			"full":            command,
			"callbackSet":     true,
			"allowedCommands": t.AllowedCommands,
		}).Info("BashTool: Command not in allowlist, requesting approval via callback")

		// Parse command into base command and args
		parts := strings.Fields(command)
		var cmd string
		var args []string
		if len(parts) > 0 {
			cmd = parts[0]
			args = parts[1:]
		}

		logrus.WithFields(logrus.Fields{
			"cmd":  cmd,
			"args": args,
		}).Debug("BashTool: Calling Executor.onApproval callback")

		approved, err := t.Executor.onApproval(ctx, ApprovalRequest{
			Command: cmd,
			Args:    args,
			Reason:  fmt.Sprintf("Command '%s' is not in the allowlist", baseCmd),
		})

		logrus.WithFields(logrus.Fields{
			"command":  cmd,
			"approved": approved,
			"error":    err,
		}).Info("BashTool: onApproval callback returned")

		if err != nil {
			logrus.WithError(err).WithField("command", cmd).Error("BashTool: Approval callback failed with error")
			return tool.TextResponse(fmt.Sprintf("Error: approval request failed: %v", err)), nil
		}
		if !approved {
			logrus.WithField("command", cmd).Warn("BashTool: Command was NOT approved by user")
			return tool.TextResponse(fmt.Sprintf("Error: command '%s' was not approved by user", baseCmd)), nil
		}
		logrus.WithField("command", baseCmd).Info("BashTool: Command approved by user, executing")
		// Execute approved command without allowlist restriction
		return t.executeCommand(ctx, command, true)
	}

	// No approval callback - deny with error
	logrus.WithFields(logrus.Fields{
		"command":         baseCmd,
		"callbackSet":     false,
		"allowedCommands": t.AllowedCommands,
	}).Warn("BashTool: Command not in allowlist AND no approval callback set - denying")
	allowedList := strings.Join(t.AllowedCommands, ", ")
	return tool.TextResponse(fmt.Sprintf("Error: command '%s' is not allowed. Allowed commands: %s", baseCmd, allowedList)), nil
}

// executeCommand executes a bash command using the extension tool
// If skipAllowlist is true, the command is executed without allowlist restriction
func (t *BashTool) executeCommand(ctx context.Context, command string, skipAllowlist bool) (*tool.ToolResponse, error) {
	// Store original working directory
	oldDir := t.Executor.GetWorkingDirectory()

	// Create extension bash tool with current working directory
	cwd := oldDir

	// Build a composite command: { command; pwd; }
	// Using { } instead of ( ) ensures cd affects the current shell context
	// The semicolon ensures pwd runs even if command fails
	compositeCommand := fmt.Sprintf("{ %s; pwd; }", command)

	// Use empty allowlist for extension tool since allowlist check was already done in Call method
	// This design allows:
	// 1. Single allowlist enforcement point (Call method)
	// 2. Clean composite command format without allowlist conflicts
	// 3. Simplified extension tool invocation
	extBash := extTools.NewBashTool(
		extTools.BashOptions([]string{}, nil, 120*time.Second, cwd),
		extTools.BashAllowChaining(true), // Allow command chaining
	)

	// Execute using extension tool
	result, err := extBash.Call(ctx, extTools.BashParams{Command: compositeCommand})
	if err != nil {
		return result, err
	}

	// Extract the directory from the last line (pwd output)
	newDir := t.extractDirectoryFromResult(result)
	if newDir != "" && newDir != oldDir {
		t.Executor.SetWorkingDirectory(newDir)
		logrus.WithFields(logrus.Fields{
			"oldDir": oldDir,
			"newDir": newDir,
		}).Info("BashTool: Working directory changed after command execution")
		cwd = newDir
	}

	// Add working directory context to response (remove trailing pwd for cleaner output)
	if result != nil && len(result.Content) > 0 {
		if textBlock, ok := result.Content[0].(*message.TextBlock); ok {
			displayText := t.removeTrailingPwd(textBlock.Text)
			result.Content[0] = message.Text(fmt.Sprintf("(cwd: %s)\n%s", cwd, displayText))
		}
	}

	return result, nil
}

// extractDirectoryFromResult extracts the directory from the command result
// The last line should be the pwd output from our composite command
func (t *BashTool) extractDirectoryFromResult(result *tool.ToolResponse) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}

	if textBlock, ok := result.Content[0].(*message.TextBlock); ok {
		// Trim trailing whitespace/newlines before splitting
		text := strings.TrimRight(textBlock.Text, "\n\r\t ")
		lines := strings.Split(text, "\n")

		if len(lines) > 0 {
			// The last line is the pwd output
			potentialDir := strings.TrimSpace(lines[len(lines)-1])
			// Verify it's a valid absolute path
			if filepath.IsAbs(potentialDir) {
				return potentialDir
			}
		}
	}
	return ""
}

// removeTrailingPwd removes the trailing pwd line from the command output
func (t *BashTool) removeTrailingPwd(text string) string {
	lines := strings.Split(text, "\n")
	if len(lines) > 1 {
		lastLine := strings.TrimSpace(lines[len(lines)-1])
		if filepath.IsAbs(lastLine) {
			return strings.Join(lines[:len(lines)-1], "\n")
		}
	}
	return text
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
// Handles subshells by extracting the first actual command inside parentheses
func (t *BashTool) extractBaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	// Handle subshell format: (cmd...)
	if len(trimmed) > 0 && trimmed[0] == '(' {
		// Find the closing parenthesis or first command inside
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] == ')' || trimmed[i] == ' ' || trimmed[i] == '\t' {
				// Extract the command inside parentheses
				innerCmd := strings.TrimSpace(trimmed[1:i])
				return t.extractBaseCommand(innerCmd)
			}
		}
		// If no closing paren found, extract what's inside
		if len(trimmed) > 1 {
			innerCmd := strings.TrimSpace(trimmed[1:])
			return t.extractBaseCommand(innerCmd)
		}
	}

	// Normal case: extract first word
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
	executor          *ToolExecutor
	chatID            string // ChatID injected from agent config (not from LLM params)
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

	// Persist to chat store using injected chatID (not from LLM params)
	if t.updateProjectFunc != nil && t.chatID != "" {
		if err := t.updateProjectFunc(t.chatID, resolvedPath); err != nil {
			logrus.WithError(err).WithField("chatID", t.chatID).Warn("Failed to update project path in chat store")
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
