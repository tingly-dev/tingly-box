package main

import (
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

type matrixOptions struct {
	scenarios  []string
	sources    []string
	targets    []string
	streaming  bool
	nonStream  bool
	jsonOutput bool
	verbose    int
	recordDir  string // Directory for recording requests/responses
	serverMode string // Server reuse mode: auto (per-scenario), all, pair
	batchCount int    // Number of times to run each test (for batch testing)
}

// newMatrixCommand creates the matrix test subcommand.
func newMatrixCommand() *cobra.Command {
	opts := &matrixOptions{}

	cmd := &cobra.Command{
		Use:   "matrix",
		Short: "Run protocol validation matrix tests",
		Long: `Run protocol validation matrix tests with virtual providers.

Tests all combinations of:
  - Source protocols (anthropic_v1, anthropic_beta, openai_chat, openai_responses)
  - Target protocols (anthropic_v1, anthropic_beta, openai_chat, openai_responses, google)
  - Scenarios (text, tool_use, tool_result, thinking, multi_turn, streaming_*)
  - Streaming modes (streaming, non-streaming)

Use flags to filter specific combinations.`,
		Example: `  # Run all matrix tests
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
  harness matrix -vv`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runMatrix(opts)
		},
	}

	cmd.Flags().StringSliceVar(&opts.scenarios, "scenario", nil, "Filter by scenario name (can repeat)")
	cmd.Flags().StringSliceVar(&opts.sources, "source", nil, "Filter by source protocol")
	cmd.Flags().StringSliceVar(&opts.targets, "target", nil, "Filter by target protocol")
	cmd.Flags().BoolVar(&opts.streaming, "streaming", false, "Run only streaming tests")
	cmd.Flags().BoolVar(&opts.nonStream, "non-streaming", false, "Run only non-streaming tests")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "Output results as JSON")
	cmd.Flags().CountVarP(&opts.verbose, "verbose", "v", "Verbose output (can repeat for more detail)")
	cmd.Flags().StringVar(&opts.recordDir, "record-dir", os.Getenv("HARNESS_RECORD_DIR"), "Directory for recording requests/responses (default: disabled)")
	cmd.Flags().StringVar(&opts.serverMode, "server-mode", "auto", "Server reuse mode: auto (per-scenario), all (single server), pair (per source-target)")
	cmd.Flags().IntVar(&opts.batchCount, "batch", 1, "Number of times to run each test (for stability/performance testing)")

	return cmd
}

// runMatrix executes the matrix tests with the given options.
func runMatrix(opts *matrixOptions) error {
	// Set log level based on verbose flag
	// Default (v=0): Warn level - minimal output
	// v=1: Info level - normal output
	// v=2+: Debug level - detailed output
	switch opts.verbose {
	case 0:
		logrus.SetLevel(logrus.WarnLevel)
	case 1:
		logrus.SetLevel(logrus.InfoLevel)
	default:
		logrus.SetLevel(logrus.DebugLevel)
	}

	// Build matrix with filters
	matrix := protocol_validate.DefaultMatrix()

	// Apply filters to limit execution (not just display)
	if len(opts.scenarios) > 0 {
		matrix = matrix.OnlyScenarios(opts.scenarios...)
	}
	if len(opts.sources) > 0 {
		matrix = matrix.OnlySources(opts.sources...)
	}
	if len(opts.targets) > 0 {
		matrix = matrix.OnlyTargets(opts.targets...)
	}
	if opts.streaming {
		matrix = matrix.OnlyStreaming(true)
	}
	if opts.nonStream {
		matrix = matrix.OnlyStreaming(false)
	}

	// Set record directory if provided
	if opts.recordDir != "" {
		matrix = matrix.WithRecordDir(opts.recordDir)
	}

	// Set server mode
	if opts.serverMode != "" && opts.serverMode != "auto" {
		matrix = matrix.WithServerMode(opts.serverMode)
	}

	// Set batch count
	if opts.batchCount > 1 {
		matrix = matrix.WithBatchCount(opts.batchCount)
	}

	// Resolve streaming filter conflict
	if opts.streaming && opts.nonStream {
		return fmt.Errorf("cannot specify both --streaming and --non-streaming")
	}

	// Execute tests (only filtered combinations)
	results := matrix.ExecuteAll()

	// Note: No need to filter results here since we filtered before execution
	// But keep filterResults for backward compatibility in case results contain
	// additional entries from skipPairs or skipSourceScenarios
	results = filterResults(results, opts)

	// Output results
	if opts.jsonOutput {
		if err := printJSON(results); err != nil {
			return fmt.Errorf("failed to output JSON: %w", err)
		}
	} else {
		printTable(results, opts.verbose)
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
func filterResults(results []protocol_validate.TestResult, opts *matrixOptions) []protocol_validate.TestResult {
	var filtered []protocol_validate.TestResult

	for _, r := range results {
		// Filter by source
		if len(opts.sources) > 0 && !contains(opts.sources, string(r.Source)) {
			continue
		}

		// Filter by target
		if len(opts.targets) > 0 && !contains(opts.targets, string(r.Target)) {
			continue
		}

		// Filter by streaming
		if opts.streaming && !r.Streaming {
			continue
		}
		if opts.nonStream && r.Streaming {
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
