package agentsec

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

// ToolExecutor manages bash command execution with allowlist enforcement
// and pluggable approval callbacks for non-allowlisted commands.
//
// It is agent-agnostic: any agent that needs controlled bash execution
// (SmartGuide, future agents) can embed or use a ToolExecutor.
type ToolExecutor struct {
	// BashAllowlist is the set of allowed base-command names (lower-case keys).
	// Populated from a []string allowlist via NewToolExecutor.
	BashAllowlist map[string]struct{}

	// BashCwd is the current working directory for command execution.
	BashCwd string

	onApproval      ApprovalCallback
	approvalTimeout time.Duration
}

// NewToolExecutor creates a ToolExecutor with the given allowlist.
// Entries are stored lower-cased for case-insensitive lookup.
func NewToolExecutor(allowlist []string) *ToolExecutor {
	m := make(map[string]struct{}, len(allowlist))
	for _, cmd := range allowlist {
		m[strings.ToLower(cmd)] = struct{}{}
	}
	return &ToolExecutor{
		BashAllowlist:   m,
		approvalTimeout: 5 * time.Minute,
	}
}

// SetApprovalCallback wires a callback for commands not in the allowlist.
func (e *ToolExecutor) SetApprovalCallback(cb ApprovalCallback) {
	e.onApproval = cb
}

// HasApprovalCallback returns true if an approval callback has been set.
func (e *ToolExecutor) HasApprovalCallback() bool {
	return e.onApproval != nil
}

// SetApprovalTimeout overrides the default 5-minute approval timeout.
func (e *ToolExecutor) SetApprovalTimeout(d time.Duration) {
	e.approvalTimeout = d
}

// SetWorkingDirectory sets the working directory used for subsequent commands.
func (e *ToolExecutor) SetWorkingDirectory(cwd string) {
	e.BashCwd = cwd
}

// GetWorkingDirectory returns the current working directory.
func (e *ToolExecutor) GetWorkingDirectory() string {
	return e.BashCwd
}

// ResolvePath resolves a possibly-relative path against the current working
// directory. Falls back to os.Getwd() if no working directory is set.
func (e *ToolExecutor) ResolvePath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	base := e.BashCwd
	if base == "" {
		if wd, err := os.Getwd(); err == nil {
			base = wd
		} else {
			base = "/"
		}
	}
	return filepath.Join(base, path)
}

// GetAllowedCommands returns the current allowlist as a sorted slice.
func (e *ToolExecutor) GetAllowedCommands() []string {
	cmds := make([]string, 0, len(e.BashAllowlist))
	for cmd := range e.BashAllowlist {
		cmds = append(cmds, cmd)
	}
	return cmds
}

// CallApproval invokes the registered approval callback with the given request.
// Returns (false, error) if no callback is set.
func (e *ToolExecutor) CallApproval(ctx context.Context, req ApprovalRequest) (bool, error) {
	if e.onApproval == nil {
		return false, fmt.Errorf("no approval callback configured")
	}
	return e.onApproval(ctx, req)
}

// ExecuteBash runs cmd with the provided args, checking the allowlist first.
// If the command is not in the allowlist and an approval callback is set,
// it requests user approval before proceeding.
func (e *ToolExecutor) ExecuteBash(ctx context.Context, cmd string, args ...string) (string, error) {
	cmdLower := strings.ToLower(cmd)
	if _, allowed := e.BashAllowlist[cmdLower]; !allowed {
		if e.onApproval != nil {
			logrus.WithFields(logrus.Fields{
				"command": cmd,
				"args":    args,
			}).Info("agentsec: command not in allowlist, requesting approval")

			approved, err := e.onApproval(ctx, ApprovalRequest{
				Command: cmd,
				Args:    args,
				Reason:  fmt.Sprintf("command %q is not in the allowlist", cmd),
			})
			if err != nil {
				return "", fmt.Errorf("approval request failed: %w", err)
			}
			if !approved {
				return "", fmt.Errorf("command %q was not approved by user", cmd)
			}
			logrus.WithField("command", cmd).Info("agentsec: command approved by user")
		} else {
			return "", fmt.Errorf("command %q is not allowed (allowed: %v)", cmd, e.GetAllowedCommands())
		}
	}

	execCmd := exec.CommandContext(ctx, cmd, args...)
	if e.BashCwd != "" {
		execCmd.Dir = e.BashCwd
	}
	out, err := execCmd.CombinedOutput()
	if err != nil {
		return string(out), fmt.Errorf("command failed: %w", err)
	}
	return string(out), nil
}
