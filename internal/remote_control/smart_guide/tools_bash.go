package smart_guide

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
)

// DefaultBashAllowlist defines the default allowed bash commands. Commands
// outside this list trigger the approval callback (if configured).
var DefaultBashAllowlist = []string{
	"ls", "pwd", "cd", "cat", "tree",
	"find", "grep", "head", "tail", "wc", "sort", "uniq",
	"mkdir", "cp", "mv", "touch", "echo", "which",
	"git", "go", "npm", "pnpm", "yarn",
	"curl", "wget",
}

// bashCommandTimeout bounds a single bash invocation.
const bashCommandTimeout = 120 * time.Second

// ============================================================================
// bash
// ============================================================================

// BashTool executes shell commands with an allowlist + approval gate.
type BashTool struct {
	Executor        *ToolExecutor
	AllowedCommands []string
}

// NewBashTool creates a new bash tool bound to the given executor and allowlist.
func NewBashTool(executor *ToolExecutor, allowlist []string) *BashTool {
	return &BashTool{Executor: executor, AllowedCommands: allowlist}
}

func (t *BashTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name: "bash",
		Description: anthropic.String(`Execute bash commands for file system operations and git.

Allowed commands: ls, pwd, cat, mkdir, cp, mv, git, curl, wget, and more.
Supports command chaining with &&, ||, |, ;.

Examples:
- List files: ls -la
- Show current directory: pwd
- Clone repository: git clone https://github.com/user/repo.git
- Change directory temporarily: cd /path/to/dir && ls`),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "The bash command to execute (e.g., 'ls -la', 'git status').",
				},
			},
			Required: []string{"command"},
		},
	}
}

type bashParams struct {
	Command string `json:"command"`
}

// Call runs a bash command, gating non-allowlisted base commands behind the
// approval callback. On approval (or for allowlisted commands) it executes the
// command and tracks any working-directory change so `cd` persists within the
// session.
func (t *BashTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var p bashParams
	if err := json.Unmarshal(rawInput, &p); err != nil {
		return "", fmt.Errorf("bash: invalid arguments: %w", err)
	}
	command := strings.TrimSpace(p.Command)
	if command == "" {
		return "Error: 'command' parameter is required", nil
	}

	baseCmd := extractBaseCommand(command)

	if !t.isAllowed(baseCmd) {
		if t.Executor.onApproval == nil {
			return fmt.Sprintf("Error: command '%s' is not allowed. Allowed commands: %s",
				baseCmd, strings.Join(t.AllowedCommands, ", ")), nil
		}
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
			return fmt.Sprintf("Error: approval request failed: %v", err), nil
		}
		if !approved {
			return fmt.Sprintf("Error: command '%s' was not approved by user", baseCmd), nil
		}
		logrus.WithField("command", baseCmd).Info("BashTool: command approved by user")
	}

	return t.execute(ctx, command), nil
}

// execute runs the command via `sh -c`, appends a trailing pwd so we can detect
// directory changes, and updates the executor's working directory accordingly.
func (t *BashTool) execute(ctx context.Context, command string) string {
	oldDir := t.Executor.GetWorkingDirectory()

	// Wrap in { ...; pwd; } so a cd inside the command is reflected in the final
	// pwd line; braces (not a subshell) keep cd effective for the trailing pwd.
	composite := fmt.Sprintf("{ %s; } ; pwd", command)

	execCtx, cancel := context.WithTimeout(ctx, bashCommandTimeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, "sh", "-c", composite)
	if oldDir != "" {
		cmd.Dir = oldDir
	}
	out, err := cmd.CombinedOutput()

	output := string(out)
	newDir := trailingDir(output)
	if newDir != "" {
		output = stripTrailingDir(output)
		if newDir != oldDir {
			t.Executor.SetWorkingDirectory(newDir)
			logrus.WithFields(logrus.Fields{"oldDir": oldDir, "newDir": newDir}).
				Info("BashTool: working directory changed")
		}
	}

	cwd := t.Executor.GetWorkingDirectory()
	body := strings.TrimRight(output, "\n")
	if err != nil {
		return fmt.Sprintf("(cwd: %s)\n%s\n[command exited with error: %v]", cwd, body, err)
	}
	return fmt.Sprintf("(cwd: %s)\n%s", cwd, body)
}

// isAllowed reports whether baseCmd is in the allowlist. An empty allowlist
// allows everything.
func (t *BashTool) isAllowed(baseCmd string) bool {
	if len(t.AllowedCommands) == 0 {
		return true
	}
	for _, cmd := range t.AllowedCommands {
		if strings.EqualFold(cmd, baseCmd) {
			return true
		}
	}
	return false
}

