package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/agentboot/claude"
	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// newAgentCommand creates the agent test subcommand.
func newAgentCommand() *cobra.Command {
	var configFile string
	var useMock bool
	var prompt string

	cmd := &cobra.Command{
		Use:   "agent <claude|codex|opencode|batch> [prompt]",
		Short: "Run end-to-end tests of an agent CLI through the tingly-box gateway",
		Long: `Run an agent CLI (claude, codex, opencode) against a tingly-box gateway
and validate that the full provider → rule → service routing works end-to-end.

Agent argument:

  claude | codex | opencode   Run a single agent
  batch                       Run every supported agent in sequence. Each agent
                              uses its own default prompt unless --prompt or a
                              positional prompt is provided. All agents run even
                              if one fails; the command exits non-zero if any
                              agent failed.

Two modes, selected by an explicit flag:

  --mock                 Virtual-model mode
    Spins up an in-process gateway wired to a virtual upstream (mock responses)
    and runs the real agent CLI against it. Exercises protocol translation and
    rule matching without touching any real upstream.

  --config <file>        Real-provider mode
    Reads a list of real providers from a YAML/CSV config file. For each entry,
    spins up an isolated gateway, registers the provider, binds the built-in
    rule (built-in-cc / built-in-codex / built-in-opencode) to a Service pointing
    at that provider+model, and runs the agent CLI against the gateway. Reports
    pass/fail per entry.

Exactly one of --mock or --config must be supplied.

Config file format — YAML (.yaml/.yml):
  models:
    - name: "my-provider"
      baseurl: "https://api.anthropic.com"
      apikey: "sk-ant-..."
      model: "claude-3-5-sonnet-20241022"
      api_style: "anthropic"   # required; auto-detected from baseurl if omitted

Config file format — CSV (.csv, header row required):
  name,baseurl,apikey,model,api_style
  my-provider,https://api.anthropic.com,sk-ant-...,claude-3-5-sonnet-20241022,anthropic

Examples:
  # Virtual-model mode
  harness agent claude   --mock
  harness agent claude   --mock "What is 2+2?"
  harness agent opencode --mock "Hello, world!"
  harness agent batch    --mock

  # Real-provider mode
  harness agent claude --config models.yaml
  harness agent codex  --config providers.csv "What is 2+2?"
  harness agent batch  --config models.yaml

Generate a config template with: harness init-config`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]
			if prompt == "" && len(args) > 1 {
				prompt = strings.Join(args[1:], " ")
			}
			switch {
			case useMock && configFile != "":
				return fmt.Errorf("--mock and --config are mutually exclusive; pick exactly one")
			case !useMock && configFile == "":
				return fmt.Errorf("must specify a mode: --mock (virtual upstream) or --config <file> (real providers)")
			}

			if strings.EqualFold(agentName, "batch") {
				return runBatchAgentTests(useMock, configFile, prompt)
			}

			if configFile != "" {
				return runRealAgentTests(agentName, configFile, prompt)
			}
			return runVirtualAgentTest(agentName, prompt)
		},
	}

	cmd.Flags().BoolVar(&useMock, "mock", false, "Virtual-model mode: run against an in-process mock upstream")
	cmd.Flags().StringVar(&configFile, "config", "", "Real-provider mode: path to provider config file (YAML or CSV)")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send (overrides positional arg and default)")

	return cmd
}

// Default test prompts for each profile type
var defaultPrompts = map[string]string{
	"claude":   "What is the capital of France?",
	"codex":    "What is 2+2?",
	"opencode": "Hello, world!",
}

