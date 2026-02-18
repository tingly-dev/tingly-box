package server

import (
	"testing"
)

func TestParseWorkingDir(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		expected    string
		expectError bool
	}{
		{
			name:        "basic working directory",
			input:       "working directory: /Users/yz/project",
			expected:    "/Users/yz/project",
			expectError: false,
		},
		{
			name:        "working directory in system prompt",
			input:       "You are a helpful assistant.\nworking directory: /home/user/code\nPlease help the user.",
			expected:    "/home/user/code",
			expectError: false,
		},
		{
			name:        "case insensitive - Working Directory",
			input:       "Working Directory: /path/to/dir",
			expected:    "/path/to/dir",
			expectError: false,
		},
		{
			name:        "case insensitive - WORKING DIRECTORY",
			input:       "WORKING DIRECTORY: /path/to/dir",
			expected:    "/path/to/dir",
			expectError: false,
		},
		{
			name:        "with extra spaces",
			input:       "working  directory  :  /path/with/spaces  ",
			expected:    "/path/with/spaces",
			expectError: false,
		},
		{
			name:        "Windows path",
			input:       "working directory: C:\\Users\\test\\project",
			expected:    "C:\\Users\\test\\project",
			expectError: false,
		},
		{
			name:        "relative path",
			input:       "working directory: ./src/components",
			expected:    "./src/components",
			expectError: false,
		},
		{
			name:        "path with spaces",
			input:       "working directory: /Users/name/My Project/src",
			expected:    "/Users/name/My Project/src",
			expectError: false,
		},
		{
			name:        "no working directory",
			input:       "You are a helpful assistant.",
			expected:    "",
			expectError: true,
		},
		{
			name:        "empty string",
			input:       "",
			expected:    "",
			expectError: true,
		},
		{
			name:        "working directory without value",
			input:       "working directory:   \n",
			expected:    "",
			expectError: true,
		},
		{
			name:        "working directory at end of string",
			input:       "Some text\nworking directory: /end/path",
			expected:    "/end/path",
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseWorkingDir(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if result != tt.expected {
					t.Errorf("expected %q, got %q", tt.expected, result)
				}
			}
		})
	}
}

func TestTryParseWorkingDir(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "valid working directory",
			input:    "working directory: /Users/yz/project",
			expected: "/Users/yz/project",
		},
		{
			name:     "no working directory",
			input:    "You are a helpful assistant.",
			expected: "",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "case insensitive",
			input:    "Working Directory: /path",
			expected: "/path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := TryParseWorkingDir(tt.input)
			if result != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, result)
			}
		})
	}
}
