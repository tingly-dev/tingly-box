package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	internalagent "github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// agentKillGrace is how long a timed-out run waits for the killed process's
// output pipe to close before giving up on it. Only the direct child is
// killed; a descendant that inherited the pipe (a runtime launcher, a spawned
// tool) can hold it open long after.
var agentKillGrace = 3 * time.Second

// syncBuffer is a bytes.Buffer safe to read while exec's copy goroutine may
// still be writing, which happens when a timed-out run stops waiting on it.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf.Bytes()...)
}

// runAgentCmdWithTimeout runs cmd, capturing stdout+stderr into one buffer,
// under the given per-entry timeout. Returns the captured output, a
// timed-out flag, and the Wait/Start error (if any). On timeout the process
// is killed and context.DeadlineExceeded is returned as the error, after at
// most agentKillGrace more for the output pipe to drain.
// When timeout <= 0 the timeout is disabled and this behaves like
// cmd.CombinedOutput().
func runAgentCmdWithTimeout(cmd *exec.Cmd, timeout time.Duration) ([]byte, bool, error) {
	var buf syncBuffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if timeout <= 0 {
		err := cmd.Run()
		return buf.Bytes(), false, err
	}

	if err := cmd.Start(); err != nil {
		return buf.Bytes(), false, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-time.After(timeout):
		_ = cmd.Process.Kill()
		select {
		case <-done: // reaped
		case <-time.After(agentKillGrace): // a descendant still holds the pipe
		}
		return buf.Bytes(), true, context.DeadlineExceeded
	case err := <-done:
		return buf.Bytes(), false, err
	}
}

// applyAgentResult copies the outcome of an agent CLI run into the unified
// result row shared by the mock, real and batch paths. Duration is left to
// the caller: the mock path takes the CLI's own, the real path times the whole
// entry including gateway setup.
func applyAgentResult(r *RealAgentTestResult, inner *AgentTestResult) {
	r.Success = inner.Success
	r.TimedOut = inner.TimedOut
	r.Output = inner.Output
	r.Error = inner.Error
	r.ExitCode = inner.ExitCode
}

// appendSummary persists one row, warning (never failing the run) when the
// summary file cannot be written. A nil writer is a no-op.
func appendSummary(writer *summaryWriter, r *RealAgentTestResult) {
	if writer == nil {
		return
	}
	if err := writer.Append(r); err != nil {
		fmt.Printf("⚠️  summary append failed: %v\n", err)
	}
}

// finishAgentResult records the shared outcome fields of one agent CLI run —
// the single implementation of the timeout / error / success classification
// every executor uses.
func finishAgentResult(result *AgentTestResult, start time.Time, output []byte, timedOut bool, timeout time.Duration, err error) {
	result.Duration = time.Since(start)
	result.Output = string(output)
	switch {
	case timedOut:
		result.Error = fmt.Sprintf("timeout after %s", timeout)
		result.ExitCode = -1
		result.TimedOut = true
	case err != nil:
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
	default:
		result.Success = true
	}
}

// virtualAgent describes how one agent CLI is run against the virtual-model
// gateway: the provider name and model it is registered under, and the driver
// that writes the agent's config and spawns the CLI.
type virtualAgent struct {
	provider string
	model    string
	run      func(env *protocoltest.AgentTestEnv, model, prompt string, timeout time.Duration) (*AgentTestResult, error)
}

var virtualAgents = map[protocoltest.AgentType]virtualAgent{
	protocoltest.AgentTypeClaudeCode: {
		provider: "virtual-claude", model: "cc",
		run: func(env *protocoltest.AgentTestEnv, _, prompt string, timeout time.Duration) (*AgentTestResult, error) {
			return executeClaudeWithEnv(env, prompt, timeout)
		},
	},
	protocoltest.AgentTypeCodex:    {provider: "virtual-codex", model: "tingly-codex", run: executeCodexWithEnv},
	protocoltest.AgentTypeOpenCode: {provider: "virtual-opencode", model: "tingly-opencode", run: executeOpenCodeWithEnv},
}

