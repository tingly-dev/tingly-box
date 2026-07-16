package codex

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Driver prepares a non-interactive `codex exec --json` process.
type Driver struct {
	mu             sync.RWMutex
	config         Config
	forceAvailable bool
}

func NewDriver(config Config) *Driver {
	defaults := DefaultConfig()
	if strings.TrimSpace(config.CLIPath) == "" {
		config.CLIPath = defaults.CLIPath
	}
	if strings.TrimSpace(config.SandboxMode) == "" {
		config.SandboxMode = defaults.SandboxMode
	}
	if strings.TrimSpace(config.ApprovalPolicy) == "" {
		config.ApprovalPolicy = defaults.ApprovalPolicy
	}
	return &Driver{config: config}
}

func (d *Driver) Type() agentboot.AgentType { return agentboot.AgentTypeCodex }

func (d *Driver) SetForceAvailable(value bool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.forceAvailable = value
}

func (d *Driver) SetCLIPath(path string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if path = strings.TrimSpace(path); path != "" {
		d.config.CLIPath = path
	}
}

func (d *Driver) IsAvailable() bool {
	d.mu.RLock()
	forced := d.forceAvailable
	path := d.config.CLIPath
	d.mu.RUnlock()
	if forced {
		return true
	}
	_, err := exec.LookPath(path)
	return err == nil
}

func (d *Driver) Prepare(_ context.Context, prompt string, opts agentboot.ExecutionOptions) (*agentboot.LaunchSpec, error) {
	d.mu.RLock()
	config := d.config
	d.mu.RUnlock()

	format := opts.OutputFormat
	if format == "" {
		format = agentboot.OutputFormatStreamJSON
	}
	if format != agentboot.OutputFormatStreamJSON {
		return nil, fmt.Errorf("codex: unsupported output format %q", format)
	}
	if opts.SessionID != "" && !opts.Resume {
		return nil, fmt.Errorf("codex: a native session ID cannot be selected for a new thread")
	}

	workDir := strings.TrimSpace(opts.ProjectPath)
	if workDir == "" {
		return nil, fmt.Errorf("codex: project path is required")
	}
	info, err := os.Stat(workDir)
	if err != nil {
		return nil, fmt.Errorf("codex: project path: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("codex: project path is not a directory")
	}
	sandboxMode := config.SandboxMode
	if opts.SandboxMode != "" {
		if opts.SandboxMode != "read-only" && opts.SandboxMode != "workspace-write" {
			return nil, fmt.Errorf("codex: unsupported task sandbox mode %q", opts.SandboxMode)
		}
		sandboxMode = opts.SandboxMode
	}

	prompt = composePrompt(prompt, opts.CustomSystemPrompt, opts.AppendSystemPrompt)
	args := []string{"exec"}
	if opts.Resume {
		if strings.TrimSpace(opts.SessionID) == "" {
			return nil, fmt.Errorf("codex: resume requires a session ID")
		}
		args = append(args, "resume")
	}
	args = append(args,
		"-c", "approval_policy="+strconv.Quote(config.ApprovalPolicy),
		"-c", "sandbox_mode="+strconv.Quote(sandboxMode),
		"--json",
	)
	if config.SkipGitRepoCheck {
		args = append(args, "--skip-git-repo-check")
	}
	if opts.Model != "" {
		args = append(args, "--model", opts.Model)
	}
	if !opts.Resume {
		args = append(args, "--cd", workDir)
	} else {
		args = append(args, opts.SessionID)
	}
	args = append(args, prompt)

	env := mergeEnv(os.Environ(), config.CustomEnv)
	env = mergeEnv(env, opts.Env)
	return &agentboot.LaunchSpec{
		Command: append([]string{config.CLIPath}, args...),
		Env:     env,
		WorkDir: workDir,
	}, nil
}

func composePrompt(prompt, customSystem, appendix string) string {
	parts := []string{strings.TrimSpace(prompt)}
	if value := strings.TrimSpace(customSystem); value != "" {
		parts = append(parts, "System instructions:\n"+value)
	}
	if value := strings.TrimSpace(appendix); value != "" {
		parts = append(parts, "Execution outcome contract:\n"+value)
	}
	return strings.Join(parts, "\n\n")
}

func mergeEnv(base, overrides []string) []string {
	if len(overrides) == 0 {
		return base
	}
	values := make(map[string]string, len(base)+len(overrides))
	order := make([]string, 0, len(base)+len(overrides))
	add := func(entry string) {
		key, _, ok := strings.Cut(entry, "=")
		if !ok || key == "" {
			return
		}
		if _, exists := values[key]; !exists {
			order = append(order, key)
		}
		values[key] = entry
	}
	for _, entry := range base {
		add(entry)
	}
	for _, entry := range overrides {
		add(entry)
	}
	merged := make([]string, 0, len(order))
	for _, key := range order {
		merged = append(merged, values[key])
	}
	return merged
}
