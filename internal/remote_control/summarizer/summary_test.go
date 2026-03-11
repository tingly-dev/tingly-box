package summarizer

import (
	"testing"
)

func TestEngine_Summarize(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty string",
			input:    "",
			expected: "No output generated.",
		},
		{
			name:     "simple output",
			input:    "Hello, world!",
			expected: "Hello, world!",
		},
		{
			name:     "multi-line output",
			input:    "Line 1\nLine 2\nLine 3",
			expected: "Line 1\nLine 2\nLine 3",
		},
		{
			name:     "long output truncated",
			input:    "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10\nLine 11",
			expected: "Line 1\nLine 2\nLine 3\nLine 4\nLine 5\nLine 6\nLine 7\nLine 8\nLine 9\nLine 10",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.Summarize(tt.input)
			if result != tt.expected {
				t.Errorf("Expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestEngine_ExtractActionItems(t *testing.T) {
	engine := NewEngine()

	input := `
TODO: Fix this bug
Another line
- [ ] Task 1
More lines
[ ] Task 2 without TODO
TODO: Another task
`

	items := engine.ExtractActionItems(input)

	if len(items) != 4 {
		t.Errorf("Expected 4 items, got %d", len(items))
	}

	expectedItems := []string{
		"TODO: Fix this bug",
		"- [ ] Task 1",
		"[ ] Task 2 without TODO",
		"TODO: Another task",
	}

	for i, item := range items {
		if item != expectedItems[i] {
			t.Errorf("Expected item '%s', got '%s'", expectedItems[i], item)
		}
	}
}

func TestEngine_CountTokens(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{
			name:     "empty",
			input:    "",
			expected: 0,
		},
		{
			name:     "short text",
			input:    "Hello world",
			expected: 2, // "Hello world" is 11 chars / 4 = 2.75 -> 2
		},
		{
			name:     "longer text",
			input:    "This is a test sentence",
			expected: 5, // "This is a test sentence" is 23 chars / 4 = 5 (integer division)
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.CountTokens(tt.input)
			if result != tt.expected {
				t.Errorf("Expected %d tokens, got %d", tt.expected, result)
			}
		})
	}
}

func TestEngine_isSummaryLine(t *testing.T) {
	engine := NewEngine()

	tests := []struct {
		name     string
		line     string
		expected bool
	}{
		{
			name:     "regular line",
			line:     "Hello world",
			expected: true,
		},
		{
			name:     "file path",
			line:     "/path/to/file.go",
			expected: false,
		},
		{
			name:     "command line",
			line:     "$ ls -la",
			expected: false,
		},
		{
			name:     "timestamp with brackets",
			line:     "[2024-01-01 12:00:00] Message",
			expected: false,
		},
		{
			name:     "ANSI color codes",
			line:     "\x1b[31mRed text\x1b[0m",
			expected: false,
		},
		{
			name:     "URL",
			line:     "https://example.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.isSummaryLine(tt.line)
			if result != tt.expected {
				t.Errorf("For '%s': expected %v, got %v", tt.line, tt.expected, result)
			}
		})
	}
}

func TestEngine_ExtractKeyInformation(t *testing.T) {
	engine := NewEngine()

	input := []string{
		"Regular line 1",
		"/path/to/file.go:10: error",
		"Regular line 2",
		"[2024-01-01] Log entry",
		"Regular line 3",
	}

	result := engine.extractKeyInformation(input)

	// Should only contain regular lines
	expected := "Regular line 1\nRegular line 2\nRegular line 3"

	if result != expected {
		t.Errorf("Expected '%s', got '%s'", expected, result)
	}
}