// executeAgentCommand runs the actual agent CLI against an ephemeral gateway
// wired to the virtual-model server.
func executeAgentCommand(agentType protocoltest.AgentType, prompt string, timeout time.Duration) (*AgentTestResult, error) {
	spec, ok := virtualAgents[agentType]
	if !ok {
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}

	env, err := protocoltest.NewAgentTestEnv(agentType)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(agentType, spec.provider, spec.model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return spec.run(env, spec.model, prompt, timeout)
}

// executeClaudeWithEnv writes settings.json and runs claude CLI against a pre-configured env.
//
// The settings env is built via internalagent.DefaultClaudeCodePrefs — the
// same canonical defaults the GUI quick-config seeds from — so the harness
// exercises the real Claude Code configuration shape (telemetry flags,
// model routing vars) rather than a hand-rolled minimal subset.
func executeClaudeWithEnv(env *protocoltest.AgentTestEnv, prompt string, timeout time.Duration) (*AgentTestResult, error) {
	start := time.Now()
	result := &AgentTestResult{
		Agent:  "claude",
		Prompt: prompt,
	}

	settingsDir, err := os.MkdirTemp("", "harness-claude-*")
	if err != nil {
		return nil, fmt.Errorf("create temp settings dir: %w", err)
	}
	defer os.RemoveAll(settingsDir)

	settingsPath := filepath.Join(settingsDir, "settings.json")
	result.SettingsPath = settingsPath

	claudeEnv, err := internalagent.DefaultClaudeCodePrefs().ToEnv(env.BaseURL(), env.ModelToken())
	if err != nil {
		return nil, fmt.Errorf("build claude env: %w", err)
	}
	settings := map[string]interface{}{
		"env": claudeEnv,
	}
	settingsJSON, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal settings: %w", err)
	}
	if err := os.WriteFile(settingsPath, settingsJSON, 0644); err != nil {
		return nil, fmt.Errorf("write settings: %w", err)
	}

	variant, err := claude.FindClaudeCLI(context.Background())
	if err != nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Sprintf("claude CLI not found: %v", err)
		result.ExitCode = 1
		return result, nil
	}

	fmt.Printf("🔧 Gateway: %s\n", env.BaseURL())
	fmt.Printf("🔧 Settings: %s\n", settingsPath)
	fmt.Printf("🚀 Command: claude --settings %s -p \"%s\"\n\n", settingsPath, prompt)

	cmd := exec.Command(variant.Path, "--settings", settingsPath, "-p", prompt)
	cmd.Env = os.Environ()

	output, timedOut, err := runAgentCmdWithTimeout(cmd, timeout)
	finishAgentResult(result, start, output, timedOut, timeout, err)
	return result, nil
}

// codexProviderKey is the [model_providers.<key>] entry the harness config uses.
const codexProviderKey = "harness"

// codexConfigTOML is the config.toml that points codex at the gateway.
// requires_openai_auth makes codex send the gateway key from auth.json's
// OPENAI_API_KEY, as the config `tb` writes does.
func codexConfigTOML(gatewayURL, model string) string {
	return fmt.Sprintf(`model = %q
model_provider = %q

[model_providers.%s]
name = "Harness"
base_url = %q
wire_api = "responses"
requires_openai_auth = true
`, model, codexProviderKey, codexProviderKey, gatewayURL)
}

