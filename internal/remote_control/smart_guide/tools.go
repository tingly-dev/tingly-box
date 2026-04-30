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
)

// ============================================================================
// Tool Context & Executor
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

// HasApprovalCallback returns true if an approval callback is set
func (e *ToolExecutor) HasApprovalCallback() bool {
	return e.onApproval != nil
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
		return ""
	}
	return e.BashCwd
}

// ResolvePath resolves a path to an absolute path
func (e *ToolExecutor) ResolvePath(path string) string {
	if !filepath.IsAbs(path) {
		currentDir := e.GetWorkingDirectory()
		if currentDir == "" {
			if wd, err := os.Getwd(); err == nil {
				currentDir = wd
			} else {
				currentDir = "/"
			}
		}
		return filepath.Join(currentDir, path)
	}
	return path
}

// ExecuteBash executes a bash command with allowlist checking
func (e *ToolExecutor) ExecuteBash(ctx context.Context, cmd string, args ...string) (string, error) {
	cmdLower := strings.ToLower(cmd)
	if _, allowed := e.BashAllowlist[cmdLower]; !allowed {
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
			return "", fmt.Errorf("command '%s' is not allowed. Allowed commands: %v", cmd, e.GetAllowedCommands())
		}
	}

	fullCmd := append([]string{cmd}, args...)
	execCmd := exec.CommandContext(ctx, fullCmd[0], fullCmd[1:]...)
	if e.BashCwd != "" {
		execCmd.Dir = e.BashCwd
	}
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
// Default Configuration
// ============================================================================
