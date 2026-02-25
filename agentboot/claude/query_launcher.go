package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
)

// QueryLauncher handles launching Claude in Query/stdio mode
type QueryLauncher struct {
	config    Config
	discovery *CLIDiscovery
}

// NewQueryLauncher creates a new Query launcher
func NewQueryLauncher(config Config) *QueryLauncher {
	return &QueryLauncher{
		config:    config,
		discovery: NewCLIDiscovery(),
	}
}

// Query executes Claude in stdio mode and returns a Query instance
// This matches the happy SDK's query() function behavior
func (l *QueryLauncher) Query(ctx context.Context, qc QueryConfig) (*Query, error) {
	if err := ValidateQueryConfig(qc); err != nil {
		return nil, fmt.Errorf("invalid query config: %w", err)
	}

	// Build command args
	args := l.buildQueryArgs(qc)

	// Get CLI path
	cliPath := l.config.CLIPath
	if cliPath == "" {
		variant, err := l.discovery.FindClaudeCLI(ctx)
		if err != nil {
			return nil, fmt.Errorf("find Claude CLI: %w", err)
		}
		cliPath = variant.Path
	}

	// Prepare environment
	cleanEnv, _ := l.discovery.GetCleanEnv(ctx)
	if len(l.config.CustomEnv) > 0 {
		cleanEnv = MergeEnv(cleanEnv, l.config.CustomEnv)
	}

	// Create command
	cmd := exec.CommandContext(ctx, cliPath, args...)
	cmd.Env = cleanEnv

	if qc.Options.CWD != "" {
		cmd.Dir = qc.Options.CWD
	}

	// Setup stdio pipes
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdin pipe: %w", err)
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, fmt.Errorf("create stderr pipe: %w", err)
	}

	// Start the command
	logrus.Infof("Claude CLI command: %s %s", cliPath, strings.Join(args, " "))
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start command: %w", err)
	}

	// Log stderr in debug mode
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			logrus.Debugf("Claude stderr: %s", scanner.Text())
		}
	}()

	// Create process done channel
	processDone := make(chan struct{})
	var exitCode int
	go func() {
		err := cmd.Wait()
		if err != nil {
			// Try to get exit code from ProcessState
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			} else {
				// If ProcessState is not available, use a non-zero exit code
				exitCode = 1
			}
		} else {
			// Process exited successfully
			if cmd.ProcessState != nil {
				exitCode = cmd.ProcessState.ExitCode()
			}
		}
		close(processDone)
	}()

	// Handle prompt input
	switch prompt := qc.Prompt.(type) {
	case string:
		// String prompt - close stdin immediately
		stdin.Close()
	case StreamPrompt, chan map[string]interface{}:
		// Stream prompt - start streaming to stdin
		if qc.Options.CanCallTool == nil {
			return nil, fmt.Errorf("stream prompt requires canCallTool callback")
		}
		var ch <-chan map[string]interface{}
		if sp, ok := prompt.(StreamPrompt); ok {
			ch = sp
		} else {
			ch = prompt.(chan map[string]interface{})
		}
		go StreamToStdin(ctx, stdin, ch)
	case nil:
		// No prompt - close stdin
		stdin.Close()
	default:
		return nil, fmt.Errorf("unsupported prompt type: %T", prompt)
	}

	// Create query instance
	queryOpts := QueryOptions{
		CanCallTool: qc.Options.CanCallTool,
		AbortSignal: qc.Options.AbortSignal,
		ProcessDone: processDone,
	}

	query := NewQuery(ctx, stdin, stdout, queryOpts)

	// Handle cleanup on process exit
	go func() {
		<-processDone
		if !query.IsClosed() {
			if exitCode != 0 {
				query.SetError(fmt.Errorf("Claude process exited with code %d", exitCode))
			}
		}
	}()

	return query, nil
}