// extractBaseCommand returns the first command word, unwrapping a leading
// subshell paren if present.
func extractBaseCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	if len(trimmed) > 0 && trimmed[0] == '(' {
		for i := 1; i < len(trimmed); i++ {
			if trimmed[i] == ')' || trimmed[i] == ' ' || trimmed[i] == '\t' {
				return extractBaseCommand(strings.TrimSpace(trimmed[1:i]))
			}
		}
		if len(trimmed) > 1 {
			return extractBaseCommand(strings.TrimSpace(trimmed[1:]))
		}
	}
	for i, r := range trimmed {
		if r == ' ' || r == '\t' {
			return strings.ToLower(trimmed[:i])
		}
	}
	return strings.ToLower(trimmed)
}

// trailingDir returns the last line of output if it is an absolute path (the
// pwd we appended), else "".
func trailingDir(output string) string {
	text := strings.TrimRight(output, "\n\r\t ")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return ""
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if filepath.IsAbs(last) {
		return last
	}
	return ""
}

// stripTrailingDir removes the trailing pwd line that trailingDir detected.
func stripTrailingDir(output string) string {
	text := strings.TrimRight(output, "\n")
	lines := strings.Split(text, "\n")
	if len(lines) == 0 {
		return output
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if filepath.IsAbs(last) {
		return strings.Join(lines[:len(lines)-1], "\n")
	}
	return output
}

// ============================================================================
// get_status
// ============================================================================

// GetStatusTool returns the current bot/session status.
type GetStatusTool struct {
	executor      *ToolExecutor
	chatID        string
	getStatusFunc func(chatID string) (*StatusInfo, error)
}

// NewGetStatusTool creates a new GetStatusTool. chatID is injected from agent
// config (not taken from model input).
func NewGetStatusTool(executor *ToolExecutor, chatID string, getStatusFunc func(chatID string) (*StatusInfo, error)) *GetStatusTool {
	return &GetStatusTool{executor: executor, chatID: chatID, getStatusFunc: getStatusFunc}
}

func (t *GetStatusTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        "get_status",
		Description: anthropic.String("Get the current bot status including agent, session, project path, and working directory."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Type:       "object",
			Properties: map[string]interface{}{},
		},
	}
}

func (t *GetStatusTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	cwd := t.executor.GetWorkingDirectory()
	if t.getStatusFunc == nil {
		return fmt.Sprintf("Current working directory: %s", cwd), nil
	}
	status, err := t.getStatusFunc(t.chatID)
	if err != nil {
		return fmt.Sprintf("Error getting status: %v", err), nil
	}
	if status != nil {
		status.WorkingDir = cwd
	}
	return fmt.Sprintf("**Current Status:**\n"+
		"• Agent: %s\n"+
		"• Session: %s\n"+
		"• Project: %s\n"+
		"• Working Directory: %s\n"+
		"• Whitelisted: %v",
		status.CurrentAgent, status.SessionID, status.ProjectPath, status.WorkingDir, status.Whitelisted), nil
}

// ============================================================================
// change_workdir
// ============================================================================

// ChangeDirTool changes the bound project directory and persists it.
type ChangeDirTool struct {
	executor          *ToolExecutor
	chatID            string
	updateProjectFunc func(chatID string, projectPath string) error
}

// NewChangeDirTool creates a new ChangeDirTool. chatID is injected from agent
// config (not taken from model input).
func NewChangeDirTool(executor *ToolExecutor, chatID string, updateProjectFunc func(chatID string, projectPath string) error) *ChangeDirTool {
	return &ChangeDirTool{executor: executor, chatID: chatID, updateProjectFunc: updateProjectFunc}
}

func (t *ChangeDirTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        "change_workdir",
		Description: anthropic.String("Change the bound project directory. This updates both the current working directory and the persisted project path."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "The directory path to change to (absolute or relative to current directory).",
				},
			},
			Required: []string{"path"},
		},
	}
}

type changeDirParams struct {
	Path string `json:"path"`
}

func (t *ChangeDirTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var p changeDirParams
	if err := json.Unmarshal(rawInput, &p); err != nil {
		return "", fmt.Errorf("change_workdir: invalid arguments: %w", err)
	}
	if p.Path == "" {
		return "Error: 'path' parameter is required", nil
	}

	resolved := t.executor.ResolvePath(p.Path)
	info, err := os.Stat(resolved)
	if err != nil {
		return fmt.Sprintf("Error: %v", err), nil
	}
	if !info.IsDir() {
		return fmt.Sprintf("Error: '%s' is not a directory", resolved), nil
	}

	t.executor.SetWorkingDirectory(resolved)
	if t.updateProjectFunc != nil && t.chatID != "" {
		if err := t.updateProjectFunc(t.chatID, resolved); err != nil {
			logrus.WithError(err).WithField("chatID", t.chatID).Warn("Failed to persist project path")
			return fmt.Sprintf("Warning: directory changed but persistence failed: %v\nNew directory: %s", err, resolved), nil
		}
	}

	lsCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	lsCmd := exec.CommandContext(lsCtx, "ls", "-la")
	lsCmd.Dir = resolved
	out, _ := lsCmd.CombinedOutput()

	return fmt.Sprintf("✅ Changed directory to: %s\n\nDirectory contents:\n%s", resolved, string(out)), nil
}
