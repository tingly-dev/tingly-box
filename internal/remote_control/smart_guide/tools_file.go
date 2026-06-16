package smart_guide

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/afk"
)

// maxFileOpSize is the maximum file/content size (10MB) these tools will handle,
// matching the behavior of the previously-used agentscope file tools.
const maxFileOpSize = 10 * 1024 * 1024

// ============================================================================
// read
// ============================================================================

// ReadFileTool reads the contents of a file, optionally limited to a line range.
type ReadFileTool struct {
	executor *ToolExecutor
}

// NewReadFileTool constructs a ReadFileTool bound to the given executor.
func NewReadFileTool(executor *ToolExecutor) *ReadFileTool {
	return &ReadFileTool{executor: executor}
}

func (t *ReadFileTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        "read",
		Description: anthropic.String("Read the contents of a file. Optionally provide a 1-based offset and a line limit to read only a slice of the file. Files larger than 10MB are rejected."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the file to read (absolute, or relative to the current working directory).",
				},
				"offset": map[string]any{
					"type":        "integer",
					"description": "Optional 1-based line number to start reading from.",
				},
				"limit": map[string]any{
					"type":        "integer",
					"description": "Optional maximum number of lines to read starting at offset.",
				},
			},
			Required: []string{"path"},
		},
	}
}

type readFileParams struct {
	Path   string `json:"path"`
	Offset int    `json:"offset"`
	Limit  int    `json:"limit"`
}

func (t *ReadFileTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var p readFileParams
	if err := json.Unmarshal(rawInput, &p); err != nil {
		return "", fmt.Errorf("read: invalid arguments: %w", err)
	}
	if p.Path == "" {
		return "Error: 'path' is required.", nil
	}

	resolved := t.executor.ResolvePath(p.Path)

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", resolved), nil
		}
		return fmt.Sprintf("Error: cannot stat %s: %v", resolved, err), nil
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: %s is a directory, not a file.", resolved), nil
	}
	if info.Size() > maxFileOpSize {
		return fmt.Sprintf("Error: file %s is %d bytes, which exceeds the 10MB limit.", resolved, info.Size()), nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", resolved, err), nil
	}

	content := string(data)
	if p.Offset > 0 || p.Limit > 0 {
		content = sliceLines(content, p.Offset, p.Limit)
	}
	return content, nil
}

// sliceLines returns the lines of content selected by a 1-based offset and a max
// line limit. An offset <= 0 is treated as 1; a limit <= 0 means "to the end".
func sliceLines(content string, offset, limit int) string {
	lines := strings.Split(content, "\n")
	start := offset - 1
	if start < 0 {
		start = 0
	}
	if start >= len(lines) {
		return ""
	}
	end := len(lines)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return strings.Join(lines[start:end], "\n")
}

// ============================================================================
// write
// ============================================================================

// WriteFileTool writes content to a file, creating parent directories as needed.
type WriteFileTool struct {
	executor *ToolExecutor
}

// NewWriteFileTool constructs a WriteFileTool bound to the given executor.
func NewWriteFileTool(executor *ToolExecutor) *WriteFileTool {
	return &WriteFileTool{executor: executor}
}

func (t *WriteFileTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        "write",
		Description: anthropic.String("Write content to a file, overwriting it if it exists and creating parent directories as needed. Content larger than 10MB is rejected."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the file to write (absolute, or relative to the current working directory).",
				},
				"content": map[string]any{
					"type":        "string",
					"description": "The full content to write to the file.",
				},
			},
			Required: []string{"path", "content"},
		},
	}
}