// buildQueryArgs builds command arguments for Query mode
func (l *QueryLauncher) buildQueryArgs(qc QueryConfig) []string {
	opts := qc.Options
	args := []string{"--output-format", "stream-json", "--verbose"}

	// Model selection
	if l.config.Model != "" {
		args = append(args, "--model", l.config.Model)
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if opts.FallbackModel != "" {
		args = append(args, "--fallback-model", opts.FallbackModel)
	}

	// System prompts
	if l.config.CustomSystemPrompt != "" {
		args = append(args, "--system-prompt", l.config.CustomSystemPrompt)
	}
	if opts.CustomSystemPrompt != "" {
		args = append(args, "--system-prompt", opts.CustomSystemPrompt)
	}
	if l.config.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", l.config.AppendSystemPrompt)
	}
	if opts.AppendSystemPrompt != "" {
		args = append(args, "--append-system-prompt", opts.AppendSystemPrompt)
	}

	// Max turns
	if opts.MaxTurns > 0 {
		args = append(args, "--max-turns", fmt.Sprintf("%d", opts.MaxTurns))
	}

	// Check if we're using stream input
	_, isStreamInput := qc.Prompt.(StreamPrompt)
	usingStreamInput := isStreamInput

	// CanCallTool requires stream input mode
	if opts.CanCallTool != nil {
		usingStreamInput = true
	}

	// Permission handling
	if usingStreamInput && opts.CanCallTool != nil {
		args = append(args, "--permission-prompt-tool", "stdio")
	}

	if l.config.PermissionMode != "" {
		args = append(args, "--permission-mode", string(l.config.PermissionMode))
	}
	if opts.PermissionMode != "" {
		args = append(args, "--permission-mode", opts.PermissionMode)
	}

	// Conversation control
	if l.config.ContinueConversation {
		args = append(args, "--continue")
	}
	if opts.ContinueConversation {
		args = append(args, "--continue")
	}
	if l.config.ResumeSessionID != "" {
		args = append(args, "--resume", l.config.ResumeSessionID)
	}
	if opts.Resume != "" {
		args = append(args, "--resume", opts.Resume)
	}

	// Tool filtering
	if len(l.config.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(l.config.AllowedTools, ","))
	}
	if len(opts.AllowedTools) > 0 {
		args = append(args, "--allowedTools", strings.Join(opts.AllowedTools, ","))
	}
	if len(l.config.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(l.config.DisallowedTools, ","))
	}
	if len(opts.DisallowedTools) > 0 {
		args = append(args, "--disallowedTools", strings.Join(opts.DisallowedTools, ","))
	}

	// MCP servers
	mcpServers := make(map[string]interface{})
	if l.config.MCPServers != nil {
		for k, v := range l.config.MCPServers {
			mcpServers[k] = v
		}
	}
	if opts.MCPServers != nil {
		for k, v := range opts.MCPServers {
			mcpServers[k] = v
		}
	}
	if len(mcpServers) > 0 {
		mcpConfig := map[string]interface{}{"mcpServers": mcpServers}
		mcpJSON, _ := json.Marshal(mcpConfig)
		args = append(args, "--mcp-config", string(mcpJSON))
	}

	if l.config.StrictMcpConfig || opts.StrictMcpConfig {
		args = append(args, "--strict-mcp-config")
	}

	// Settings path
	if l.config.SettingsPath != "" {
		args = append(args, "--settings", l.config.SettingsPath)
	}
	if opts.SettingsPath != "" {
		args = append(args, "--settings", opts.SettingsPath)
	}

	// Handle prompt input
	switch prompt := qc.Prompt.(type) {
	case string:
		if strings.TrimSpace(prompt) != "" {
			args = append(args, "--print", strings.TrimSpace(prompt))
		}
	case StreamPrompt, chan map[string]interface{}:
		// Use --input-format stream-json for stream prompts
		// IMPORTANT: --input-format requires --print to work
		// We use --print "" to enable stdin reading without a prompt string
		args = append(args, "--print", "")
		args = append(args, "--input-format", "stream-json")
	case nil:
		// No prompt - don't add --print or --input-format
	}

	return args
}

// StreamPromptBuilder helps build stream prompts
type StreamPromptBuilder struct {
	messages chan map[string]interface{}
	closed   bool
	mu       sync.Mutex
}

// NewStreamPromptBuilder creates a new stream prompt builder
func NewStreamPromptBuilder() *StreamPromptBuilder {
	return &StreamPromptBuilder{
		messages: make(chan map[string]interface{}, 100),
		closed:   false,
	}
}

// Add adds a message to the stream
func (b *StreamPromptBuilder) Add(msg map[string]interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return fmt.Errorf("stream builder is closed")
	}

	select {
	case b.messages <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("timeout adding message to stream")
	}
}

// AddText adds a text message to the stream
func (b *StreamPromptBuilder) AddText(text string) error {
	msg := map[string]interface{}{
		"type":    "text",
		"content": text,
	}
	return b.Add(msg)
}

// AddUserMessage adds a user message to the stream
func (b *StreamPromptBuilder) AddUserMessage(content string) error {
	msg := map[string]interface{}{
		"type": "user",
		"message": map[string]interface{}{
			"role":    "user",
			"content": content,
		},
	}
	return b.Add(msg)
}

// Close closes the stream and returns the channel for use with Query
func (b *StreamPromptBuilder) Close() StreamPrompt {
	b.mu.Lock()
	defer b.mu.Unlock()

	if !b.closed {
		b.closed = true
		close(b.messages)
	}

	return b.messages
}

// Messages returns the underlying message channel
func (b *StreamPromptBuilder) Messages() StreamPrompt {
	return b.messages
}

// IsClosed returns true if the builder is closed
func (b *StreamPromptBuilder) IsClosed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.closed
}

// QueryWithContext is a convenience function that creates a Query with the given context
func QueryWithContext(ctx context.Context, prompt string, opts ...QueryOption) (*Query, error) {
	config := &QueryOptionsConfig{}
	for _, opt := range opts {
		opt(config)
	}

	launcher := NewQueryLauncher(Config{})
	qc := QueryConfig{
		Prompt:  prompt,
		Options: config,
	}

	return launcher.Query(ctx, qc)
}

// QueryOption is a functional option for QueryWithContext
type QueryOption func(*QueryOptionsConfig)

// WithModel sets the model
func WithModel(model string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.Model = model
	}
}

// WithFallbackModel sets the fallback model
func WithFallbackModel(model string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.FallbackModel = model
	}
}

// WithCustomSystemPrompt sets a custom system prompt
func WithCustomSystemPrompt(prompt string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.CustomSystemPrompt = prompt
	}
}

// WithCWD sets the working directory
func WithCWD(cwd string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.CWD = cwd
	}
}

// WithResume sets the resume session ID
func WithResume(sessionID string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.Resume = sessionID
	}
}

// WithContinue sets continue conversation mode
func WithContinue() QueryOption {
	return func(c *QueryOptionsConfig) {
		c.ContinueConversation = true
	}
}

// WithCanCallTool sets the tool permission callback
func WithCanCallTool(callback CanCallToolCallback) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.CanCallTool = callback
	}
}

// WithMCPServers sets MCP servers
func WithMCPServers(servers map[string]interface{}) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.MCPServers = servers
	}
}

// WithAllowedTools sets allowed tools
func WithAllowedTools(tools ...string) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.AllowedTools = tools
	}
}

// WithAbortSignal sets the abort signal
func WithAbortSignal(signal <-chan struct{}) QueryOption {
	return func(c *QueryOptionsConfig) {
		c.AbortSignal = signal
	}
}
