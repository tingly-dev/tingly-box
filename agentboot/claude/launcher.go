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

// handlerWrapper wraps agentboot.MessageHandler to claude.MessageHandler
type handlerWrapper struct {
	handler agentboot.MessageHandler
}

func (w *handlerWrapper) OnMessage(msg Message) error {
	return w.handler.OnMessage(msg)
}

func (w *handlerWrapper) OnError(err error) {
	w.handler.OnError(err)
}

func (w *handlerWrapper) OnComplete(result *ResultCompletion) {
	w.handler.OnComplete(&agentboot.CompletionResult{
		Success:   result.Success,
		SessionID: result.SessionID,
		Error:     result.Error,
	})
}

// Launcher handles Claude Code CLI execution
type Launcher struct {
	mu                sync.RWMutex
	defaultFormat     agentboot.OutputFormat
	cliPath           string
	skipPerms         bool
	config            Config
	permissionHandler permission.Handler
	controlManager    *ControlManager
	discovery         *CLIDiscovery
}

// NewLauncher creates a new Claude launcher
func NewLauncher(config Config) *Launcher {
	return &Launcher{
		defaultFormat:     agentboot.OutputFormatStreamJSON,
		cliPath:           "claude",
		skipPerms:         false,
		config:            config,
		permissionHandler: nil,
		controlManager:    NewControlManager(),
		discovery:         NewCLIDiscovery(),
	}
}

// GetControlManager returns the control manager
func (l *Launcher) GetControlManager() *ControlManager {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.controlManager
}