// executeCodexWithEnv writes CODEX_HOME config and runs codex CLI against a pre-configured env.
func executeCodexWithEnv(env *protocoltest.AgentTestEnv, model string, prompt string, timeout time.Duration) (*AgentTestResult, error) {
	start := time.Now()
	result := &AgentTestResult{
		Agent:  "codex",
		Prompt: prompt,
	}

	gatewayURL := env.BaseURL() + "/tingly/codex"
	apiKey := env.ModelToken()

	codexHome, err := os.MkdirTemp("", "harness-codex-*")
	if err != nil {
		return nil, fmt.Errorf("create temp codex home: %w", err)
	}
	defer os.RemoveAll(codexHome)

	configTOML := codexConfigTOML(gatewayURL, model)

	configPath := filepath.Join(codexHome, "config.toml")
	result.SettingsPath = configPath
	if err := os.WriteFile(configPath, []byte(configTOML), 0644); err != nil {
		return nil, fmt.Errorf("write codex config: %w", err)
	}

	authJSON := fmt.Sprintf(`{"auth_mode":"apikey","OPENAI_API_KEY":%q}`, apiKey)
	if err := os.WriteFile(filepath.Join(codexHome, "auth.json"), []byte(authJSON), 0644); err != nil {
		return nil, fmt.Errorf("write codex auth: %w", err)
	}

	binPath, err := exec.LookPath("codex")
	if err != nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Sprintf("codex CLI not found: %v", err)
		result.ExitCode = 1
		return result, nil
	}

	fmt.Printf("🔧 Gateway: %s\n", gatewayURL)
	fmt.Printf("🔧 Config: %s\n", configPath)
	fmt.Printf("🚀 Command: CODEX_HOME=%s codex exec --dangerously-bypass-approvals-and-sandbox %q\n\n", codexHome, prompt)

	cmd := exec.Command(binPath, "exec", "--dangerously-bypass-approvals-and-sandbox", prompt)
	cmd.Env = append(os.Environ(), fmt.Sprintf("CODEX_HOME=%s", codexHome))
	output, timedOut, err := runAgentCmdWithTimeout(cmd, timeout)
	finishAgentResult(result, start, output, timedOut, timeout, err)
	return result, nil
}

// executeOpenCodeWithEnv writes XDG config and runs opencode CLI against a pre-configured env.
//
// The opencode config is built via internalagent.BuildOpenCodeConfig — the
// same logic the production `agent apply` flow uses — so the harness exercises
// the real provider layout (provider key "tingly-box", model map shape)
// instead of a hand-rolled variant that could drift from production.
func executeOpenCodeWithEnv(env *protocoltest.AgentTestEnv, model string, prompt string, timeout time.Duration) (*AgentTestResult, error) {
	start := time.Now()
	result := &AgentTestResult{
		Agent:  "opencode",
		Prompt: prompt,
	}

	// providerKey must match the provider key BuildOpenCodeConfig writes.
	const providerKey = "tingly-box"
	gatewayURL := env.BaseURL() + "/tingly/opencode"
	apiKey := env.ModelToken()

	xdgDir, err := os.MkdirTemp("", "harness-opencode-*")
	if err != nil {
		return nil, fmt.Errorf("create temp xdg dir: %w", err)
	}
	defer os.RemoveAll(xdgDir)

	configDir := filepath.Join(xdgDir, "opencode")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("create opencode config dir: %w", err)
	}

	configContent := internalagent.BuildOpenCodeConfig(gatewayURL, apiKey, map[string]interface{}{
		model: map[string]interface{}{"name": model},
	})
	configJSON, err := json.MarshalIndent(configContent, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal opencode config: %w", err)
	}
	configPath := filepath.Join(configDir, "config.json")
	result.SettingsPath = configPath
	if err := os.WriteFile(configPath, configJSON, 0644); err != nil {
		return nil, fmt.Errorf("write opencode config: %w", err)
	}

	binPath, err := exec.LookPath("opencode")
	if err != nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Sprintf("opencode CLI not found: %v", err)
		result.ExitCode = 1
		return result, nil
	}

	fmt.Printf("🔧 Gateway: %s\n", gatewayURL)
	fmt.Printf("🔧 Config: %s\n", configPath)
	fmt.Printf("🚀 Command: opencode run -m %s/%s %q\n\n", providerKey, model, prompt)

	cmd := exec.Command(binPath, "run", "-m", fmt.Sprintf("%s/%s", providerKey, model), prompt)
	cmd.Env = append(os.Environ(), fmt.Sprintf("XDG_CONFIG_HOME=%s", xdgDir))
	output, timedOut, err := runAgentCmdWithTimeout(cmd, timeout)
	finishAgentResult(result, start, output, timedOut, timeout, err)
	return result, nil
}

// exitCode extracts the exit code from an error
func exitCode(err error) int {
	if exitErr, ok := err.(*exec.ExitError); ok {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
			return status.ExitStatus()
		}
	}
	return 1
}

// AgentTestResult represents the result of a profile test
type AgentTestResult struct {
	Agent        string
	Prompt       string
	Success      bool
	TimedOut     bool
	Duration     time.Duration
	Output       string
	Error        string
	ExitCode     int
	SettingsPath string
}
