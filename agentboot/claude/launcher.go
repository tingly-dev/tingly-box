package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/events"
	"github.com/tingly-dev/tingly-box/agentboot/permission"
)

// Launcher handles Claude Code CLI execution
type Launcher struct {
	mu                sync.RWMutex
	defaultFormat     agentboot.OutputFormat
	cliPath           string
	skipPerms         bool
	config            Config
	permissionHandler permission.Handler
}

// NewLauncher creates a new Claude launcher
func NewLauncher(config Config) *Launcher {
	return &Launcher{
		defaultFormat:     agentboot.OutputFormatText,
		cliPath:           "claude",
		skipPerms:         false,
		config:            config,
		permissionHandler: nil,
	}
}

// SetPermissionHandler sets the permission handler
func (l *Launcher) SetPermissionHandler(handler permission.Handler) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.permissionHandler = handler
}

// GetPermissionHandler returns the current permission handler
func (l *Launcher) GetPermissionHandler() permission.Handler {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.permissionHandler
}

// Execute runs Claude Code with the given prompt
func (l *Launcher) Execute(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.Result, error) {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}
	return l.ExecuteWithTimeout(ctx, prompt, timeout, opts)
}

// ExecuteWithTimeout runs Claude Code with a specific timeout
func (l *Launcher) ExecuteWithTimeout(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
) (*agentboot.Result, error) {
	start := time.Now()

	if !l.IsAvailable() {
		return &agentboot.Result{
			Error:  "claude CLI not found",
			Format: opts.OutputFormat,
		}, exec.ErrNotFound
	}

	// Determine output format
	format := opts.OutputFormat
	if format == "" {
		l.mu.RLock()
		format = l.defaultFormat
		l.mu.RUnlock()
	}
	if format == "" {
		format = agentboot.OutputFormatText
	}

	// Build command args
	var args []string
	switch format {
	case agentboot.OutputFormatStreamJSON:
		args = []string{"--output-format", "stream-json", "--verbose"}
		if prompt != "" {
			args = append(args, "--print", prompt)
		}
	case agentboot.OutputFormatText:
		args = []string{"--print", "--output-format", "text"}
		if prompt != "" {
			args = append(args, prompt)
		}
	default:
		return &agentboot.Result{
			Error:  fmt.Sprintf("invalid output format: %s", format),
			Format: format,
		}, fmt.Errorf("invalid format")
	}

	if l.skipPerms && !isRoot() {
		args = append(args, "--dangerously-skip-permissions")
	}

	cmd := exec.CommandContext(ctx, l.cliPath, args...)
	if strings.TrimSpace(opts.ProjectPath) != "" {
		if stat, err := os.Stat(opts.ProjectPath); err == nil && stat.IsDir() {
			cmd.Dir = opts.ProjectPath
		} else if err != nil {
			return &agentboot.Result{
				Error:  "invalid project path: " + err.Error(),
				Format: format,
			}, err
		} else {
			return &agentboot.Result{
				Error:  "invalid project path: not a directory",
				Format: format,
			}, os.ErrInvalid
		}
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// For stream-json with permission handling, we need stdin
	var stdin io.WriteCloser
	if format == agentboot.OutputFormatStreamJSON && l.permissionHandler != nil {
		var err error
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
		}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		if stdin != nil {
			stdin.Close()
		}
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	if stdin != nil {
		// Start permission handler goroutine (reading from stderr for control messages)
		go l.handleControlMessages(context.Background(), stdin, &stderr)

		// Close stdin since we're not sending any initial input
		stdin.Close()
	}

	// Wait for command to complete
	err := cmd.Wait()
	duration := time.Since(start)

	stderrOutput := strings.TrimSpace(stderr.String())

	result := &agentboot.Result{
		Duration: duration,
		Format:   format,
		Metadata: make(map[string]interface{}),
	}

	// Process output based on format
	if format == agentboot.OutputFormatStreamJSON {
		// Parse stream-json events
		parser := events.NewParser()
		if err := parser.Parse(bytes.NewReader(stdout.Bytes())); err != nil {
			result.Error = fmt.Sprintf("parse error: %v", err)
			result.ExitCode = 1
			return result, err
		}

		// Collect all events
		for event := range parser.Events() {
			// Convert events.Event to agentboot.Event
			result.Events = append(result.Events, agentboot.Event{
				Type:      event.Type,
				Data:      event.Data,
				Timestamp: event.Timestamp,
				Raw:       event.Raw,
			})

			// Extract metadata
			switch event.Type {
			case "status":
				result.Metadata["status"] = event.Data["status"]
			case "thinking":
				result.Metadata["thinking"] = event.Data
			case "token_count":
				result.Metadata["tokens"] = event.Data
			}
		}

		// Extract text output
		result.Output = result.TextOutput()
	} else {
		// Text format - use raw output
		result.Output = strings.TrimSpace(stdout.String())
	}

	// Handle errors
	if err != nil {
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
	logrus.Infof("Claude Code execution completed in %v (format: %s)", duration, format)

	return result, nil
}

// IsAvailable checks if Claude Code CLI is available
func (l *Launcher) IsAvailable() bool {
	cmd := exec.Command("which", "claude")
	if err := cmd.Run(); err == nil {
		l.mu.Lock()
		l.cliPath = "claude"
		l.mu.Unlock()
		return true
	}

	// Fallback to anthropic CLI
	cmd = exec.Command("which", "anthropic")
	if err := cmd.Run(); err == nil {
		l.mu.Lock()
		l.cliPath = "anthropic"
		l.mu.Unlock()
		return true
	}

	return false
}

// Type returns the agent type
func (l *Launcher) Type() agentboot.AgentType {
	return agentboot.AgentTypeClaude
}

// SetDefaultFormat sets the default output format
func (l *Launcher) SetDefaultFormat(format agentboot.OutputFormat) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.defaultFormat = format
}

// GetDefaultFormat returns the current default format
func (l *Launcher) GetDefaultFormat() agentboot.OutputFormat {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.defaultFormat == "" {
		return agentboot.OutputFormatText
	}
	return l.defaultFormat
}

// SetSkipPermissions enables or disables skip permissions mode
func (l *Launcher) SetSkipPermissions(enabled bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.skipPerms = enabled
}

// SetCLIPath sets an explicit CLI path
func (l *Launcher) SetCLIPath(path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if strings.TrimSpace(path) != "" {
		l.cliPath = path
	}
}

// handleControlMessages handles incoming control messages from stderr
func (l *Launcher) handleControlMessages(ctx context.Context, stdin io.WriteCloser, stderr io.Reader) {
	parser := events.NewParser()

	// Parse stderr for control messages
	go func() {
		_ = parser.Parse(stderr)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-parser.Events():
			if !ok {
				return
			}

			if event.Type == EventTypeControlRequest {
				if controlData, ok := event.Data["request"].(map[string]interface{}); ok {
					if subtype, _ := controlData["subtype"].(string); subtype == "can_use_tool" {
						req := l.parsePermissionRequest(controlData)

						// Get permission decision
						if l.permissionHandler != nil {
							result, err := l.permissionHandler.CanUseTool(ctx, req)
							if err != nil {
								logrus.Errorf("Permission handler error: %v", err)
								result = agentboot.PermissionResult{Approved: false}
							}

							// Send control response via stdin
							_ = l.sendPermissionResponse(stdin, req.RequestID, result)
						}
					}
				}
			}
		}
	}
}

