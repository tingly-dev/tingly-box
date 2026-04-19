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

// newAgentCommand creates the profile test subcommand.
func newAgentCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "agent <claude|codex|opencode> [prompt]",
		Short: "Agent-based e2e tests with real agent CLI",
		Long: `Test real profiles by executing actual agent CLI commands.

Agent tests execute real agent commands (claude, codex, opencode)
against virtual models to validate end-to-end functionality:

  - claude:   Execute 'claude --settings <test-settings> -p <prompt>'
  - codex:    Execute 'codex exec -c ... <prompt>'
  - opencode: Execute 'opencode run -m ... <prompt>'

Examples:
  # Test claude with default prompt
  harness profile claude

  # Test claude with custom prompt
  harness profile claude "What is 2+2?"

  # Test opencode profile
  harness profile opencode "Hello, world!"

  # Test with real providers from a config file
  harness profile real claude --models models.yaml`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAgentTest(args)
		},
	}

	cmd.AddCommand(newAgentRealCommand())
	cmd.AddCommand(newInitConfigCommand())

	return cmd
}

// Default test prompts for each profile type
var defaultPrompts = map[string]string{
	"claude":   "What is the capital of France?",
	"codex":    "What is 2+2?",
	"opencode": "Hello, world!",
}

// runAgentTest executes a profile test by running the actual agent CLI command
func runAgentTest(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: harness profile <claude|codex|opencode> [prompt]")
	}

	profileName := args[0]
	prompt := ""
	if len(args) > 1 {
		prompt = strings.Join(args[1:], " ")
	} else {
		prompt = defaultPrompts[profileName]
	}

	// Resolve profile type
	profileType := parseAgentType(profileName)
	if profileType == "" {
		return fmt.Errorf("unknown profile: %s (available: claude, codex, opencode)", profileName)
	}

	fmt.Printf("🧪 Testing profile: %s\n", profileName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Println()

	// Execute the agent command
	result, err := executeAgentCommand(profileType, prompt)
	if err != nil {
		fmt.Printf("❌ Execution failed: %v\n", err)
		return err
	}

	// Print results
	printAgentTestResult(result)

	// Return error if test failed
	if !result.Success {
		return fmt.Errorf("profile test failed")
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

// executeAgentCommand executes the actual agent CLI command
func executeAgentCommand(profileType protocol_validate.AgentType, prompt string) (*AgentTestResult, error) {
	switch profileType {
	case protocol_validate.AgentTypeClaudeCode:
		return executeClaudeTest(prompt)
	case protocol_validate.AgentTypeCodex:
		return executeCodexTest(prompt)
	case protocol_validate.AgentTypeOpenCode:
		return executeOpenCodeTest(prompt)
	default:
		return nil, fmt.Errorf("unknown profile type: %s", profileType)
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
