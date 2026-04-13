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

// newProfileCommand creates the profile test subcommand.
func newProfileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile <claude|codex|opencode> [prompt]",
		Short: "Profile-based e2e tests with real agent CLI",
		Long: `Test real profiles by executing actual agent CLI commands.

Profile tests execute real agent commands (claude, codex, opencode)
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
  harness profile opencode "Hello, world!"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runProfileTest(args)
		},
	}

	return cmd
}

// Default test prompts for each profile type
var defaultPrompts = map[string]string{
	"claude":   "What is the capital of France?",
	"codex":    "What is 2+2?",
	"opencode": "Hello, world!",
}

// runProfileTest executes a profile test by running the actual agent CLI command
func runProfileTest(args []string) error {
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
	profileType := parseProfileType(profileName)
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
	printProfileTestResult(result)

	// Return error if test failed
	if !result.Success {
		return fmt.Errorf("profile test failed")
	}

	return nil
}

// ProfileTestResult represents the result of a profile test
type ProfileTestResult struct {
	Profile      string
	Prompt       string
	Success      bool
	Duration     time.Duration
	Output       string
	Error        string
	ExitCode     int
	SettingsPath string
}

// executeAgentCommand executes the actual agent CLI command
func executeAgentCommand(profileType protocol_validate.ProfileType, prompt string) (*ProfileTestResult, error) {
	switch profileType {
	case protocol_validate.ProfileTypeClaudeCode:
		return executeClaudeTest(prompt)
	case protocol_validate.ProfileTypeCodex:
		return executeCodexTest(prompt)
	case protocol_validate.ProfileTypeOpenCode:
		return executeOpenCodeTest(prompt)
	default:
		return nil, fmt.Errorf("unknown profile type: %s", profileType)
	}
}

// executeClaudeTest executes claude CLI backed by an ephemeral gateway + virtual server.
// Flow: NewProfileTestEnv → SetupProfile → write settings.json → claude --settings <file> -p <prompt>
func executeClaudeTest(prompt string) (*ProfileTestResult, error) {
	start := time.Now()
	result := &ProfileTestResult{
		Profile: "claude",
		Prompt:  prompt,
	}

	const model = "tingly/cc"

	// 1. Start isolated gateway + virtual server
	env, err := protocol_validate.NewProfileTestEnv(protocol_validate.ProfileTypeClaudeCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	// 2. Register virtual provider + routing rule (requestModel = model = ANTHROPIC_MODEL)
	if err := env.SetupProfile(protocol_validate.ProfileTypeClaudeCode, "virtual-claude", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	// 3. Write settings.json pointing at the ephemeral gateway
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

	// 4. Discover claude binary
	variant, err := claude.FindClaudeCLI(context.Background())
	if err != nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Sprintf("claude CLI not found: %v", err)
		result.ExitCode = 1
		return result, nil
	}

	fmt.Printf("🔧 Gateway: %s\n", env.BaseURL())
	fmt.Printf("🔧 Settings: %s\n", settingsPath)
	fmt.Printf("🚀 Command: claude --settings %s -p %s\n\n", settingsPath, prompt)

	// 5. Execute claude non-interactively
	cmd := exec.Command(variant.Path, "--settings", settingsPath, "-p", prompt)
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

// executeCodexTest executes codex CLI non-interactively backed by an ephemeral gateway + virtual server.
// Flow: NewProfileTestEnv → SetupProfile → codex exec -c model_providers.harness.base_url=<gatewayURL> ... <prompt>
func executeCodexTest(prompt string) (*ProfileTestResult, error) {
	start := time.Now()
	result := &ProfileTestResult{
		Profile: "codex",
		Prompt:  prompt,
	}

	const model = "tingly-codex" // must match built-in-codex RequestModel
	const providerKey = "harness"

	// 1. Start isolated gateway + virtual server
	env, err := protocol_validate.NewProfileTestEnv(protocol_validate.ProfileTypeCodex)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	// 2. Register virtual provider + routing rule
	if err := env.SetupProfile(protocol_validate.ProfileTypeCodex, "virtual-codex", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	gatewayURL := env.BaseURL() + "/tingly/codex"
	apiKey := env.ModelToken()

	binPath, err := exec.LookPath("codex")
	if err != nil {
		result.Duration = time.Since(start)
		result.Error = fmt.Sprintf("codex CLI not found: %v", err)
		result.ExitCode = 1
		return result, nil
	}

	fmt.Printf("🔧 Gateway: %s\n", gatewayURL)
	fmt.Printf("🚀 Command: codex exec -c model_providers.%s.base_url=%s ... %q\n\n", providerKey, gatewayURL, prompt)

	execArgs := []string{
		"exec",
		"-c", fmt.Sprintf("model_providers.%s.name=%s", providerKey, providerKey),
		"-c", fmt.Sprintf("model_providers.%s.base_url=%s", providerKey, gatewayURL),
		"-c", fmt.Sprintf("model_providers.%s.wire_api=responses", providerKey),
		"-c", fmt.Sprintf("model_providers.%s.requires_openai_auth=false", providerKey),
		"-c", fmt.Sprintf("model=%s", model),
		"-c", fmt.Sprintf("model_provider=%s", providerKey),
		"-c", fmt.Sprintf("provider_api_keys.%s=%s", providerKey, apiKey),
		"--dangerously-bypass-approvals-and-sandbox",
		prompt,
	}

	cmd := exec.Command(binPath, execArgs...)
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

// executeOpenCodeTest executes opencode CLI non-interactively backed by an ephemeral gateway + virtual server.
// Flow: NewProfileTestEnv → SetupProfile → XDG_CONFIG_HOME=<tmpdir> opencode run --dangerously-skip-permissions -m harness/<model> <prompt>
func executeOpenCodeTest(prompt string) (*ProfileTestResult, error) {
	start := time.Now()
	result := &ProfileTestResult{
		Profile: "opencode",
		Prompt:  prompt,
	}

	const model = "tingly-opencode" // must match built-in-opencode RequestModel
	const providerKey = "harness"

	// 1. Start isolated gateway + virtual server
	env, err := protocol_validate.NewProfileTestEnv(protocol_validate.ProfileTypeOpenCode)
	if err != nil {
		return nil, fmt.Errorf("create test env: %w", err)
	}
	defer env.Close(false)

	// 2. Register virtual provider + routing rule
	if err := env.SetupProfile(protocol_validate.ProfileTypeOpenCode, "virtual-opencode", model); err != nil {
		return nil, fmt.Errorf("setup profile: %w", err)
	}

	gatewayURL := env.BaseURL() + "/tingly/opencode"
	apiKey := env.ModelToken()

	// 3. Write opencode config.json into a temp XDG dir
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

// printProfileTestResult prints the test result
func printProfileTestResult(result *ProfileTestResult) {
	duration := fmt.Sprintf("%dms", result.Duration.Milliseconds())

	if result.Success {
		fmt.Printf("✅ PASS  %s  Duration: %s\n", result.Profile, duration)
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
		fmt.Printf("❌ FAIL  %s  Duration: %s\n", result.Profile, duration)
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

// parseProfileType converts a string to ProfileType
func parseProfileType(s string) protocol_validate.ProfileType {
	switch strings.ToLower(s) {
	case "claude", "claude-code", "claudecode", "cc":
		return protocol_validate.ProfileTypeClaudeCode
	case "codex":
		return protocol_validate.ProfileTypeCodex
	case "opencode", "open-code", "oc":
		return protocol_validate.ProfileTypeOpenCode
	default:
		return ""
	}
}
