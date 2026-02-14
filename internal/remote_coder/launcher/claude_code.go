package launcher

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// Result represents the result of a Claude Code execution
type Result struct {
	Output   string // Claude Code output
	ExitCode int    // Process exit code
	Error    string // Error message if failed
	Duration time.Duration
}

// ClaudeCodeLauncher handles Claude Code CLI execution
type ClaudeCodeLauncher struct {
	defaultTimeout  time.Duration
	cliPath         string // Path to Claude CLI (defaults to "claude")
	skipPermissions bool   // Whether to skip permission prompts
}

// ExecuteOptions controls Claude Code execution
type ExecuteOptions struct {
	ProjectPath string
}

// NewClaudeCodeLauncher creates a new Claude Code launcher
func NewClaudeCodeLauncher() *ClaudeCodeLauncher {
	return &ClaudeCodeLauncher{
		defaultTimeout:  5 * time.Minute,
		cliPath:         "claude",
		skipPermissions: false,
	}
}

// SetSkipPermissions enables or disables skip permissions mode
func (l *ClaudeCodeLauncher) SetSkipPermissions(enabled bool) {
	l.skipPermissions = enabled
}

// SetCLIPath sets an explicit CLI path
func (l *ClaudeCodeLauncher) SetCLIPath(path string) {
	if strings.TrimSpace(path) != "" {
		l.cliPath = path
	}
}

// Execute runs Claude Code with the given prompt
func (l *ClaudeCodeLauncher) Execute(ctx context.Context, prompt string, opts ExecuteOptions) (*Result, error) {
	return l.ExecuteWithTimeout(ctx, prompt, l.defaultTimeout, opts)
}

// ExecuteWithTimeout runs Claude Code with a specific timeout
func (l *ClaudeCodeLauncher) ExecuteWithTimeout(ctx context.Context, prompt string, timeout time.Duration, opts ExecuteOptions) (*Result, error) {
	start := time.Now()

	if !l.IsAvailable() {
		return &Result{Error: "claude CLI not found"}, exec.ErrNotFound
	}

	// Build command args
	args := []string{"--print", "--output-format", "text"}

	// Only add skip permissions flag if not running as root
	if l.skipPermissions && !isRoot() {
		args = append(args, "--dangerously-skip-permissions")
	}

	args = append(args, prompt)

	cmd := exec.CommandContext(ctx, l.cliPath, args...)
	if strings.TrimSpace(opts.ProjectPath) != "" {
		if stat, err := os.Stat(opts.ProjectPath); err == nil && stat.IsDir() {
			cmd.Dir = opts.ProjectPath
		} else if err != nil {
			return &Result{Error: "invalid project path: " + err.Error()}, err
		} else {
			return &Result{Error: "invalid project path: not a directory"}, os.ErrInvalid
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	output := strings.TrimSpace(stdout.String())
	stderrOutput := strings.TrimSpace(stderr.String())

	result := &Result{
		Output:   output,
		Duration: duration,
	}

	if err != nil {
		// Check if it's a timeout
		if ctx.Err() == context.DeadlineExceeded {
			result.Error = "execution timed out"
		} else if exitErr, ok := err.(*exec.ExitError); ok {
			result.ExitCode = exitErr.ExitCode()
			result.Error = stderrOutput
			if result.Error == "" {
				result.Error = exitErr.Error()
			}
		} else {
			result.Error = err.Error()
		}
		logrus.Errorf("Claude Code execution failed: %v", err)
		return result, err
	}

	result.ExitCode = 0
	logrus.Infof("Claude Code execution completed in %v", duration)

	return result, nil
}

// IsAvailable checks if Claude Code CLI is available
func (l *ClaudeCodeLauncher) IsAvailable() bool {
	// Check for claude CLI first
	cmd := exec.Command("which", "claude")
	if err := cmd.Run(); err == nil {
		l.cliPath = "claude"
		return true
	}

	// Fallback to anthropic CLI
	cmd = exec.Command("which", "anthropic")
	if err := cmd.Run(); err == nil {
		l.cliPath = "anthropic"
		return true
	}

	return false
}

// GetCLIInfo returns information about available Claude Code CLI
func (l *ClaudeCodeLauncher) GetCLIInfo() map[string]string {
	info := make(map[string]string)

	cmd := exec.Command("which", "claude")
	if err := cmd.Run(); err == nil {
		info["claude"] = "available"
	}

	cmd = exec.Command("which", "anthropic")
	if err := cmd.Run(); err == nil {
		info["anthropic"] = "available"
	}

	return info
}

// isRoot returns true if running as root user
func isRoot() bool {
	uid := os.Getuid()
	return uid == 0
}
