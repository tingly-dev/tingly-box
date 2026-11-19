package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/claude"
	internalagent "github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// agentRunTimeout is the max wall-clock duration for a single agent CLI
// invocation. Set by the `--timeout` flag in newAgentCommand before any work
// runs. A zero value disables the timeout. Exposed as a package-level var
// (rather than threaded through every executor signature) because it's a
// cross-cutting concern shared by all agent paths — mock, real, batch.
var agentRunTimeout time.Duration

// runAgentCmdWithTimeout runs cmd, capturing stdout+stderr into one buffer,
// under the configured per-entry timeout. Returns the captured output, a
// timed-out flag, and the Wait/Start error (if any). On timeout the process
// is killed and context.DeadlineExceeded is returned as the error.
// When agentRunTimeout <= 0 the timeout is disabled and this behaves like
// cmd.CombinedOutput().
func runAgentCmdWithTimeout(cmd *exec.Cmd) ([]byte, bool, error) {
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf

	if agentRunTimeout <= 0 {
		err := cmd.Run()
		return buf.Bytes(), false, err
	}

	if err := cmd.Start(); err != nil {
		return buf.Bytes(), false, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case <-time.After(agentRunTimeout):
		_ = cmd.Process.Kill()
		<-done // reap the goroutine so it doesn't leak
		return buf.Bytes(), true, context.DeadlineExceeded
	case err := <-done:
		return buf.Bytes(), false, err
	}
}

// finishAgentResult records the shared outcome fields of one agent CLI run —
// the single implementation of the timeout / error / success classification
// every executor uses.
func finishAgentResult(result *AgentTestResult, start time.Time, output []byte, timedOut bool, err error) {
	result.Duration = time.Since(start)
	result.Output = string(output)
	switch {
	case timedOut:
		result.Error = fmt.Sprintf("timeout after %s", agentRunTimeout)
		result.ExitCode = -1
		result.TimedOut = true
	case err != nil:
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
	default:
		result.Success = true
	}
}

// executeAgentCommand executes the actual agent CLI command against the virtual-model gateway.
func executeAgentCommand(agentType protocoltest.AgentType, prompt string) (*AgentTestResult, error) {
	switch agentType {
	case protocoltest.AgentTypeClaudeCode:
		return executeClaudeTest(prompt)
	case protocoltest.AgentTypeCodex:
		return executeCodexTest(prompt)
	case protocoltest.AgentTypeOpenCode:
		return executeOpenCodeTest(prompt)
	default:
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
}

// executeClaudeTest executes claude CLI backed by an ephemeral gateway + virtual server.
func executeClaudeTest(prompt string) (*AgentTestResult, error) {
	const model = "cc"

	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeClaudeCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocoltest.AgentTypeClaudeCode, "virtual-claude", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeClaudeWithEnv(env, prompt)
}

// executeClaudeWithEnv writes settings.json and runs claude CLI against a pre-configured env.
//
// The settings env is built via internalagent.DefaultClaudeCodePrefs — the
// same canonical defaults the GUI quick-config seeds from — so the harness
// exercises the real Claude Code configuration shape (telemetry flags,
// model routing vars) rather than a hand-rolled minimal subset.
func executeClaudeWithEnv(env *protocoltest.AgentTestEnv, prompt string) (*AgentTestResult, error) {
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

	output, timedOut, err := runAgentCmdWithTimeout(cmd)
	finishAgentResult(result, start, output, timedOut, err)
	return result, nil
}

// executeCodexTest executes codex CLI backed by an ephemeral gateway + virtual server.
func executeCodexTest(prompt string) (*AgentTestResult, error) {
	const model = "tingly-codex"

	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeCodex)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocoltest.AgentTypeCodex, "virtual-codex", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeCodexWithEnv(env, model, prompt)
}

// executeCodexWithEnv writes CODEX_HOME config and runs codex CLI against a pre-configured env.
func executeCodexWithEnv(env *protocoltest.AgentTestEnv, model string, prompt string) (*AgentTestResult, error) {
	start := time.Now()
	result := &AgentTestResult{
		Agent:  "codex",
		Prompt: prompt,
	}

	const providerKey = "harness"
	gatewayURL := env.BaseURL() + "/tingly/codex"
	apiKey := env.ModelToken()

	codexHome, err := os.MkdirTemp("", "harness-codex-*")
	if err != nil {
		return nil, fmt.Errorf("create temp codex home: %w", err)
	}
	defer os.RemoveAll(codexHome)

	configTOML := fmt.Sprintf(`model = %q
model_provider = %q
disable_response_storage = true

[model_providers.%s]
name = "Harness"
base_url = %q
wire_api = "responses"
`, model, providerKey, providerKey, gatewayURL)

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
	output, timedOut, err := runAgentCmdWithTimeout(cmd)
	finishAgentResult(result, start, output, timedOut, err)
	return result, nil
}

// executeOpenCodeTest executes opencode CLI backed by an ephemeral gateway + virtual server.
func executeOpenCodeTest(prompt string) (*AgentTestResult, error) {
	const model = "tingly-opencode"

	env, err := protocoltest.NewAgentTestEnv(protocoltest.AgentTypeOpenCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocoltest.AgentTypeOpenCode, "virtual-opencode", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeOpenCodeWithEnv(env, model, prompt)
}

// executeOpenCodeWithEnv writes XDG config and runs opencode CLI against a pre-configured env.
//
// The opencode config is built via internalagent.BuildOpenCodeConfig — the
// same logic the production `agent apply` flow uses — so the harness exercises
// the real provider layout (provider key "tingly-box", model map shape)
// instead of a hand-rolled variant that could drift from production.
func executeOpenCodeWithEnv(env *protocoltest.AgentTestEnv, model string, prompt string) (*AgentTestResult, error) {
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
	output, timedOut, err := runAgentCmdWithTimeout(cmd)
	finishAgentResult(result, start, output, timedOut, err)
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
