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

// RealProfileTestResult holds the result of one real-model profile run.
type RealProfileTestResult struct {
	EntryName string
	Profile   string
	Prompt    string
	Model     string
	Success   bool
	Duration  time.Duration
	Output    string
	Error     string
	ExitCode  int
}

// newProfileRealCommand creates the `harness profile real` subcommand.
func newProfileRealCommand() *cobra.Command {
	var modelsFile string
	var prompt string

	cmd := &cobra.Command{
		Use:   "real <claude|codex|opencode>",
		Short: "Test profile against real providers from a models config file",
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
      api_style: "anthropic"   # optional; auto-detected from baseurl if omitted

Config file format — CSV (.csv, header row required):
  name,baseurl,apikey,model,api_style
  my-provider,https://api.anthropic.com,sk-ant-...,claude-3-5-sonnet-20241022,anthropic

Examples:
  harness profile real claude --config models.yaml
  harness profile real claude --config models.csv
  harness profile real codex --config providers.csv "What is 2+2?"`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			agentName := args[0]
			if prompt == "" && len(args) > 1 {
				prompt = strings.Join(args[1:], " ")
			}
			return runRealProfileTests(agentName, modelsFile, prompt)
		},
	}

	cmd.Flags().StringVar(&modelsFile, "config", "models.yaml", "Path to models config YAML file")
	cmd.Flags().StringVar(&prompt, "prompt", "", "Prompt to send (overrides positional arg and default)")

	return cmd
}

// loadRealModelsConfig reads and parses a models config file (YAML or CSV).
func loadRealModelsConfig(path string) (*protocol_validate.RealModelsConfig, error) {
	return protocol_validate.LoadRealModelsConfig(path)
}

// runRealProfileTests iterates over all model entries and runs the agent against each.
func runRealProfileTests(agentName string, modelsFile string, prompt string) error {
	profileType := parseProfileType(agentName)
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

	fmt.Printf("🧪 Real profile test: %s\n", agentName)
	fmt.Printf("📝 Prompt: %s\n", prompt)
	fmt.Printf("📋 Models: %d entries from %s\n\n", len(cfg.Models), modelsFile)

	results := make([]*RealProfileTestResult, 0, len(cfg.Models))
	for i, entry := range cfg.Models {
		fmt.Printf("── [%d/%d] %s ──\n", i+1, len(cfg.Models), entry.Name)
		r := runOneRealProfileTest(profileType, entry, prompt)
		results = append(results, r)
		printRealProfileTestResult(r)
		fmt.Println()
	}

	printRealProfileSummary(results)

	for _, r := range results {
		if !r.Success {
			return fmt.Errorf("one or more real profile tests failed")
		}
	}
	return nil
}

// runOneRealProfileTest runs the agent CLI for a single model entry.
func runOneRealProfileTest(profileType protocol_validate.ProfileType, entry protocol_validate.RealModelEntry, prompt string) *RealProfileTestResult {
	result := &RealProfileTestResult{
		EntryName: entry.Name,
		Profile:   string(profileType),
		Prompt:    prompt,
		Model:     entry.Model,
	}
	start := time.Now()

	// Start isolated gateway (virtual server is also started but unused here)
	env, err := protocol_validate.NewProfileTestEnv(profileType)
	if err != nil {
		result.Error = fmt.Sprintf("create test env: %v", err)
		result.Duration = time.Since(start)
		return result
	}
	defer env.Close(false)

	apiStyle := protocol_validate.ResolveAPIStyle(entry)
	providerName := fmt.Sprintf("real-%s", entry.Name)

	if err := env.SetupRealProfile(profileType, providerName, entry.Model, entry.BaseURL, entry.APIKey, apiStyle); err != nil {
		result.Error = fmt.Sprintf("setup real profile: %v", err)
		result.Duration = time.Since(start)
		return result
	}

	var agentResult *ProfileTestResult
	switch profileType {
	case protocol_validate.ProfileTypeClaudeCode:
		agentResult, err = executeClaudeWithEnv(env, entry.Model, prompt)
	case protocol_validate.ProfileTypeCodex:
		agentResult, err = executeCodexWithEnv(env, entry.Model, prompt)
	case protocol_validate.ProfileTypeOpenCode:
		agentResult, err = executeOpenCodeWithEnv(env, entry.Model, prompt)
	default:
		result.Error = fmt.Sprintf("unsupported profile type: %s", profileType)
		result.Duration = time.Since(start)
		return result
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

// printRealProfileTestResult prints the result of one model entry.
func printRealProfileTestResult(r *RealProfileTestResult) {
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

// printRealProfileSummary prints a summary table of all real profile results.
func printRealProfileSummary(results []*RealProfileTestResult) {
	pass, fail := 0, 0
	for _, r := range results {
		if r.Success {
			pass++
		} else {
			fail++
		}
	}

	fmt.Printf("📊 Real Profile Test Summary\n")
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d\n\n", len(results), pass, fail)

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "Entry\tModel\tStatus\tDuration")
	fmt.Fprintln(w, "-----\t-----\t------\t--------")
	for _, r := range results {
		status := "✓ PASS"
		if !r.Success {
			status = "✗ FAIL"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n",
			r.EntryName,
			r.Model,
			status,
			r.Duration.Round(time.Millisecond),
		)
	}
	w.Flush()
	fmt.Println()
}
