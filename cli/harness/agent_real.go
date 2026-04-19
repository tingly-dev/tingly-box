package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// RealAgentTestResult holds the result of one real-model Agent run.
type RealAgentTestResult struct {
	EntryName string
	Agent     string
	APIStyle  string
	Prompt    string
	Model     string
	Success   bool
	Duration  time.Duration
	Output    string
	Error     string
	ExitCode  int
}

// newAgentRealCommand creates the `harness Agent real` subcommand.
func newAgentRealCommand() *cobra.Command {
	var modelsFile string
	var prompt string

	cmd := &cobra.Command{
		Use:   "real <claude|codex|opencode>",
		Short: "Test Agent against real providers from a models config file",
		Long: `Test the real agent CLI against actual upstream providers defined in a YAML config file.

For each model entry in the config, the harness:
  1. Starts an isolated gateway instance
  2. Wires the gateway to the real upstream provider
  3. Runs the agent CLI (claude/codex/opencode) against the gateway
  4. Reports pass/fail for each entry

Config file format — YAML (.yaml/.yml):
  models:
    - name: "my-provider"
      baseurl: "https://api.anthropic.com"
      apikey: "sk-ant-..."
      model: "claude-3-5-sonnet-20241022"
      api_style: "anthropic"   # required; auto-detected from baseurl if omitted
      api_type: "anthropic_v1" # optional; defaults based on api_style

Config file format — CSV (.csv, header row required):
  name,baseurl,apikey,model,api_style,api_type
  my-provider,https://api.anthropic.com,sk-ant-...,claude-3-5-sonnet-20241022,anthropic,anthropic_v1

Examples:
  harness Agent real claude --config models.yaml
  harness Agent real claude --config models.csv
  harness Agent real codex --config providers.csv "What is 2+2?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]
			if prompt == "" && len(args) > 1 {
				prompt = strings.Join(args[1:], " ")
			}
			return runRealAgentTests(agentName, modelsFile, prompt)
		},
	}

	cmd.Flags().StringVar(&modelsFile, "config", "models.yaml", "Path to models config YAML file")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send (overrides positional arg and default)")

	return cmd
}

// missingFields returns the names of fields required to run a test that are missing or placeholder.
func missingFields(entry protocol_validate.RealModelEntry) []string {
	var miss []string
	if strings.TrimSpace(entry.BaseURL) == "" {
		miss = append(miss, "baseurl")
	}
	apiKey := strings.TrimSpace(entry.APIKey)
	if apiKey == "" || apiKey == "YOUR_API_KEY" {
		miss = append(miss, "apikey")
	}
	model := strings.TrimSpace(entry.Model)
	if model == "" || model == "MODEL_NAME" {
		miss = append(miss, "model")
	}
	// api_style is now required
	if strings.TrimSpace(entry.APIStyle) == "" {
		miss = append(miss, "api_style")
	}
	return miss
}

// loadRealModelsConfig reads and parses a models config file (YAML or CSV).
func loadRealModelsConfig(path string) (*protocol_validate.RealModelsConfig, error) {
	return protocol_validate.LoadRealModelsConfig(path)
}

// runRealAgentTests iterates over all model entries and runs the agent against each.
func runRealAgentTests(agentName string, modelsFile string, prompt string) error {
	profileType := parseAgentType(agentName)
	if profileType == "" {
		return fmt.Errorf("unknown agent: %q (available: claude, codex, opencode)", agentName)
	}

	if prompt == "" {
		if p, ok := defaultPrompts[agentName]; ok {
			prompt = p
		} else {
			prompt = "What is the capital of France?"
		}
	}

	cfg, err := loadRealModelsConfig(modelsFile)
	if err != nil {
		return err
	}

	// Separate runnable entries from incomplete ones.
	var runnable []protocol_validate.RealModelEntry
	var skipped []string
	for _, entry := range cfg.Models {
		miss := missingFields(entry)
		if len(miss) > 0 {
			skipped = append(skipped, fmt.Sprintf("%s (missing: %s)", entry.Name, strings.Join(miss, ", ")))
		} else {
			runnable = append(runnable, entry)
		}
	}

	if len(skipped) > 0 {
		fmt.Printf("⚠️  Skipping %d incomplete entries:\n", len(skipped))
		for _, s := range skipped {
			fmt.Printf("   • %s\n", s)
		}
		fmt.Println()
	}

	if len(runnable) == 0 {
		return fmt.Errorf("no runnable entries in %s — fill in apikey, baseurl, and model fields", modelsFile)
	}

	fmt.Printf("🧪 Real Agent test: %s\n", agentName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Printf("📋 Models: %d runnable entries from %s\n\n", len(runnable), modelsFile)

	results := make([]*RealAgentTestResult, 0, len(runnable))
	for i, entry := range runnable {
		fmt.Printf("── [%d/%d] %s ──\n", i+1, len(runnable), entry.Name)
		r := runOneRealAgentTest(profileType, entry, prompt)
		results = append(results, r)
		printRealAgentTestResult(r)
		fmt.Println()
	}

	printRealAgentSummary(results)

	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("one or more real Agent tests failed")
		}
	}
	return nil
}

// runOneRealAgentTest runs the agent CLI for a single model entry.
func runOneRealAgentTest(agentType protocol_validate.AgentType, entry protocol_validate.RealModelEntry, prompt string) *RealAgentTestResult {
	result := &RealAgentTestResult{
		EntryName: entry.Name,
		Agent:     string(agentType),
		Prompt:    prompt,
		Model:     entry.Model,
	}
	start := time.Now()

	// Start isolated gateway (virtual server is also started but unused here)
	env, err := protocol_validate.NewAgentTestEnv(agentType)
	if err != nil {
		result.Error = fmt.Sprintf("create test env: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer env.Close(false)

	apiStyle, err := protocol_validate.ResolveAPIStyle(entry)
	if err != nil {
		result.Error = fmt.Sprintf("resolve api_style: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	result.APIStyle = apiStyle
	providerName := fmt.Sprintf("real-%s", entry.Name)

	if err := env.SetupRealAgent(agentType, providerName, entry.Model, entry.BaseURL, entry.APIKey, apiStyle); err != nil {
		result.Error = fmt.Sprintf("setup real Agent: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	// The client CLI must send the gateway rule's RequestModel (the fixed
	// tingly/* name), not entry.Model. The gateway matches the built-in-<agent>
	// rule by RequestModel, then forwards upstream via Service{Provider, Model=entry.Model}.
	// Sending entry.Model directly bypasses rule matching and breaks the
	// provider → rule → service routing contract.
	var requestModel string
	switch agentType {
	case protocol_validate.AgentTypeClaudeCode:
		requestModel = "tingly/cc"
	case protocol_validate.AgentTypeCodex:
		requestModel = "tingly-codex"
	case protocol_validate.AgentTypeOpenCode:
		requestModel = "tingly-opencode"
	default:
		result.Error = fmt.Sprintf("unsupported Agent type: %s", agentType)
		result.Duration = time.Since(start)
		return result
	}

	var agentResult *AgentTestResult
	switch agentType {
	case protocol_validate.AgentTypeClaudeCode:
		agentResult, err = executeClaudeWithEnv(env, requestModel, prompt)
	case protocol_validate.AgentTypeCodex:
		agentResult, err = executeCodexWithEnv(env, requestModel, prompt)
	case protocol_validate.AgentTypeOpenCode:
		agentResult, err = executeOpenCodeWithEnv(env, requestModel, prompt)
	}

	result.Duration = time.Since(start)

	if err != nil {
		result.Error = err.Error()
		return result
	}

	result.Success = agentResult.Success
	result.Output = agentResult.Output
	result.Error = agentResult.Error
	result.ExitCode = agentResult.ExitCode
	return result
}

// printRealAgentTestResult prints the result of one model entry.
func printRealAgentTestResult(r *RealAgentTestResult) {
	duration := fmt.Sprintf("%dms", r.Duration.Milliseconds())
	if r.Success {
		fmt.Printf("✅ PASS  [%s]  model=%s  duration=%s\n", r.EntryName, r.Model, duration)
		if r.Output != "" {
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			limit := 10
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Printf("  %s\n", line)
			}
			if len(lines) > 10 {
				fmt.Printf("  ... (%d more lines)\n", len(lines)-10)
			}
		}
	} else {
		fmt.Printf("❌ FAIL  [%s]  model=%s  duration=%s\n", r.EntryName, r.Model, duration)
		if r.Error != "" {
			fmt.Printf("  Error: %s\n", r.Error)
		}
		if r.Output != "" {
			lines := strings.Split(strings.TrimSpace(r.Output), "\n")
			limit := 5
			if len(lines) < limit {
				limit = len(lines)
			}
			for _, line := range lines[:limit] {
				fmt.Printf("  %s\n", line)
			}
		}
	}
}

// printRealAgentSummary prints a summary table of all real Agent results.
func printRealAgentSummary(results []*RealAgentTestResult) {
	pass, fail := 0, 0
	for _, r := range results {
		if r.Success {
			pass++
		} else {
			fail++
		}
	}

	fmt.Printf("📊 Real Agent Test Summary\n")
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d\n\n", len(results), pass, fail)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Agent\tEntry\tModel\tStatus\tDuration\tAPI Style")
	fmt.Fprintln(w, "-----\t-----\t-----\t------\t--------\t---------")
	for _, r := range results {
		status := "✓ PASS"
		if !r.Success {
			status = "✗ FAIL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Agent,
			r.EntryName,
			r.Model,
			status,
			r.Duration.Round(time.Millisecond),
			r.APIStyle,
		)
	}
	w.Flush()
	fmt.Println()
}
