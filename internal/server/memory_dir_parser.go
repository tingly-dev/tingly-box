package server

import (
	"fmt"
	"regexp"
	"strings"
)

// workingDirPattern matches "working directory: {path}" in system prompts
// Case-insensitive, captures the path after the colon
var workingDirPattern = regexp.MustCompile(`(?i)working\s+directory\s*:\s*(.+?)(?:\n|$)`)

// ParseWorkingDir extracts the working directory path from a system prompt
// Returns the path or error if not found
func ParseWorkingDir(systemPrompt string) (string, error) {
	matches := workingDirPattern.FindStringSubmatch(systemPrompt)
	if matches == nil {
		return "", fmt.Errorf("working directory not found in system prompt")
	}

	path := strings.TrimSpace(matches[1])
	if path == "" {
		return "", fmt.Errorf("working directory path is empty")
	}

	return path, nil
}

// TryParseWorkingDir attempts to extract working directory, returns empty string if not found
func TryParseWorkingDir(systemPrompt string) string {
	path, err := ParseWorkingDir(systemPrompt)
	if err != nil {
		return ""
	}
	return path
}
