package summarizer

import (
	"strings"
)

// Engine handles summary extraction from Claude Code output
type Engine struct{}

// NewEngine creates a new summarization engine
func NewEngine() *Engine {
	return &Engine{}
}

// Summarize generates a summary from Claude Code output
func (e *Engine) Summarize(output string) string {
	if output == "" {
		return "No output generated."
	}

	lines := strings.Split(output, "\n")
	summary := e.extractKeyInformation(lines)

	if summary == "" {
		// Fallback: first few lines
		if len(lines) > 3 {
			summary = strings.Join(lines[:3], "\n") + "\n... (truncated)"
		} else {
			summary = output
		}
	}

	return summary
}

// extractKeyInformation extracts key information from the output
func (e *Engine) extractKeyInformation(lines []string) string {
	var keyLines []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip very long lines (likely code blocks)
		if len(trimmed) > 500 {
			continue
		}

		// Include lines that look like summary/content
		if e.isSummaryLine(trimmed) {
			keyLines = append(keyLines, trimmed)
		}
	}

	// If we found key lines, join them
	if len(keyLines) > 0 {
		// Limit to first 10 lines
		if len(keyLines) > 10 {
			keyLines = keyLines[:10]
		}
		return strings.Join(keyLines, "\n")
	}

	return ""
}

// isSummaryLine determines if a line contains summary-worthy content
func (e *Engine) isSummaryLine(line string) bool {
	// Skip file paths
	if strings.HasPrefix(line, "/") || strings.Contains(line, ":") {
		return false
	}

	// Skip command-like lines
	if strings.HasPrefix(line, "$ ") || strings.HasPrefix(line, "> ") {
		return false
	}

	// Skip timestamp patterns
	if len(line) > 20 && strings.Contains(line, "[") && strings.Contains(line, "]") {
		return false
	}

	// Skip ANSI color codes
	if strings.Contains(line, "\x1b[") {
		return false
	}

	return true
}

// ExtractActionItems extracts action items or TODO items from output
func (e *Engine) ExtractActionItems(output string) []string {
	var items []string
	lines := strings.Split(output, "\n")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, "TODO") ||
			strings.Contains(trimmed, "[ ]") ||
			strings.HasPrefix(trimmed, "- [ ]") {
			items = append(items, trimmed)
		}
	}

	return items
}

// CountTokens estimates token count from output (rough approximation)
func (e *Engine) CountTokens(output string) int {
	// Rough estimate: 4 characters per token on average
	return len(output) / 4
}