type writeFileParams struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (t *WriteFileTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var p writeFileParams
	if err := json.Unmarshal(rawInput, &p); err != nil {
		return "", fmt.Errorf("write: invalid arguments: %w", err)
	}
	if p.Path == "" {
		return "Error: 'path' is required.", nil
	}
	if len(p.Content) > maxFileOpSize {
		return fmt.Sprintf("Error: content is %d bytes, which exceeds the 10MB limit.", len(p.Content)), nil
	}

	resolved := t.executor.ResolvePath(p.Path)

	if dir := filepath.Dir(resolved); dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Sprintf("Error: cannot create directory %s: %v", dir, err), nil
		}
	}

	if err := os.WriteFile(resolved, []byte(p.Content), 0644); err != nil {
		return fmt.Sprintf("Error: cannot write %s: %v", resolved, err), nil
	}

	return fmt.Sprintf("Wrote %d bytes to %s", len(p.Content), resolved), nil
}

// ============================================================================
// edit
// ============================================================================

// EditFileTool replaces an exact, unique occurrence of old_text in a file.
type EditFileTool struct {
	executor *ToolExecutor
}

// NewEditFileTool constructs an EditFileTool bound to the given executor.
func NewEditFileTool(executor *ToolExecutor) *EditFileTool {
	return &EditFileTool{executor: executor}
}

func (t *EditFileTool) Param() anthropic.BetaToolParam {
	return anthropic.BetaToolParam{
		Name:        "edit",
		Description: anthropic.String("Edit a file by replacing an exact occurrence of old_text with new_text. old_text must appear exactly once in the file, otherwise the edit is rejected."),
		InputSchema: anthropic.BetaToolInputSchemaParam{
			Properties: map[string]any{
				"path": map[string]any{
					"type":        "string",
					"description": "Path to the file to edit (absolute, or relative to the current working directory).",
				},
				"old_text": map[string]any{
					"type":        "string",
					"description": "The exact text to find. It must appear exactly once in the file.",
				},
				"new_text": map[string]any{
					"type":        "string",
					"description": "The replacement text.",
				},
			},
			Required: []string{"path", "old_text", "new_text"},
		},
	}
}

type editFileParams struct {
	Path    string `json:"path"`
	OldText string `json:"old_text"`
	NewText string `json:"new_text"`
}

func (t *EditFileTool) Call(ctx context.Context, rawInput json.RawMessage) (string, error) {
	var p editFileParams
	if err := json.Unmarshal(rawInput, &p); err != nil {
		return "", fmt.Errorf("edit: invalid arguments: %w", err)
	}
	if p.Path == "" {
		return "Error: 'path' is required.", nil
	}
	if p.OldText == "" {
		return "Error: 'old_text' is required.", nil
	}

	resolved := t.executor.ResolvePath(p.Path)

	info, err := os.Stat(resolved)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Sprintf("Error: file not found: %s", resolved), nil
		}
		return fmt.Sprintf("Error: cannot stat %s: %v", resolved, err), nil
	}
	if info.IsDir() {
		return fmt.Sprintf("Error: %s is a directory, not a file.", resolved), nil
	}
	if info.Size() > maxFileOpSize {
		return fmt.Sprintf("Error: file %s is %d bytes, which exceeds the 10MB limit.", resolved, info.Size()), nil
	}

	data, err := os.ReadFile(resolved)
	if err != nil {
		return fmt.Sprintf("Error: cannot read %s: %v", resolved, err), nil
	}

	content := string(data)
	count := strings.Count(content, p.OldText)
	switch {
	case count == 0:
		return fmt.Sprintf("Error: old_text was not found in %s.", resolved), nil
	case count > 1:
		return fmt.Sprintf("Error: old_text appears %d times in %s; it must be unique.", count, resolved), nil
	}

	updated := strings.Replace(content, p.OldText, p.NewText, 1)
	if err := os.WriteFile(resolved, []byte(updated), info.Mode().Perm()); err != nil {
		return fmt.Sprintf("Error: cannot write %s: %v", resolved, err), nil
	}

	return fmt.Sprintf("Edited %s (1 replacement)", resolved), nil
}

// Compile-time assertions that each tool implements the engine Tool interface.
var (
	_ afk.Tool = (*ReadFileTool)(nil)
	_ afk.Tool = (*WriteFileTool)(nil)
	_ afk.Tool = (*EditFileTool)(nil)
)
