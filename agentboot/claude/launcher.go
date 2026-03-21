package claude

import (
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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/agentboot"
	"github.com/tingly-dev/tingly-box/agentboot/events"
	"github.com/tingly-dev/tingly-box/agentboot/mitm"
)

// Launcher handles Claude Code CLI execution
type Launcher struct {
	mu             sync.RWMutex
	defaultFormat  agentboot.OutputFormat
	cliPath        string
	skipPerms      bool
	config         Config
	controlManager *ControlManager
	discovery      *CLIDiscovery

	// executionContext stores the current execution context for permission requests
	executionContext struct {
		sessionID string
		chatID    string
		platform  string
		botUUID   string
	}
}

// NewLauncher creates a new Claude launcher
func NewLauncher(config Config) *Launcher {
	return &Launcher{
		defaultFormat:  agentboot.OutputFormatStreamJSON,
		cliPath:        "claude",
		skipPerms:      false,
		config:         config,
		controlManager: NewControlManager(),
		discovery:      NewCLIDiscovery(),
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
		err := l.ExecuteWithHandler(ctx, prompt, timeout, opts, opts.Handler)
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

func (l *Launcher) ExecuteWithHandler(ctx context.Context,
	prompt string,
	timeout time.Duration,
	opts agentboot.ExecutionOptions,
	handler agentboot.MessageHandler,
) error {

	// Set execution context for permission requests
	l.mu.Lock()
	l.executionContext.sessionID = opts.SessionID
	l.executionContext.chatID = opts.ChatID
	l.executionContext.platform = opts.Platform
	l.executionContext.botUUID = opts.BotUUID
	l.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"session_id": opts.SessionID,
	}).Info("ExecuteWithHandler starting")

	// Clear execution context when done
	defer func() {
		l.mu.Lock()
		l.executionContext.sessionID = ""
		l.executionContext.chatID = ""
		l.executionContext.platform = ""
		l.executionContext.botUUID = ""
		l.mu.Unlock()
	}()

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

	logrus.Infof("claude code cmd: %s", strings.Join(args, " "))

	// Create command
	cmd, err := l.buildCommand(ctx, args, opts)
	if err != nil {
		return err
	}

	accumulator := NewMessageAccumulator()

	// Build stream prompt with the user message
	// The server automatically wraps the user input in the correct stream-json format
	builder := NewStreamPromptBuilder()
	builder.AddUserMessage(prompt)
	inputPrompt := builder.Close()

	runner := mitm.New(
		cmd,
		nil,
		nil,
	)

	// Track if we intentionally killed the process (expected termination)
	var processIntentionallyKilled bool

	runner.Codec = mitm.CodecJSON

	inputSource := mitm.NewChanSource(100)

	// done channel signals the input feeder goroutine to stop
	done := make(chan struct{})
	go func() {
		for {
			select {
			case m, ok := <-inputPrompt:
				if !ok {
					return
				}
				inputSource.Write(m)
			case <-done:
				return
			}
		}
	}()

	runner.InputSource = inputSource

	// Ensure cleanup on exit
	defer func() {
		close(done)
		inputSource.Close()
	}()

	outputHandler := func(ctx context.Context, c *mitm.IOContext) (*mitm.OutputResult, error) {
		// Try to parse as JSON
		var data map[string]interface{}
		var ok = false
		if data, ok = c.Msg.(map[string]any); !ok {

			// Parser finished (EOF reached)
			// The Runner.Run() will call cmd.Wait() for us, don't call it here
			return nil, fmt.Errorf("invalid event data")
		}

		// Create event from parsed data
		event := events.NewEventFromMap(data)

		logrus.Debugf("[Event] %s", event)

		messages, _, resultSuccess := accumulator.AddEvent(event)

		for _, msg := range messages {
			logrus.WithFields(logrus.Fields{
				"event_type": event.Type,
			}).Debug("handleControlMessages received event")

			switch {
			// Check if this is a control request event (e.g., "control_request")
			case strings.HasPrefix(event.Type, EventTypeControl):

				// Fall back to legacy handling
				if controlData, ok := event.Data["request"].(map[string]interface{}); ok {
					subtype, _ := controlData["subtype"].(string)
					requestID := getString(event.Data, "request_id")

					switch subtype {
					case "can_use_tool":
						toolName, _ := controlData["tool_name"].(string)

						// Check if this is an AskUserQuestion tool
						if toolName == "AskUserQuestion" {
							req := l.parseAskRequestFromControl(controlData)
							req.ID = requestID

							logrus.WithFields(logrus.Fields{
								"platform":   req.Platform,
								"chat_id":    req.ChatID,
								"session_id": req.SessionID,
								"request_id": req.ID,
								"tool_name":  req.ToolName,
							}).Info("Processing AskUserQuestion control request")

							// Get ask response
							if handler != nil {
								result, err := handler.OnAsk(ctx, req)
								if err != nil {
									logrus.Errorf("Ask handler error: %v", err)
									result = agentboot.AskResult{ID: requestID, Approved: false}
								}

								// Send control response via stdin
								input := l.sendAskResponseNew(requestID, result)
								inputSource.Write(input)
							} else {
								logrus.Warn("Ask handler is nil, cannot process ask request")
							}
						} else {
							// Regular permission request
							req := l.parsePermissionRequest(controlData)
							req.RequestID = requestID

							logrus.WithFields(logrus.Fields{
								"tool_name":  req.ToolName,
								"session_id": req.SessionID,
								"request_id": req.RequestID,
							}).Info("Processing can_use_tool control request")

							// Get permission decision
							if handler != nil {
								result, err := handler.OnApproval(ctx, req)
								if err != nil {
									logrus.Errorf("Permission handler error: %v", err)
									result = agentboot.PermissionResult{Approved: false}
								}

								// Send control response via stdin
								input := l.sendPermissionResponseNew(requestID, result, req.Input)
								inputSource.Write(input)
							} else {
								logrus.Warn("Permission handler is nil, cannot process permission request")
							}
						}

					default:
						logrus.Warnf("Unsupported control request subtype: %s", subtype)

					}
				}
			case event.Type == EventTypeAssistant && opts.PermissionPromptTool == "":
				requestID := getString(event.Data, "request_id")

				if assistant, ok := msg.(*AssistantMessage); ok {
					for _, c := range assistant.Message.Content {
						if c.Name == "AskUserQuestion" {
							req := l.parseAskRequest(c)
							req.ID = requestID

							logrus.WithFields(logrus.Fields{
								"platform":   req.Platform,
								"chat_id":    req.ChatID,
								"session_id": req.SessionID,
								"request_id": req.ID,
								"tool_name":  req.ToolName,
							}).Info("Processing ask_user control request")

							// Get ask response
							if handler != nil {
								result, err := handler.OnAsk(ctx, req)
								if err != nil {
									logrus.Errorf("Ask handler error: %v", err)
									result = agentboot.AskResult{ID: requestID, Approved: false}
								}

								// Send control response via stdin
								input := l.sendAskResponseNew(requestID, result)
								inputSource.Write(input)
							} else {
								logrus.Warn("Ask handler is nil, cannot process ask request")
							}
						}
					}

				}

			case event.Type == EventTypeResult:
				handler.OnComplete(&agentboot.CompletionResult{
					Success: resultSuccess,
				})
				// Got final result, stop processing immediately
				// The process will be cleaned up by deferred close(done) and inputSource.Close()
				processIntentionallyKilled = true
				_ = cmd.Process.Kill()
				//_ = cmd.Wait()
				logrus.Warnf("killed: %d", cmd.Process.Pid)
				return &mitm.OutputResult{Action: mitm.Stop}, nil
			default:
				if hErr := handler.OnMessage(msg); hErr != nil {
					handler.OnError(hErr)
				}
			}
		}

		return &mitm.OutputResult{Action: mitm.Pass}, nil
	}

	runner.OutputHandler = outputHandler

	err = runner.Run(ctx)

	// Check if the error is due to the process being killed
	// If we intentionally killed it (after getting result), this is expected and not an error
	if err != nil && processIntentionallyKilled {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Process was killed by signal - this is expected if we killed it
			// Check if process exited with a signal (not normal exit)
			if exitErr.ProcessState != nil && !exitErr.ProcessState.Success() {
				logrus.WithFields(logrus.Fields{
					"exit_code": exitErr.ProcessState.ExitCode(),
					"state":     exitErr.ProcessState.String(),
				}).Debug("Process terminated after intentional kill - this is expected")
				return nil
			}
		}
	}

	return err
}