// GetDiscovery returns the CLI discovery instance
func (l *Launcher) GetDiscovery() *CLIDiscovery {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.discovery
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
		// Use configured default timeout
		l.mu.RLock()
		timeout = l.config.DefaultExecutionTimeout
		l.mu.RUnlock()
		// Fallback to 5 minutes if not configured
		if timeout == 0 {
			timeout = 5 * time.Minute
		}
	}
	logrus.Infof("launching claude code...: %s", prompt)
	// If handler is provided in options, use ExecuteWithHandler directly
	if opts.Handler != nil {
		// Wrap the agentboot.Handler to claude.MessageHandler
		wrappedHandler := &handlerWrapper{handler: opts.Handler}
		err := l.ExecuteWithHandler(ctx, prompt, timeout, opts, wrappedHandler)
		// The handler should have collected the result
		return nil, err
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
	args, err := l.buildCommandArgs(format, prompt, opts)
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
	args, err := l.buildCommandArgs(format, prompt, opts)
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

// buildCommandArgs constructs CLI arguments based on format, prompt, and config options
func (l *Launcher) buildCommandArgs(format agentboot.OutputFormat, prompt string, opts agentboot.ExecutionOptions) ([]string, error) {
	var args []string

	// Get config options
	l.mu.RLock()
	config := l.config
	l.mu.RUnlock()

	// Model selection
	if config.Model != "" {
		args = append(args, "--model", config.Model)
	}

	// Resume / Continue handling
	sessionID := opts.SessionID
	if sessionID == "" && config.ResumeSessionID != "" {
		sessionID = config.ResumeSessionID
	}

	if sessionID != "" && (opts.Resume || config.ContinueConversation) {
		args = append(args, "--resume", sessionID)
	} else if sessionID != "" {
		// Use --session-id for new sessions with specific ID
		args = append(args, "--session-id", sessionID)
	} else if config.ContinueConversation {
		args = append(args, "--continue")
	}

	// System prompt options
	if config.CustomSystemPrompt != "" {
		args = append(args, "--custom-system-prompt", config.CustomSystemPrompt)
	}
	if config.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", config.AppendSystemPrompt)
	}

	// Tool filtering
	if len(config.AllowedTools) > 0 {
		args = append(args, "--allowed-tools", strings.Join(config.AllowedTools, ","))
	}
	if len(config.DisallowedTools) > 0 {
		args = append(args, "--disallowed-tools", strings.Join(config.DisallowedTools, ","))
	}

	// Settings path
	if config.SettingsPath != "" {
		args = append(args, "--settings", config.SettingsPath)
	}

	// Permission mode
	if config.PermissionMode != "" {
		switch config.PermissionMode {
		case PermissionModeAuto:
			args = append(args, "--permission-mode", "auto")
		case PermissionModeManual:
			args = append(args, "--permission-mode", "manual")
		case PermissionModeOnce:
			args = append(args, "--permission-mode", "once")
		}
	}

	// MCP server configuration (if any)
	if len(config.MCPServers) > 0 {
		mcpArgs, err := l.buildMCPArgs(config.MCPServers)
		if err != nil {
			return nil, fmt.Errorf("build MCP args: %w", err)
		}
		args = append(args, mcpArgs...)
	}

	switch format {
	case agentboot.OutputFormatStreamJSON:
		args = append(args, "--output-format", "stream-json", "--verbose")
		if prompt != "" {
			args = append(args, "--print", prompt)
		}
	case agentboot.OutputFormatText:
		args = append(args, "--print", "--output-format", "text")
		if prompt != "" {
			args = append(args, prompt)
		}
	default:
		return nil, fmt.Errorf("invalid output format: %s", format)
	}

	l.mu.RLock()
	skipPerms := l.skipPerms
	l.mu.RUnlock()

	// Skip permissions takes precedence over permission mode
	if skipPerms && !isRoot() {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args, nil
}

// buildMCPArgs constructs MCP server arguments from config
func (l *Launcher) buildMCPArgs(servers map[string]interface{}) ([]string, error) {
	var args []string

	for name, config := range servers {
		serverConfig, ok := config.(map[string]interface{})
		if !ok {
			continue
		}

		// Build --mcp-server argument: name:key1=value1:key2=value2
		var parts []string
		parts = append(parts, name)

		for k, v := range serverConfig {
			parts = append(parts, fmt.Sprintf("%s=%v", k, v))
		}

		args = append(args, "--mcp-server", strings.Join(parts, ":"))
	}

	return args, nil
}

// buildCommand creates the exec.Cmd with proper working directory and environment
func (l *Launcher) buildCommand(ctx context.Context, args []string, opts agentboot.ExecutionOptions) (*exec.Cmd, error) {
	l.mu.RLock()
	cliPath := l.cliPath
	config := l.config
	discovery := l.discovery
	l.mu.RUnlock()

	// Use CLI discovery if path is not explicitly set
	if cliPath == "claude" || cliPath == "anthropic" {
		if variant, err := discovery.FindClaudeCLI(ctx); err == nil && variant != nil {
			cliPath = variant.Path
		}
	}

	cmd := exec.CommandContext(ctx, cliPath, args...)

	// Set working directory
	if strings.TrimSpace(opts.ProjectPath) != "" {
		if stat, err := os.Stat(opts.ProjectPath); err == nil && stat.IsDir() {
			cmd.Dir = opts.ProjectPath
		} else if err != nil {
			return nil, fmt.Errorf("invalid project path: %w", err)
		} else {
			return nil, os.ErrInvalid
		}
	}

	// Build clean environment with custom variables
	cleanEnv, err := discovery.GetCleanEnv(ctx)
	if err != nil {
		logrus.Debugf("Failed to get clean env: %v", err)
		cleanEnv = os.Environ()
	}

	// Merge custom environment variables
	if len(config.CustomEnv) > 0 {
		cmd.Env = MergeEnv(cleanEnv, config.CustomEnv)
	} else {
		cmd.Env = cleanEnv
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

// IsAvailable checks if Claude Code CLI is available using CLI discovery
func (l *Launcher) IsAvailable() bool {
	l.mu.RLock()
	discovery := l.discovery
	cliPath := l.cliPath
	l.mu.RUnlock()

	// If explicit path is set, verify it exists
	if cliPath != "" && cliPath != "claude" && cliPath != "anthropic" {
		if _, err := os.Stat(cliPath); err == nil {
			return true
		}
		return false
	}

	// Use discovery to find CLI
	variant, err := discovery.FindClaudeCLI(context.Background())
	if err != nil {
		return false
	}

	// Update cliPath for future use
	l.mu.Lock()
	l.cliPath = variant.Path
	l.mu.Unlock()

	return true
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
	controlMgr := l.GetControlManager()

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
				// Try to handle via control manager first
				if err := controlMgr.HandleControlMessage(event.Data); err == nil {
					// Control manager handled it
					continue
				}

				// Fall back to legacy handling
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

// Interrupt sends an interrupt request to the Claude process
func (l *Launcher) Interrupt(ctx context.Context, stdin io.WriteCloser, reason string) error {
	controlMgr := l.GetControlManager()

	builder := NewCancelRequestBuilder().
		WithCancel("execution").
		WithReason(reason)

	return controlMgr.SendRequestAsync(builder.Build(), stdin)
}

// SendPermissionRequest sends a permission request and waits for response
func (l *Launcher) SendPermissionRequest(ctx context.Context, req agentboot.PermissionRequest, stdin io.WriteCloser) (agentboot.PermissionResult, error) {
	controlMgr := l.GetControlManager()

	builder := NewPermissionRequestBuilder().
		WithRequestID(req.RequestID).
		WithTool(req.ToolName, req.Input)

	ctrlReq := builder.Build()
	resp, err := controlMgr.SendRequest(ctx, ctrlReq, stdin)
	if err != nil {
		return agentboot.PermissionResult{Approved: false}, err
	}

	// Parse response
	result := agentboot.PermissionResult{Approved: true}
	if resp.Response != nil {
		if subtype, _ := resp.Response["subtype"].(string); subtype == "error" {
			result.Approved = false
			result.Reason, _ = resp.Response["error"].(string)
		}
	}

	return result, nil
}
