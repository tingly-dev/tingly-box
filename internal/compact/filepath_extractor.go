package compact

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
)

// FilePathExtractor extracts file paths from tool parameters.
type FilePathExtractor struct {
	// Pattern for file paths (extension-agnostic)
	pattern *regexp.Regexp
}

// NewFilePathExtractor creates a new extractor.
func NewFilePathExtractor() *FilePathExtractor {
	return &FilePathExtractor{
		// Match any path-like string (preserves original format)
		// Matches: /path, ./path, ../path, C:\path, relative/path, etc.
		pattern: regexp.MustCompile(`[a-zA-Z0-9_\-./\\]+[a-zA-Z0-9_\-./\\]`),
	}
}

// Extract extracts all file paths from input string.
func (e *FilePathExtractor) Extract(input string) []string {
	seen := make(map[string]bool)
	var results []string

	// Split by common delimiters and check each part
	parts := regexp.MustCompile(`[,\s\n"'\(\)\[\]{}]+`).Split(input, -1)

	for _, part := range parts {
		if part == "" {
			continue
		}
		if e.looksLikePath(part) && !seen[part] {
			seen[part] = true
			results = append(results, part)
		}
	}

	return results
}

// looksLikePath checks if string looks like a file path.
func (e *FilePathExtractor) looksLikePath(s string) bool {
	if len(s) < 2 {
		return false
	}

	// Check for path indicators
	hasPathSeparator := strings.Contains(s, "/") || strings.Contains(s, "\\")
	hasDot := strings.Contains(s, ".")

	// Must have separator, OR have dot and reasonable length
	// Short filenames like "a.go" should be accepted
	if hasPathSeparator {
		return true
	}
	if hasDot && len(s) >= 4 {
		return true
	}
	// Very short filenames with extensions (like "go")
	if hasDot && len(s) >= 3 {
		return true
	}

	return false
}

// ExtractFromMap extracts file paths from map (tool input parameters).
func (e *FilePathExtractor) ExtractFromMap(m map[string]any) []string {
	var results []string

	// Common parameter names that contain file paths
	pathKeys := []string{"path", "file", "filename", "filepath", "file_path", "uri", "url"}

	// Check path keys first
	for _, key := range pathKeys {
		if val, ok := m[key]; ok {
			switch v := val.(type) {
			case string:
				results = append(results, e.Extract(v)...)
			}
		}
	}

	// Also check all string values
	for _, v := range m {
		switch val := v.(type) {
		case string:
			results = append(results, e.Extract(val)...)
		case []string:
			for _, s := range val {
				results = append(results, e.Extract(s)...)
			}
		case map[string]any:
			results = append(results, e.ExtractFromMap(val)...)
		}
	}

	return deduplicate(results)
}

// deduplicate removes duplicate strings from slice.
func deduplicate(slice []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, s := range slice {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// Virtual tool constants
const (
	VirtualReadTool   = "read_file"
	ExpiredContentMsg = "内容已过期,请重新查看"
)

// CreateVirtualToolCalls creates virtual tool use + result messages.
func CreateVirtualToolCalls(files []string) []any {
	if len(files) == 0 {
		return nil
	}

	var result []any

	// Create virtual assistant message with tool_use blocks
	var toolBlocks []anthropic.ContentBlockParamUnion
	for _, file := range files {
		toolBlocks = append(toolBlocks, anthropic.NewToolUseBlock(
			VirtualReadTool,
			map[string]any{"path": file},
			VirtualReadTool,
		))
	}

	result = append(result, anthropic.NewAssistantMessage(toolBlocks...))

	// Create virtual user message with tool_result blocks
	var resultBlocks []anthropic.ContentBlockParamUnion
	for range files {
		resultBlocks = append(resultBlocks, anthropic.NewToolResultBlock(
			VirtualReadTool,
			ExpiredContentMsg,
			false,
		))
	}

	result = append(result, anthropic.NewUserMessage(resultBlocks...))

	return result
}

// CreateBetaVirtualToolCalls creates virtual tool calls for beta API.
func CreateBetaVirtualToolCalls(files []string) []any {
	if len(files) == 0 {
		return nil
	}

	var result []any

	// Create virtual assistant message with tool_use blocks
	var toolBlocks []anthropic.BetaContentBlockParamUnion
	for _, file := range files {
		toolBlocks = append(toolBlocks, anthropic.NewBetaToolUseBlock(
			VirtualReadTool,
			map[string]any{"path": file},
			VirtualReadTool,
		))
	}

	result = append(result, anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleAssistant,
		Content: toolBlocks,
	})

	// Create virtual user message with tool_result blocks
	var resultBlocks []anthropic.BetaContentBlockParamUnion
	for range files {
		var toolResult anthropic.BetaToolResultBlockParam
		toolResult.ToolUseID = VirtualReadTool
		// Create text block for content
		textBlock := anthropic.BetaTextBlockParam{
			Text: ExpiredContentMsg,
			Type: "text",
		}
		toolResult.Content = []anthropic.BetaToolResultBlockParamContentUnion{
			{OfText: &textBlock},
		}
		resultBlocks = append(resultBlocks, anthropic.BetaContentBlockParamUnion{OfToolResult: &toolResult})
	}

	result = append(result, anthropic.BetaMessageParam{
		Role:    anthropic.BetaMessageParamRoleUser,
		Content: resultBlocks,
	})

	return result
}

// FormatFileNote formats collected file paths into a note string.
func FormatFileNote(files []string) string {
	if len(files) == 0 {
		return ""
	}
	return fmt.Sprintf("\n\n📎 Files: %s", strings.Join(files, ", "))
}