// buildCommandArgs constructs CLI arguments based on format, prompt, and config options
func (l *Launcher) buildCommandArgs(format agentboot.OutputFormat, prompt string, opts agentboot.ExecutionOptions) ([]string, error) {
	// Get config options
	l.mu.RLock()
	config := l.config
	skipPerms := l.skipPerms
	l.mu.RUnlock()

	// Convert ExecutionOptions to CommonOptions
	commonOpts := CommonOptions{
		Model:                opts.Model,
		FallbackModel:        opts.FallbackModel,
		MaxTurns:             opts.MaxTurns,
		CustomSystemPrompt:   opts.CustomSystemPrompt,
		AppendSystemPrompt:   opts.AppendSystemPrompt,
		AllowedTools:         opts.AllowedTools,
		DisallowedTools:      opts.DisallowedTools,
		MCPServers:           opts.MCPServers,
		StrictMcpConfig:      opts.StrictMcpConfig,
		PermissionMode:       opts.PermissionMode,
		SettingsPath:         opts.SettingsPath,
		PermissionPromptTool: opts.PermissionPromptTool,
	}

	// Auto-enable permission-prompt-tool if handler is set and using stream-json format
	if format == agentboot.OutputFormatStreamJSON && commonOpts.PermissionPromptTool == "" {
		commonOpts.PermissionPromptTool = "stdio"
	}

	// Handle session/resume with opts.SessionID taking precedence
	if opts.SessionID != "" {
		if opts.Resume || config.ContinueConversation {
			commonOpts.Resume = opts.SessionID
		}
		// Note: If not resuming, session-id is handled separately below
	} else if config.ResumeSessionID != "" {
		commonOpts.Resume = config.ResumeSessionID
	}

	// Use shared argument builder for common options
	args := BuildCommonArgs(config, commonOpts)

	// Handle --session-id for new sessions with specific ID (not resume)
	if opts.SessionID != "" && !opts.Resume && !config.ContinueConversation {
		args = append(args, "--session-id", opts.SessionID)
	}

	// Format-specific arguments
	switch format {
	case agentboot.OutputFormatStreamJSON:
		args = append(args, "--output-format", "stream-json", "--verbose")
		if prompt != "" && commonOpts.PermissionPromptTool == "" {
			args = append(args, "--print", prompt)
		} else {
			args = append(args, "--print", "")
			args = append(args, "--input-format", "stream-json")
		}
	case agentboot.OutputFormatText:
		args = append(args, "--print", "--output-format", "text")
		if prompt != "" {
			args = append(args, prompt)
		}
	default:
		return nil, fmt.Errorf("invalid output format: %s", format)
	}

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

// handleExecutionError processes execution errors
func (l *Launcher) handleExecutionError(err error, stderr string, handler agentboot.MessageHandler) error {
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

	handler.OnComplete(&agentboot.CompletionResult{
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

// parseAskRequest parses an ask request from control data
// Note: data is already the "request" object from the control message, not the full event.Data
func (l *Launcher) parseAskRequest(content anthropic.ContentBlockUnion) agentboot.AskRequest {

	// Inject chat context from execution context
	l.mu.RLock()
	sessionID := l.executionContext.sessionID
	chatID := l.executionContext.chatID
	platform := l.executionContext.platform
	l.mu.RUnlock()

	input := map[string]any{}
	json.Unmarshal(content.Input, &input)

	return agentboot.AskRequest{
		Type:      content.Type,
		AgentType: agentboot.AgentTypeClaude,

		Platform:  platform,
		ChatID:    chatID,
		SessionID: sessionID,

		ToolName: content.Name,
		Input:    input,
		Message:  content.Text,
		CallID:   content.ID,
	}
}

// parseAskRequestFromControl parses an ask request from control_request event data
func (l *Launcher) parseAskRequestFromControl(controlData map[string]interface{}) agentboot.AskRequest {
	// Inject chat context from execution context
	l.mu.RLock()
	sessionID := l.executionContext.sessionID
	chatID := l.executionContext.chatID
	platform := l.executionContext.platform
	botUUID := l.executionContext.botUUID
	l.mu.RUnlock()

	toolName, _ := controlData["tool_name"].(string)
	toolUseID, _ := controlData["tool_use_id"].(string)
	input, _ := controlData["input"].(map[string]interface{})

	return agentboot.AskRequest{
		Type:      "tool_use",
		AgentType: agentboot.AgentTypeClaude,

		Platform:  platform,
		ChatID:    chatID,
		BotUUID:   botUUID,
		SessionID: sessionID,

		ToolName: toolName,
		Input:    input,
		CallID:   toolUseID,
	}
}

// sendAskResponse sends an ask response to Claude Code
func (l *Launcher) sendAskResponse(stdin io.WriteCloser, requestID string, result agentboot.AskResult) error {
	response := map[string]interface{}{
		"request_id": requestID,
		"type":       "control_response",
	}

	innerResponse := map[string]interface{}{
		"request_id": requestID,
	}

	if result.Approved {
		innerResponse["subtype"] = "success"
		if result.UpdatedInput != nil {
			innerResponse["response"] = map[string]interface{}{
				"behavior":     "allow",
				"updatedInput": result.UpdatedInput,
			}
		} else {
			innerResponse["response"] = map[string]interface{}{
				"behavior": "allow",
			}
		}
	} else {
		innerResponse["subtype"] = "error"
		innerResponse["error"] = result.Reason
		if result.Reason == "" {
			innerResponse["error"] = "User denied this request"
		}
	}

	response["response"] = innerResponse

	data, _ := json.Marshal(response)
	_, err := stdin.Write(append(data, '\n'))
	return err
}

// sendAskResponse sends an ask response to Claude Code
func (l *Launcher) sendAskResponseNew(requestID string, result agentboot.AskResult) map[string]any {
	response := map[string]interface{}{
		"request_id": requestID,
		"type":       "control_response",
	}

	innerResponse := map[string]interface{}{
		"request_id": requestID,
	}

	if result.Approved {
		innerResponse["subtype"] = "success"
		if result.UpdatedInput != nil {
			innerResponse["response"] = map[string]interface{}{
				"behavior":     "allow",
				"updatedInput": result.UpdatedInput,
			}
		} else {
			innerResponse["response"] = map[string]interface{}{
				"behavior": "allow",
			}
		}
	} else {
		innerResponse["subtype"] = "error"
		innerResponse["error"] = result.Reason
		if result.Reason == "" {
			innerResponse["error"] = "User denied this request"
		}
	}

	response["response"] = innerResponse

	return response
}

// parsePermissionRequest parses a permission request from control data
// Note: data is already the "request" object from the control message, not the full event.Data
func (l *Launcher) parsePermissionRequest(data map[string]interface{}) agentboot.PermissionRequest {
	// data is already the request object, use it directly
	requestData := data

	// Get input map
	input := getMap(requestData, "input")
	if input == nil {
		input = make(map[string]interface{})
	}

	// Inject chat context from execution context
	l.mu.RLock()
	sessionID := l.executionContext.sessionID
	chatID := l.executionContext.chatID
	platform := l.executionContext.platform
	botUUID := l.executionContext.botUUID
	l.mu.RUnlock()

	if chatID != "" {
		input["_chat_id"] = chatID
	}
	if platform != "" {
		input["_platform"] = platform
	}

	// RequestID needs to be extracted from the parent event.Data, not from the request object
	// This is passed separately via the control data flow
	return agentboot.PermissionRequest{
		RequestID: getString(data, "request_id"), // This may be empty, needs to be set from caller
		AgentType: agentboot.AgentTypeClaude,
		ToolName:  getString(requestData, "tool_name"),
		Input:     input,
		Timestamp: time.Now(),
		SessionID: sessionID,
		BotUUID:   botUUID, // Include bot UUID for proper routing
	}
}

// sendPermissionResponse sends a permission response to Claude Code
func (l *Launcher) sendPermissionResponseNew(requestID string, result agentboot.PermissionResult, originalInput map[string]interface{}) map[string]any {
	response := map[string]interface{}{
		"request_id": requestID,
		"type":       "control_response",
	}

	innerResponse := map[string]interface{}{
		"subtype":    "success", // Always use "success" for control_response
		"request_id": requestID,
	}

	if result.Approved {
		// Allow: must include updatedInput (original or modified)
		updatedInput := result.UpdatedInput
		if updatedInput == nil {
			// If no updatedInput provided, use the original input
			updatedInput = originalInput
		}
		innerResponse["response"] = map[string]interface{}{
			"behavior":     "allow",
			"updatedInput": updatedInput,
		}
	} else {
		// Deny: must include message
		message := result.Reason
		if message == "" {
			message = "User denied this request"
		}
		innerResponse["response"] = map[string]interface{}{
			"behavior": "deny",
			"message":  message,
		}
	}

	response["response"] = innerResponse

	return response
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
