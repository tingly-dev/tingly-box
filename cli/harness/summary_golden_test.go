package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Pins the on-disk shape of one result row (CSV columns + markdown file) so
// refactors of how results are produced cannot silently change it.
func TestSummaryWriterGoldenOutput(t *testing.T) {
	cases := []struct {
		name string
		r    RealAgentTestResult
		row  []string // every column except timestamp (0) and output_id (10)
		md   string
	}{
		{
			name: "pass",
			r: RealAgentTestResult{
				EntryName: "mock", Agent: "claude", APIStyle: "anthropic",
				Prompt: "What is 2+2?", Model: "cc", RequestModel: "tingly/cc",
				Success: true, Duration: 1500 * time.Millisecond, Output: "4",
			},
			row: []string{"claude", "mock", "cc", "anthropic", "tingly/cc", "", "PASS", "1500", "0", "What is 2+2?", ""},
			md: "# Agent Test Result: mock\n\n**Agent**: claude\n**Entry**: mock\n**Model**: cc\n**API Style**: anthropic\n" +
				"**Request Model**: tingly/cc\n**Status**: PASS\n**Duration**: 1500ms\n**Exit Code**: 0\n\n" +
				"## Prompt\n\nWhat is 2+2?\n\n## Output\n\n4\n\n## Error\n\n(none)\n",
		},
		{
			name: "timeout",
			r: RealAgentTestResult{
				EntryName: "acme/m1", Agent: "codex", APIStyle: "openai", BaseURL: "https://x.test",
				Model: "m1", RequestModel: "tingly-codex", TimedOut: true, ExitCode: -1,
				Duration: 2 * time.Second, Error: "timeout after 2s",
			},
			row: []string{"codex", "acme/m1", "m1", "openai", "tingly-codex", "https://x.test", "TIMEOUT", "2000", "-1", "", "timeout after 2s"},
			md: "# Agent Test Result: acme/m1\n\n**Agent**: codex\n**Entry**: acme/m1\n**Model**: m1\n**API Style**: openai\n" +
				"**Request Model**: tingly-codex\n**Base URL**: https://x.test\n**Status**: TIMEOUT\n**Duration**: 2000ms\n**Exit Code**: -1\n\n" +
				"## Prompt\n\n(empty)\n\n## Output\n\n(empty)\n\n## Error\n\ntimeout after 2s\n",
		},
		{
			name: "fail",
			r: RealAgentTestResult{
				EntryName: "-", Agent: "opencode", Prompt: "hi", ExitCode: 1,
				Output: "boom", Error: "exit status 1",
			},
			row: []string{"opencode", "-", "", "", "", "", "FAIL", "0", "1", "hi", "exit status 1"},
			md: "# Agent Test Result: -\n\n**Agent**: opencode\n**Entry**: -\n**Model**: \n**API Style**: \n" +
				"**Request Model**: \n**Status**: FAIL\n**Duration**: 0ms\n**Exit Code**: 1\n\n" +
				"## Prompt\n\nhi\n\n## Output\n\nboom\n\n## Error\n\nexit status 1\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			csvPath := filepath.Join(dir, "summary.csv")
			outDir := filepath.Join(dir, "out")
			w, err := openSummaryWriter(csvPath, outDir)
			if err != nil {
				t.Fatal(err)
			}
			if err := w.Append(&tc.r); err != nil {
				t.Fatal(err)
			}
			if err := w.Close(); err != nil {
				t.Fatal(err)
			}

			f, err := os.Open(csvPath)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			rows, err := csv.NewReader(f).ReadAll()
			if err != nil {
				t.Fatal(err)
			}
			if len(rows) != 2 {
				t.Fatalf("want header + 1 row, got %d rows", len(rows))
			}
			if strings.Join(rows[0], ",") != strings.Join(summaryCSVColumns, ",") {
				t.Fatalf("header = %v", rows[0])
			}
			got := rows[1]
			if _, err := time.Parse(time.RFC3339, got[0]); err != nil {
				t.Fatalf("timestamp %q: %v", got[0], err)
			}
			outputID := got[10]
			if len(outputID) != 36 {
				t.Fatalf("output_id %q is not a UUID", outputID)
			}
			gotRow := append(append([]string{}, got[1:10]...), got[11:]...)
			if strings.Join(gotRow, "\x00") != strings.Join(tc.row, "\x00") {
				t.Fatalf("row = %q\nwant  %q", gotRow, tc.row)
			}

			files, err := filepath.Glob(filepath.Join(outDir, outputID[:8]+"-*.md"))
			if err != nil || len(files) != 1 {
				t.Fatalf("output file for %s: %v %v", outputID, files, err)
			}
			body, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			if string(body) != tc.md {
				t.Fatalf("markdown =\n%q\nwant\n%q", body, tc.md)
			}
		})
	}
}
