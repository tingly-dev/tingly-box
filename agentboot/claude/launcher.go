package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

	// Use streaming execution internally
	collector := NewResultCollector()
	if err := l.ExecuteWithHandler(ctx, prompt, timeout, opts, collector); err != nil {
		return collector.Result(), err
	}

	result := collector.Result()
	result.Duration = time.Since(start)

	if result.Error != "" {
		return result, errors.New(result.Error)
	}

	return result, nil
}

// ExecuteWithHandler executes Claude and streams events to a message handler
func (l *Launcher) ExecuteWithHandler(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
	handler MessageHandler,
) error {
	// Create context with timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	if !l.IsAvailable() {
		return exec.ErrNotFound
	}

	// Determine output format
	format := opts.OutputFormat
	if format == "" {
		l.mu.RLock()
		format = l.defaultFormat
		l.mu.RUnlock()
	}
	if format == "" {
		format = agentboot.OutputFormatStreamJSON
	}

	// Build command args
	args, err := l.buildCommandArgs(format, prompt)
	if err != nil {
		return err
	}

	// Create command
	cmd, err := l.buildCommand(ctx, args, opts)
	if err != nil {
		return err
	}

	// Setup pipes
	var stderr bytes.Buffer
	var stdin io.WriteCloser

	// Create stdout pipe for real-time reading
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	defer stdoutReader.Close()
	// Note: stdoutWriter is closed by goroutine after cmd.Wait() completes

	cmd.Stdout = stdoutWriter
	cmd.Stderr = &stderr

	if format == agentboot.OutputFormatStreamJSON && l.permissionHandler != nil {
		stdin, err = cmd.StdinPipe()
		if err != nil {
			return fmt.Errorf("failed to create stdin pipe: %w", err)
		}
	}

	// Start the command
	if err := cmd.Start(); err != nil {
		if stdin != nil {
			stdin.Close()
		}
		return fmt.Errorf("failed to start command: %w", err)
	}

	if stdin != nil {
		go l.handleControlMessages(ctx, stdin, &stderr)
		stdin.Close()
	}

	// Start parser in background
	parser := events.NewParser()
	accumulator := NewMessageAccumulator()
	go parser.Parse(stdoutReader)

	// Main event loop: process events until completion or context cancelled
	for {
		select {
		case <-ctx.Done():
			// Context cancelled: kill command and cleanup
			_ = cmd.Process.Kill()
			_ = stdoutWriter.Close() // Signal EOF to parser
			// Drain parser events until channel closes
			for range parser.Events() {
			}
			// Wait for command to complete and check error
			if err := cmd.Wait(); err != nil {
				return l.handleExecutionError(err, stderr.String(), handler)
			}
			return ctx.Err()

		case event, ok := <-parser.Events():
			if !ok {
				// Parser finished (EOF reached)
				// Now wait for command to complete
				if err := cmd.Wait(); err != nil {
					return l.handleExecutionError(err, stderr.String(), handler)
				}
				return nil
			}

			messages, hasResult, resultSuccess := accumulator.AddEvent(event)

			for _, msg := range messages {
				if hErr := handler.OnMessage(msg); hErr != nil {
					handler.OnError(hErr)
				}
			}

			if hasResult {
				handler.OnComplete(&ResultCompletion{
					Success: resultSuccess,
				})
				// Got final result, terminate command early
				_ = cmd.Process.Kill()
				_ = stdoutWriter.Close()
				_ = cmd.Wait() // Ignore error since we're terminating
				return nil
			}
		}
	}
}

