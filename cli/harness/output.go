package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// printTable prints test results in tabular format.
func printTable(results []protocol_validate.TestResult, verbose int) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)

	// Summary
	pass, fail, skip := countResults(results)
	fmt.Printf("\n📊 Matrix Test Results\n")
	fmt.Printf("Total: %d | ✓ Pass: %d | ✗ Fail: %d | ⊘ Skip: %d\n",
		len(results), pass, fail, skip)

	// Device info
	fmt.Printf("Host: %s/%s | CPUs: %d | Go: %s | Time: %s\n\n",
		runtime.GOOS, runtime.GOARCH,
		runtime.NumCPU(),
		runtime.Version(),
		time.Now().Format("2006-01-02 15:04:05"))

	// Table header - always show Duration (ms) for consistency
	fmt.Fprintln(w, "Scenario\tSource\tTarget\tStreaming\tStatus\tDuration (ms)")
	fmt.Fprintln(w, "--------\t------\t------\t---------\t------\t-------------")

	for _, r := range results {
		// Unified status format: always show N/M
		batchCount := r.BatchCount
		if batchCount == 0 {
			batchCount = 1
		}
		batchPassed := r.BatchPassed
		if batchPassed == 0 && r.Passed {
			batchPassed = batchCount
		}

		var status string
		if r.Skipped {
			status = fmt.Sprintf("⊘ SKIP: %s", truncateString(r.SkipReason, 30))
		} else if r.Passed {
			status = fmt.Sprintf("✓ PASS %d/%d", batchPassed, batchCount)
		} else {
			status = fmt.Sprintf("✗ FAIL %d/%d", batchPassed, batchCount)
		}

		streaming := "no"
		if r.Streaming {
			streaming = "yes"
		}

		if batchCount > 1 {
			// Batch mode: show min/avg/max
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\tmin:%s avg:%s max:%s\n",
				r.Scenario,
				r.Source,
				r.Target,
				streaming,
				status,
				r.BatchMinDur.Round(time.Millisecond),
				r.BatchAvgDur.Round(time.Millisecond),
				r.BatchMaxDur.Round(time.Millisecond))
		} else {
			// Single mode (batch=1): show just the duration
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\n",
				r.Scenario,
				r.Source,
				r.Target,
				streaming,
				status,
				r.Duration.Round(time.Millisecond))
		}
	}

	w.Flush()

	// Verbose output for failures
	if verbose > 0 {
		printFailures(results, verbose)
	}
}

// printJSON prints test results in JSON format.
func printJSON(results []protocol_validate.TestResult) error {
	type JSONResult struct {
		Name       string `json:"name"`
		Scenario   string `json:"scenario"`
		Source     string `json:"source"`
		Target     string `json:"target"`
		Streaming  bool   `json:"streaming"`
		Status     string `json:"status"`
		SkipReason string `json:"skip_reason,omitempty"`
		Errors     []struct {
			Assertion string `json:"assertion"`
			Error     string `json:"error"`
		} `json:"errors,omitempty"`
		DurationMS int64 `json:"duration_ms"`
	}

	out := make([]JSONResult, len(results))
	for i, r := range results {
		out[i] = JSONResult{
			Name:       r.Name,
			Scenario:   r.Scenario,
			Source:     string(r.Source),
			Target:     string(r.Target),
			Streaming:  r.Streaming,
			DurationMS: r.Duration.Milliseconds(),
		}

		if r.Skipped {
			out[i].Status = "skip"
			out[i].SkipReason = r.SkipReason
		} else if r.Passed {
			out[i].Status = "pass"
		} else {
			out[i].Status = "fail"
			for _, e := range r.Errors {
				out[i].Errors = append(out[i].Errors, struct {
					Assertion string `json:"assertion"`
					Error     string `json:"error"`
				}{
					Assertion: e.Assertion,
					Error:     e.Error,
				})
			}
		}
	}

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(out)
}

// printFailures prints detailed information about failed tests.
func printFailures(results []protocol_validate.TestResult, verbose int) {
	hasFailures := false
	for _, r := range results {
		if !r.Passed && !r.Skipped {
			hasFailures = true
			break
		}
	}

	if !hasFailures {
		return
	}

	fmt.Printf("\n📋 Failed Tests\n")
	fmt.Println(strings.Repeat("=", 70))

	for _, r := range results {
		if !r.Passed && !r.Skipped {
			fmt.Printf("\n✗ %s\n", r.Name)
			for _, e := range r.Errors {
				fmt.Printf("  Assertion failed: %s\n", e.Assertion)
				fmt.Printf("  Error: %s\n", e.Error)
				if verbose > 1 && e.Context != "" {
					fmt.Printf("  Context: %s\n", e.Context)
				}
			}
		}
	}
	fmt.Println()
}

// countResults counts pass/fail/skip totals.
func countResults(results []protocol_validate.TestResult) (pass, fail, skip int) {
	for _, r := range results {
		if r.Skipped {
			skip++
		} else if r.Passed {
			pass++
		} else {
			fail++
		}
	}
	return
}

// truncateString truncates a string to max length.
func truncateString(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
