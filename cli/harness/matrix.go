package main

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// MatrixCmd runs the protocol validation matrix tests.
//
// Tests all combinations of:
//   - Source protocols (anthropic_v1, anthropic_beta, openai_chat, openai_responses)
//   - Target protocols (anthropic_v1, anthropic_beta, openai_chat, openai_responses, google)
//   - Scenarios (text, tool_use, tool_result, thinking, multi_turn, streaming_*)
//   - Streaming modes (streaming, non-streaming)
//
// Use flags to filter specific combinations.
type MatrixCmd struct {
	Scenarios  []string `kong:"name='scenario',sep=',',help='Filter by scenario name (can repeat or comma-separate)'"`
	Sources    []string `kong:"name='source',sep=',',help='Filter by source protocol (can repeat or comma-separate)'"`
	Targets    []string `kong:"name='target',sep=',',help='Filter by target protocol (can repeat or comma-separate)'"`
	Streaming  bool     `kong:"name='streaming',help='Run only streaming tests'"`
	NonStream  bool     `kong:"name='non-streaming',help='Run only non-streaming tests'"`
	JsonOutput bool     `kong:"name='json',help='Output results as JSON'"`
	Verbose    int      `kong:"name='verbose',short='v',type='counter',help='Verbose output (repeat for more detail)'"`
	RecordDir  string   `kong:"name='record-dir',env='HARNESS_RECORD_DIR',help='Directory for recording requests/responses (default: disabled)'"`
	ServerMode string   `kong:"name='server-mode',default='auto',help='Server reuse mode: auto (per-scenario), all (single server), pair (per source-target)'"`
	BatchCount int      `kong:"name='batch',default='1',help='Number of times to run each test (for stability/performance testing)'"`
	MCPEnabled bool     `kong:"name='mcp',help='Enable MCP feature flag in test env'"`
}

// Help returns extended help text shown by `harness matrix --help`.
func (*MatrixCmd) Help() string {
	return `Examples:
  # Run all matrix tests
  harness matrix

  # Run specific scenario only
  harness matrix --scenario text

  # Run all scenarios for specific source/target
  harness matrix --source anthropic_v1 --target openai_chat

  # Run only streaming tests
  harness matrix --streaming

  # JSON output for CI/CD
  harness matrix --json

  # Verbose output with details
  harness matrix -vv`
}

// Run executes the matrix tests with the parsed flags.
func (m *MatrixCmd) Run() error {
	verbose := m.Verbose

	// Set log level based on verbose flag
	// Default (v=0): Warn level - minimal output
	// v=1: Info level - normal output
	// v=2+: Debug level - detailed output
	switch verbose {
	case 0:
		logrus.SetLevel(logrus.WarnLevel)
	case 1:
		logrus.SetLevel(logrus.InfoLevel)
	default:
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Resolve streaming filter conflict early.
	if m.Streaming && m.NonStream {
		return fmt.Errorf("cannot specify both --streaming and --non-streaming")
	}

	// Build matrix with filters
	matrix := protocol_validate.DefaultMatrix()

	if len(m.Scenarios) > 0 {
		matrix = matrix.OnlyScenarios(m.Scenarios...)
	}
	if len(m.Sources) > 0 {
		matrix = matrix.OnlySources(m.Sources...)
	}
	if len(m.Targets) > 0 {
		matrix = matrix.OnlyTargets(m.Targets...)
	}
	if m.Streaming {
		matrix = matrix.OnlyStreaming(true)
	}
	if m.NonStream {
		matrix = matrix.OnlyStreaming(false)
	}
	if m.RecordDir != "" {
		matrix = matrix.WithRecordDir(m.RecordDir)
	}
	if m.ServerMode != "" && m.ServerMode != "auto" {
		matrix = matrix.WithServerMode(m.ServerMode)
	}
	if m.BatchCount > 1 {
		matrix = matrix.WithBatchCount(m.BatchCount)
	}
	if m.MCPEnabled {
		matrix = matrix.WithMCPEnabled()
	}

	// Execute tests (only filtered combinations).
	results := matrix.ExecuteAll()

	// Filter results for backward compatibility (skipPairs etc.).
	results = filterResults(results, m)

	// Output results
	if m.JsonOutput {
		if err := printJSON(results); err != nil {
			return fmt.Errorf("failed to output JSON: %w", err)
		}
	} else {
		printTable(results, verbose)
	}

	// Determine exit code
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			return fmt.Errorf("some tests failed")
		}
	}
	return nil
}

// filterResults filters test results based on command options.
func filterResults(results []protocol_validate.TestResult, m *MatrixCmd) []protocol_validate.TestResult {
	var filtered []protocol_validate.TestResult

	for _, r := range results {
		if len(m.Sources) > 0 && !contains(m.Sources, string(r.Source)) {
			continue
		}
		if len(m.Targets) > 0 && !contains(m.Targets, string(r.Target)) {
			continue
		}
		if m.Streaming && !r.Streaming {
			continue
		}
		if m.NonStream && r.Streaming {
			continue
		}
		filtered = append(filtered, r)
	}

	return filtered
}

// contains checks if a string slice contains a specific value.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