// ExecuteStream executes Claude and returns a stream handler
func (l *Launcher) ExecuteStream(
	ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
) (*StreamHandler, error) {
	// Create context with timeout
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		_ = cancel // Will be done when stream handler closes
	}

	if !l.IsAvailable() {
		return nil, exec.ErrNotFound
	}

	// Determine output format
	format := opts.OutputFormat
	if format == "" {
		l.mu.RLock()
		format = l.defaultFormat
		l.mu.RUnlock()
	}
	if format == "" {
		format = agentboot.OutputFormatStreamJSON
	}

	// Build command args
	args, err := l.buildCommandArgs(format, prompt)
	if err != nil {
		return nil, err
	}

	// Create command
	cmd, err := l.buildCommand(ctx, args, opts)
	if err != nil {
		return nil, err
	}

	// Create stream handler
	streamHandler := NewStreamHandler(100)

	// Setup pipes
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var stdin io.WriteCloser
	if format == agentboot.OutputFormatStreamJSON && l.permissionHandler != nil {
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
		go l.handleControlMessages(ctx, stdin, &stderr)
		_ = stdin.Close()
	}

	// Create a pipe for real-time output streaming
	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	cmd.Stdout = stdoutWriter

	// Start the command
	if err := cmd.Start(); err != nil {
		stdoutReader.Close()
		stdoutWriter.Close()
		if stdin != nil {
			stdin.Close()
		}
		return nil, fmt.Errorf("failed to start command: %w", err)
	}

	// Start parsing in background - read from pipe in real-time
	go l.streamOutput(cmd, events.NewParser(), stdoutReader, streamHandler)

	// Close writer after command starts (reader will get EOF)
	go func() {
		cmd.Wait()
		stdoutWriter.Close()
	}()

	return streamHandler, nil
}

// buildCommandArgs constructs CLI arguments based on format and prompt
func (l *Launcher) buildCommandArgs(format agentboot.OutputFormat, prompt string) ([]string, error) {
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
		return nil, fmt.Errorf("invalid output format: %s", format)
	}

	l.mu.RLock()
	skipPerms := l.skipPerms
	l.mu.RUnlock()

	if skipPerms && !isRoot() {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args, nil
}

// buildCommand creates the exec.Cmd with proper working directory
func (l *Launcher) buildCommand(ctx context.Context, args []string, opts agentboot.ExecutionOptions) (*exec.Cmd, error) {
	l.mu.RLock()
	cliPath := l.cliPath
	l.mu.RUnlock()

	cmd := exec.CommandContext(ctx, cliPath, args...)

	if strings.TrimSpace(opts.ProjectPath) != "" {
		if stat, err := os.Stat(opts.ProjectPath); err == nil && stat.IsDir() {
			cmd.Dir = opts.ProjectPath
		} else if err != nil {
			return nil, fmt.Errorf("invalid project path: %w", err)
		} else {
			return nil, os.ErrInvalid
		}
	}

	return cmd, nil
}

// streamOutput parses output and sends to stream handler
func (l *Launcher) streamOutput(cmd *exec.Cmd, p *events.Parser, stdout io.Reader, handler *StreamHandler) {
	defer handler.Close()

	// Parse stdout in real-time
	if err := p.Parse(stdout); err != nil {
		handler.errorChan <- err
		return
	}

	// Wait for command to complete
	cmd.Wait()
}

// handleExecutionError processes execution errors
func (l *Launcher) handleExecutionError(err error, stderr string, handler MessageHandler) error {
	var errMsg string

	// Check for timeout error
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		errMsg = "execution timed out"
	} else if exitErr, ok := err.(*exec.ExitError); ok {
		errMsg = strings.TrimSpace(stderr)
		if errMsg == "" {
			errMsg = exitErr.Error()
		}
	} else {
		errMsg = err.Error()
	}

	handler.OnComplete(&ResultCompletion{
		Success: false,
		Error:   errMsg,
	})

	return fmt.Errorf("claude execution failed: %w", err)
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

			// Check if this is a control request event (e.g., "control_request")
			if strings.HasPrefix(event.Type, EventTypeControl) {
				if controlData, ok := event.Data["request"].(map[string]interface{}); ok {
					if subtype, _ := controlData["subtype"].(string); subtype == "can_use_tool" {
						req := l.parsePermissionRequest(controlData)

						// Get permission decision
						l.mu.RLock()
						handler := l.permissionHandler
						l.mu.RUnlock()

						if handler != nil {
							result, err := handler.CanUseTool(ctx, req)
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
			"subtype":    "success",
			"request_id": requestID,
		}
	} else {
		response["response"] = map[string]interface{}{
			"subtype":    "error",
			"request_id": requestID,
			"error":      result.Reason,
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
