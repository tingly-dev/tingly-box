package summarizer

import (
	"strings"
	"time"
)

// Engine handles summary extraction from Claude Code output
type Engine struct {
	templates map[string]SummaryTemplate // Summary templates for different use cases
}

// SummaryTemplate defines a summary template for different scenarios
type SummaryTemplate struct {
	Name        string // Template name
	Description string // Template description
	Success     string // Template for successful execution
	Error       string // Template for failed execution
	Partial     string // Template for partial success
}

// ToolCallContext represents context about a tool call
type ToolCallContext struct {
	ToolName  string        // Name of the tool
	Arguments string        // Arguments passed to tool
	Result    string        // Result/output from tool
	Success   bool          // Whether tool call succeeded
	Duration  time.Duration // How long the tool call took
}

// NewEngine creates a new summarization engine
func NewEngine() *Engine {
	engine := &Engine{
		templates: make(map[string]SummaryTemplate),
	}
	engine.initializeTemplates()
	return engine
}

// initializeTemplates initializes default summary templates
func (e *Engine) initializeTemplates() {
	// SmartGuide templates
	e.templates["smartguide_success"] = SummaryTemplate{
		Name:        "SmartGuide Success",
		Description: "Template for successful SmartGuide operations",
		Success:     "Successfully completed the requested task.",
	}
	e.templates["smartguide_error"] = SummaryTemplate{
		Name:        "SmartGuide Error",
		Description: "Template for failed SmartGuide operations",
		Error:       "Task could not be completed due to errors.",
	}
	e.templates["smartguide_partial"] = SummaryTemplate{
		Name:        "SmartGuide Partial",
		Description: "Template for partial success",
		Partial:     "Task partially completed with some issues.",
	}

	// Git operation templates
	e.templates["git_clone"] = SummaryTemplate{
		Name:        "Git Clone",
		Description: "Template for git clone operations",
		Success:     "Successfully cloned repository.",
		Error:       "Failed to clone repository.",
	}

	// Project setup templates
	e.templates["project_setup"] = SummaryTemplate{
		Name:        "Project Setup",
		Description: "Template for project setup operations",
		Success:     "Successfully set up project.",
		Error:       "Failed to set up project.",
	}
}

// GetTemplate retrieves a template by name
func (e *Engine) GetTemplate(name string) (SummaryTemplate, bool) {
	template, ok := e.templates[name]
	return template, ok
}

// RegisterTemplate registers a new template
func (e *Engine) RegisterTemplate(name string, template SummaryTemplate) {
	e.templates[name] = template
}

// SummarizeToolCalls summarizes a list of tool calls
func (e *Engine) SummarizeToolCalls(toolCalls []ToolCallContext) []string {
	var summaries []string

	for _, call := range toolCalls {
		summary := e.formatToolCall(call)
		summaries = append(summaries, summary)
	}

	return summaries
}

// formatToolCall formats a single tool call
func (e *Engine) formatToolCall(call ToolCallContext) string {
	switch call.ToolName {
	case "bash_cd":
		return formatString("Changed directory to: %s", call.Arguments)
	case "bash_ls":
		return "Listed directory contents"
	case "bash_pwd":
		return formatString("Current directory: %s", call.Result)
	case "git_clone":
		return formatString("Cloned repository: %s", call.Arguments)
	case "git_status":
		return "Checked git status"
	case "get_status":
		return "Retrieved current status"
	case "get_project":
		return formatString("Retrieved project info: %s", call.Result)
	default:
		return formatString("%s: %s", call.ToolName, call.Arguments)
	}
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

// ExtractToolNames extracts unique tool names from tool calls
func (e *Engine) ExtractToolNames(toolCalls []ToolCallContext) []string {
	seen := make(map[string]bool)
	names := []string{}

	for _, call := range toolCalls {
		if !seen[call.ToolName] {
			seen[call.ToolName] = true
			names = append(names, call.ToolName)
		}
	}

	return names
}

// FilterSuccessfulToolCalls filters only successful tool calls
func (e *Engine) FilterSuccessfulToolCalls(toolCalls []ToolCallContext) []ToolCallContext {
	var successful []ToolCallContext
	for _, call := range toolCalls {
		if call.Success {
			successful = append(successful, call)
		}
	}
	return successful
}

// FilterFailedToolCalls filters only failed tool calls
func (e *Engine) FilterFailedToolCalls(toolCalls []ToolCallContext) []ToolCallContext {
	var failed []ToolCallContext
	for _, call := range toolCalls {
		if !call.Success {
			failed = append(failed, call)
		}
	}
	return failed
}

// Helper function for string formatting
func formatString(format string, args ...string) string {
	if len(args) == 0 {
		return format
	}
	result := format
	for _, arg := range args {
		placeholder := "%s"
		// Replace first occurrence
		idx := strings.Index(result, placeholder)
		if idx != -1 {
			result = result[:idx] + arg + result[idx+len(placeholder):]
		}
	}
	return result
}