// runVirtualAgentTest executes a virtual-model e2e test by running the actual
// agent CLI command against an in-process gateway wired to a mock upstream.
func runVirtualAgentTest(agentName string, prompt string) error {
	if prompt == "" {
		prompt = defaultPrompts[agentName]
	}

	agentType := parseAgentType(agentName)
	if agentType == "" {
		return fmt.Errorf("unknown agent: %q (available: claude, codex, opencode)", agentName)
	}

	fmt.Printf("🧪 Virtual-model test: %s\n", agentName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Println()

	// Execute the agent command
	result, err := executeAgentCommand(agentType, prompt)
	if err != nil {
		fmt.Printf("❌ Execution failed: %v\n", err)
		return err
	}

	// Print results
	printAgentTestResult(result)

	// Return error if test failed
	if !result.Success {
		return fmt.Errorf("virtual-model agent test failed")
	}

	return nil
}

// batchAgents is the ordered list of agents to run in batch mode.
var batchAgents = []string{"claude", "codex", "opencode"}

// runBatchAgentTests runs every supported agent in sequence. All agents run
// regardless of earlier failures; the command returns an error iff any agent
// failed. In virtual mode each agent uses its own default prompt unless
// `prompt` is non-empty. In real mode the same config file is reused across
// agents.
func runBatchAgentTests(useMock bool, configFile string, prompt string) error {
	fmt.Printf("🧪 Batch agent test: %v\n", batchAgents)
	if configFile != "" {
		fmt.Printf("📋 Config: %s\n", configFile)
	} else {
		fmt.Printf("📋 Mode: virtual upstream (--mock)\n")
	}
	if prompt != "" {
		fmt.Printf("📝 Prompt: %s\n", prompt)
	} else {
		fmt.Printf("📝 Prompt: <per-agent default>\n")
	}
	fmt.Println()

	type batchOutcome struct {
		agent string
		err   error
	}
	outcomes := make([]batchOutcome, 0, len(batchAgents))

	for i, agentName := range batchAgents {
		fmt.Printf("══ [%d/%d] agent=%s ══\n", i+1, len(batchAgents), agentName)

		var err error
		switch {
		case configFile != "":
			err = runRealAgentTests(agentName, configFile, prompt)
		case useMock:
			err = runVirtualAgentTest(agentName, prompt)
		default:
			// Caller already validated one of the two flags is set.
			err = fmt.Errorf("internal: batch invoked without mode")
		}
		outcomes = append(outcomes, batchOutcome{agent: agentName, err: err})
		fmt.Println()
	}

	// Aggregate summary.
	fmt.Printf("📊 Batch Summary\n")
	passCount, failCount := 0, 0
	for _, o := range outcomes {
		if o.err == nil {
			passCount++
			fmt.Printf("  ✓ %s\n", o.agent)
		} else {
			failCount++
			fmt.Printf("  ✗ %s — %v\n", o.agent, o.err)
		}
	}
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d\n", len(outcomes), passCount, failCount)

	if failCount > 0 {
		return fmt.Errorf("%d of %d agents failed in batch", failCount, len(outcomes))
	}
	return nil
}

// AgentTestResult represents the result of a profile test
type AgentTestResult struct {
	Agent        string
	Prompt       string
	Success      bool
	Duration     time.Duration
	Output       string
	Error        string
	ExitCode     int
	SettingsPath string
}

// executeAgentCommand executes the actual agent CLI command against the virtual-model gateway.
func executeAgentCommand(agentType protocol_validate.AgentType, prompt string) (*AgentTestResult, error) {
	switch agentType {
	case protocol_validate.AgentTypeClaudeCode:
		return executeClaudeTest(prompt)
	case protocol_validate.AgentTypeCodex:
		return executeCodexTest(prompt)
	case protocol_validate.AgentTypeOpenCode:
		return executeOpenCodeTest(prompt)
	default:
		return nil, fmt.Errorf("unknown agent type: %s", agentType)
	}
}

// executeClaudeTest executes claude CLI backed by an ephemeral gateway + virtual server.
func executeClaudeTest(prompt string) (*AgentTestResult, error) {
	const model = "tingly/cc"

	env, err := protocol_validate.NewAgentTestEnv(protocol_validate.AgentTypeClaudeCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocol_validate.AgentTypeClaudeCode, "virtual-claude", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeClaudeWithEnv(env, model, prompt)
}

// executeClaudeWithEnv writes settings.json and runs claude CLI against a pre-configured env.
func executeClaudeWithEnv(env *protocol_validate.AgentTestEnv, model string, prompt string) (*AgentTestResult, error) {
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

	settings := map[string]interface{}{
		"env": map[string]string{
			"ANTHROPIC_BASE_URL":   env.BaseURL() + "/tingly/claude_code",
			"ANTHROPIC_AUTH_TOKEN": env.ModelToken(),
			"ANTHROPIC_MODEL":      model,
		},
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
	cmd.Env = append(os.Environ())

	output, err := cmd.CombinedOutput()

	result.Duration = time.Since(start)
	result.Output = string(output)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
		result.Success = false
	} else {
		result.Success = true
	}

	return result, nil
}

// executeCodexTest executes codex CLI backed by an ephemeral gateway + virtual server.
func executeCodexTest(prompt string) (*AgentTestResult, error) {
	const model = "tingly-codex"

	env, err := protocol_validate.NewAgentTestEnv(protocol_validate.AgentTypeCodex)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocol_validate.AgentTypeCodex, "virtual-codex", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeCodexWithEnv(env, model, prompt)
}

// executeCodexWithEnv writes CODEX_HOME config and runs codex CLI against a pre-configured env.
func executeCodexWithEnv(env *protocol_validate.AgentTestEnv, model string, prompt string) (*AgentTestResult, error) {
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
	output, err := cmd.CombinedOutput()

	result.Duration = time.Since(start)
	result.Output = string(output)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
		result.Success = false
	} else {
		result.Success = true
	}

	return result, nil
}

// executeOpenCodeTest executes opencode CLI backed by an ephemeral gateway + virtual server.
func executeOpenCodeTest(prompt string) (*AgentTestResult, error) {
	const model = "tingly-opencode"

	env, err := protocol_validate.NewAgentTestEnv(protocol_validate.AgentTypeOpenCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	if err := env.SetupAgent(protocol_validate.AgentTypeOpenCode, "virtual-opencode", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	return executeOpenCodeWithEnv(env, model, prompt)
}

// executeOpenCodeWithEnv writes XDG config and runs opencode CLI against a pre-configured env.
func executeOpenCodeWithEnv(env *protocol_validate.AgentTestEnv, model string, prompt string) (*AgentTestResult, error) {
	start := time.Now()
	result := &AgentTestResult{
		Agent:  "opencode",
		Prompt: prompt,
	}

	const providerKey = "harness"
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

	configContent := map[string]interface{}{
		"$schema": "https://opencode.ai/config.json",
		"provider": map[string]interface{}{
			providerKey: map[string]interface{}{
				"name": providerKey,
				"npm":  "@ai-sdk/anthropic",
				"models": map[string]interface{}{
					model: map[string]interface{}{"name": model},
				},
				"options": map[string]interface{}{
					"apiKey":  apiKey,
					"baseURL": gatewayURL,
				},
			},
		},
	}
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
	fmt.Printf("🚀 Command: opencode run --dangerously-skip-permissions -m %s/%s %q\n\n", providerKey, model, prompt)

	cmd := exec.Command(binPath, "run", "--dangerously-skip-permissions", "-m", fmt.Sprintf("%s/%s", providerKey, model), prompt)
	cmd.Env = append(os.Environ(), fmt.Sprintf("XDG_CONFIG_HOME=%s", xdgDir))
	output, err := cmd.CombinedOutput()

	result.Duration = time.Since(start)
	result.Output = string(output)

	if err != nil {
		result.Error = err.Error()
		result.ExitCode = exitCode(err)
		result.Success = false
	} else {
		result.Success = true
	}

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

// printAgentTestResult prints the test result
func printAgentTestResult(result *AgentTestResult) {
	duration := fmt.Sprintf("%dms", result.Duration.Milliseconds())

	if result.Success {
		fmt.Printf("✅ PASS  %s  Duration: %s\n", result.Agent, duration)
		if result.Output != "" {
			fmt.Printf("┌─────────────────────────────────────┐\n")
			fmt.Printf("│ Output:                              │\n")
			fmt.Printf("└─────────────────────────────────────┘\n")
			lines := strings.Split(strings.TrimSpace(result.Output), "\n")
			for i, line := range lines {
				if i >= 10 {
					fmt.Printf("... (%d more lines)\n", len(lines)-10)
					break
				}
				fmt.Printf("%s\n", line)
			}
		}
	} else {
		fmt.Printf("❌ FAIL  %s  Duration: %s\n", result.Agent, duration)
		fmt.Printf("┌─────────────────────────────────────┐\n")
		fmt.Printf("│ Error:                               │\n")
		fmt.Printf("└─────────────────────────────────────┘\n")
		fmt.Printf("Exit Code: %d\n", result.ExitCode)
		fmt.Printf("Error: %s\n", result.Error)
		if result.Output != "" {
			fmt.Printf("Output:\n%s\n", result.Output)
		}
	}
}

// parseAgentType converts a string to AgentType
func parseAgentType(s string) protocol_validate.AgentType {
	switch strings.ToLower(s) {
	case "claude", "claude-code", "claudecode", "cc":
		return protocol_validate.AgentTypeClaudeCode
	case "codex":
		return protocol_validate.AgentTypeCodex
	case "opencode", "open-code", "oc":
		return protocol_validate.AgentTypeOpenCode
	default:
		return ""
	}
}
