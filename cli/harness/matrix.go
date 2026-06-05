package main

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocoltest"
)

// MatrixCmd runs the protocol validation matrix tests.
//
// The set of (source → target) pairs is defined explicitly in
// internal/protocoltest.DefaultPairs() rather than as a Cartesian
// product, so what's exercised matches the dispatch graph documented
// in internal/protocol/README.md. Each pair runs against every
// scenario and both streaming modes.
//
// --source and --target filter pairs by their source/target component.
type MatrixCmd struct {
	Scenarios  []string `kong:"name='scenario',sep=',',help='Filter by scenario name (can repeat or comma-separate)'"`
	Sources    []string `kong:"name='source',sep=',',help='Filter by source protocol (can repeat or comma-separate)'"`
	Targets    []string `kong:"name='target',sep=',',help='Filter by target protocol (can repeat or comma-separate)'"`
	Streaming  bool   `kong:"name='streaming',help='Run only streaming tests'"`
	NonStream  bool   `kong:"name='non-streaming',help='Run only non-streaming tests'"`
	Mode       string `kong:"name='mode',default='all',enum='all,single,transitive',help='Hop selection: all (default), single (A→B only), transitive (A→B→C only)'"`
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
  # Run everything: single-hop + two-hop (default)
  harness matrix

  # Run only two-hop (A→B→C) transitive chain tests
  harness matrix --mode=transitive

  # Run only single-hop (A→B) tests
  harness matrix --mode=single

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

	// Resolve flag conflicts early.
	if m.Streaming && m.NonStream {
		return fmt.Errorf("cannot specify both --streaming and --non-streaming")
	}

	// Build matrix with filters
	matrix := protocoltest.DefaultMatrix()

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

	// Collect results for selected hop sections (--mode controls which).
	var combined []protocoltest.TestResult
	if m.Mode != "transitive" {
		combined = append(combined, matrix.ExecuteAll()...)
	}
	if m.Mode != "single" {
		combined = append(combined, matrix.ExecuteAllTransitive()...)
	}
	results := filterResults(combined, m)

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
func filterResults(results []protocoltest.TestResult, m *MatrixCmd) []protocoltest.TestResult {
	var filtered []protocoltest.TestResult

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

