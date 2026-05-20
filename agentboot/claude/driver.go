package claude

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/agentboot"
)

// Driver implements agentboot.AgentDriver for Claude Code CLI.
// It is responsible for binary discovery, CLI argument construction,
// and environment preparation. It does NOT communicate with the running process.
type Driver struct {
	mu             sync.RWMutex
	config         Config
	skipPerms      bool
	cliPath        string
	discovery      *CLIDiscovery
	forceAvailable bool
}

// NewDriver creates a new Claude Driver.
func NewDriver(config Config) *Driver {
	return &Driver{
		config:    config,
		cliPath:   "claude",
		discovery: NewCLIDiscovery(),
	}
}

// Type returns the agent type.
func (d *Driver) Type() agentboot.AgentType {
	return agentboot.AgentTypeClaude
}

// SetForceAvailable bypasses binary-presence checks. Tests inject a
// process.Factory that does not spawn the real claude binary; in that case
// IsAvailable should return true regardless of what's on disk.
func (d *Driver) SetForceAvailable(v bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.forceAvailable = v
}

// IsAvailable checks whether the Claude Code CLI binary is present and usable.
func (d *Driver) IsAvailable() bool {
	d.mu.RLock()
	if d.forceAvailable {
		d.mu.RUnlock()
		return true
	}
	discovery := d.discovery
	cliPath := d.cliPath
	d.mu.RUnlock()

	if cliPath != "" && cliPath != "claude" && cliPath != "anthropic" {
		_, err := os.Stat(cliPath)
		return err == nil
	}

	variant, err := discovery.FindClaudeCLI(context.Background())
	if err != nil {
		return false
	}

	d.mu.Lock()
	d.cliPath = variant.Path
	d.mu.Unlock()
	return true
}

// SetSkipPermissions enables or disables the --dangerously-skip-permissions flag.
func (d *Driver) SetSkipPermissions(enabled bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.skipPerms = enabled
}

// SetCLIPath overrides the path to the Claude CLI binary.
func (d *Driver) SetCLIPath(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if strings.TrimSpace(path) != "" {
		d.cliPath = path
	}
}

// Prepare builds a LaunchSpec for the given prompt and execution options.
func (d *Driver) Prepare(ctx context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.LaunchSpec, error) {
	d.mu.RLock()
	config := d.config
	skipPerms := d.skipPerms
	d.mu.RUnlock()

	format := opts.OutputFormat
	if format == "" {
		format = agentboot.OutputFormatStreamJSON
	}

	args, err := d.buildArgs(format, prompt, opts, config, skipPerms)
	if err != nil {
		return nil, err
	}

	binary, err := d.resolveBinary(ctx)
	if err != nil {
		return nil, err
	}

	logrus.Infof("claude code cmd: %s %s", binary, strings.Join(args, " "))

	command := append([]string{binary}, args...)

	// Build environment
	cleanEnv, envErr := d.discovery.GetCleanEnv(ctx)
	if envErr != nil {
		logrus.Debugf("Failed to get clean env: %v", envErr)
		cleanEnv = os.Environ()
	}
	env := cleanEnv
	if len(config.CustomEnv) > 0 {
		env = MergeEnv(env, config.CustomEnv)
	}
	// Per-execution env overrides (e.g. gateway routing for remote control).
	if len(opts.Env) > 0 {
		env = MergeEnv(env, opts.Env)
	}

	// Build the initial prompt channel (stream-json mode injects a user message into stdin)
	var initialInput <-chan any
	if format == agentboot.OutputFormatStreamJSON {
		builder := NewStreamPromptBuilder()
		builder.AddUserMessage(prompt)
		ch := builder.Close()
		// Convert chan map[string]any to chan any
		anyC := make(chan any, 100)
		go func() {
			defer close(anyC)
			for m := range ch {
				anyC <- m
			}
		}()
		initialInput = anyC
	}

	// Resolve working directory
	workDir := ""
	if strings.TrimSpace(opts.ProjectPath) != "" {
		if stat, err := os.Stat(opts.ProjectPath); err != nil {
			return nil, fmt.Errorf("invalid project path: %w", err)
		} else if !stat.IsDir() {
			return nil, os.ErrInvalid
		}
		workDir = opts.ProjectPath
	}

	return &agentboot.LaunchSpec{
		Command:      command,
		Env:          env,
		WorkDir:      workDir,
		InitialInput: initialInput,
	}, nil
}

// resolveBinary returns the path to the Claude CLI binary, using discovery.
func (d *Driver) resolveBinary(ctx context.Context) (string, error) {
	d.mu.RLock()
	cliPath := d.cliPath
	discovery := d.discovery
	d.mu.RUnlock()

	if cliPath != "" && cliPath != "claude" && cliPath != "anthropic" {
		return cliPath, nil
	}

	variant, err := discovery.FindClaudeCLI(ctx)
	if err != nil {
		return "", fmt.Errorf("claude CLI not found: %w", err)
	}

	d.mu.Lock()
	d.cliPath = variant.Path
	d.mu.Unlock()

	return variant.Path, nil
}

// buildArgs constructs the CLI arguments for a Claude execution.
func (d *Driver) buildArgs(
	format agentboot.OutputFormat,
	prompt string,
	opts agentboot.ExecutionOptions,
	config Config,
	skipPerms bool,
) ([]string, error) {
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

	// Auto-enable stdio permission protocol for stream-json mode.
	if format == agentboot.OutputFormatStreamJSON && commonOpts.PermissionPromptTool == "" {
		commonOpts.PermissionPromptTool = "stdio"
	}

	// Session/resume handling.
	if opts.SessionID != "" {
		if opts.Resume || config.ContinueConversation {
			commonOpts.Resume = opts.SessionID
		}
	} else if config.ResumeSessionID != "" {
		commonOpts.Resume = config.ResumeSessionID
	}

	args := BuildCommonArgs(config, commonOpts)

	// Non-resume session ID.
	if opts.SessionID != "" && !opts.Resume && !config.ContinueConversation {
		args = append(args, "--session-id", opts.SessionID)
	}

	// Format-specific flags.
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

	if skipPerms && !isRoot() {
		args = append(args, "--dangerously-skip-permissions")
	}

	return args, nil
}

func isRoot() bool {
	return os.Getuid() == 0
}
