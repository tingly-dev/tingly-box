package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// DefaultOutputDir is the directory where individual output files are written.
const DefaultOutputDir = "harness-output"

// outputWriter manages writing test output to individual markdown files.
// Each test result gets a unique UUID and a corresponding .md file containing
// the full prompt, output, and error details.
type outputWriter struct {
	dir string // output directory path
}

// openOutputWriter creates (or opens) the output directory and initializes
// the writer. No ID state needs to be tracked since we use UUIDs.
func openOutputWriter(dir string) (*outputWriter, error) {
	if dir == "" {
		dir = DefaultOutputDir
	}

	// Create directory if it doesn't exist
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("create output directory %s: %w", dir, err)
	}

	return &outputWriter{dir: dir}, nil
}

// Write creates a markdown file for the given test result and returns the
// assigned UUID. The file is named "{uuid[:8]}-{entry}.md" for easy identification.
func (o *outputWriter) Write(r *RealAgentTestResult) (id string, err error) {
	if o == nil {
		return "", nil
	}

	// Generate UUID for this output
	id = uuid.New().String()

	// Use short UUID (first 8 chars) for filename to keep it manageable
	shortID := id[:8]
	safeEntry := sanitizeFilename(r.EntryName)
	filename := fmt.Sprintf("%s-%s.md", shortID, safeEntry)
	path := filepath.Join(o.dir, filename)

	// Build markdown content
	var content strings.Builder

	content.WriteString(fmt.Sprintf("# Agent Test Result: %s\n\n", r.EntryName))
	content.WriteString(fmt.Sprintf("**Agent**: %s\n", r.Agent))
	content.WriteString(fmt.Sprintf("**Entry**: %s\n", r.EntryName))
	content.WriteString(fmt.Sprintf("**Model**: %s\n", r.Model))
	content.WriteString(fmt.Sprintf("**API Style**: %s\n", r.APIStyle))
	content.WriteString(fmt.Sprintf("**Request Model**: %s\n", r.RequestModel))
	if r.BaseURL != "" {
		content.WriteString(fmt.Sprintf("**Base URL**: %s\n", r.BaseURL))
	}
	content.WriteString(fmt.Sprintf("**Status**: %s\n", getStatus(r)))
	content.WriteString(fmt.Sprintf("**Duration**: %dms\n", r.Duration.Milliseconds()))
	content.WriteString(fmt.Sprintf("**Exit Code**: %d\n\n", r.ExitCode))

	// Prompt section
	content.WriteString("## Prompt\n\n")
	if r.Prompt != "" {
		content.WriteString(r.Prompt)
	} else {
		content.WriteString("(empty)")
	}
	content.WriteString("\n\n")

	// Output section
	content.WriteString("## Output\n\n")
	if r.Output != "" {
		content.WriteString(r.Output)
	} else {
		content.WriteString("(empty)")
	}
	content.WriteString("\n\n")

	// Error section
	content.WriteString("## Error\n\n")
	if r.Error != "" {
		content.WriteString(r.Error)
	} else {
		content.WriteString("(none)")
	}
	content.WriteString("\n")

	// Write file
	if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
		return "", fmt.Errorf("write output file %s: %w", path, err)
	}

	return id, nil
}

// Close is a no-op for outputWriter (files are written immediately).
// Kept for API compatibility with summaryWriter.
func (o *outputWriter) Close() error {
	return nil
}

// sanitizeFilename converts an entry name to a safe filename by replacing
// slashes and other problematic characters with underscores.
func sanitizeFilename(name string) string {
	// Replace problematic characters with underscore
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, ch := range unsafe {
		result = strings.ReplaceAll(result, ch, "_")
	}
	// Limit length to avoid filesystem issues
	if len(result) > 50 {
		result = result[:50]
	}
	return strings.TrimSpace(result)
}

// getStatus returns a human-readable status string for the result.
func getStatus(r *RealAgentTestResult) string {
	switch {
	case r.Success:
		return "PASS"
	case r.TimedOut:
		return "TIMEOUT"
	default:
		return "FAIL"
	}
}