// parsePermissionRequest parses a permission request from control data
func (l *Launcher) parsePermissionRequest(data map[string]interface{}) agentboot.PermissionRequest {
	requestData, _ := data["request"].(map[string]interface{})

	return agentboot.PermissionRequest{
		RequestID: getString(data, "request_id"),
		ToolName:  getString(requestData, "tool_name"),
		Input:     getMap(requestData, "input"),
		Timestamp: time.Now(),
	}
}

// sendPermissionResponse sends a permission response to Claude Code
func (l *Launcher) sendPermissionResponse(stdin io.WriteCloser, requestID string, result agentboot.PermissionResult) error {
	response := map[string]interface{}{
		"request_id": requestID,
		"type":       "control_response",
	}

	if result.Approved {
		response["response"] = map[string]interface{}{
			"subtype":   "success",
			"request_id": requestID,
		}
	} else {
		response["response"] = map[string]interface{}{
			"subtype":   "error",
			"request_id": requestID,
			"error":     result.Reason,
		}
	}

	data, _ := json.Marshal(response)
	_, err := stdin.Write(append(data, '\n'))
	return err
}

func isRoot() bool {
	uid := os.Getuid()
	return uid == 0
}

// Helper functions for type-safe map access
func getString(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

func getMap(m map[string]interface{}, key string) map[string]interface{} {
	if val, ok := m[key]; ok {
		if m, ok := val.(map[string]interface{}); ok {
			return m
		}
	}
	return nil
}
