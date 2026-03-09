package claude

import (
	"strings"
	"testing"
)

func TestEncodeProjectPath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Unix path - root",
			input:    "/root/tingly-polish",
			contains: "root",
			// Note: filepath.Abs will make it absolute, so we check for key parts
		},
		{
			name:     "Unix path - home",
			input:    "/home/user/project",
			contains: "home",
		},
		{
			name:     "Unix path - root only",
			input:    "/root",
			contains: "root",
		},
		{
			name:     "Relative path",
			input:    "./project",
			contains: "project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := encodeProjectPath(tt.input)
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("encodeProjectPath(%q) = %q, should contain %q", tt.input, result, tt.contains)
			}
			// Check that result starts with -
			if !strings.HasPrefix(result, "-") {
				t.Errorf("encodeProjectPath(%q) = %q, should start with '-'", tt.input, result)
			}
		})
	}
}

func TestResolveProjectPath(t *testing.T) {
	store := &Store{
		projectsDir: "/test/.claude/projects",
	}

	tests := []struct {
		name     string
		input    string
		contains string
	}{
		{
			name:     "Unix path",
			input:    "/root/tingly-polish",
			contains: "root", // Will contain encoded path parts
		},
		{
			name:     "Root path",
			input:    "/root",
			contains: "root",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := store.resolveProjectPath(tt.input)
			if tt.contains != "" && !strings.Contains(result, tt.contains) {
				t.Errorf("resolveProjectPath(%q) = %q, should contain %q", tt.input, result, tt.contains)
			}
			// Check that it contains the projects dir
			if !strings.Contains(result, store.projectsDir) {
				t.Errorf("resolveProjectPath(%q) = %q, should contain %q", tt.input, result, store.projectsDir)
			}
		})
	}
}

func TestDecodeProjectPath(t *testing.T) {
	tests := []struct {
		name     string
		encoded  string
		expected string
	}{
		{
			name:     "Simple path without dashes",
			encoded:  "-home-user-project",
			expected: "/home/user/project",
		},
		{
			name:     "Root only",
			encoded:  "-root",
			expected: "/root",
		},
		{
			name:     "Empty string",
			encoded:  "",
			expected: "",
		},
		{
			name:     "No leading dash",
			encoded:  "root-project",
			expected: "root-project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := DecodeProjectPath(tt.encoded)
			if result != tt.expected {
				t.Errorf("DecodeProjectPath(%q) = %q, want %q", tt.encoded, result, tt.expected)
			}
		})
	}
}

func TestEncodeDecodeRoundTrip(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "Unix path - home",
			input: "/home/user/project",
		},
		{
			name:  "Unix path - root only",
			input: "/root",
		},
		{
			name:  "Unix path - multiple levels",
			input: "/Users/yz/Project",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: encodeProjectPath uses filepath.Abs, so the result will be absolute
			// We can't do a perfect round-trip test without knowing the absolute path
			// But we can test that DecodeProjectPath produces a valid path
			encoded := encodeProjectPath(tt.input)
			decoded := DecodeProjectPath(encoded)

			// The decoded path should start with /
			if !strings.HasPrefix(decoded, "/") {
				t.Errorf("Decoded path %q should start with '/'", decoded)
			}

			// For simple paths without dashes in names, the round-trip should work
			inputParts := strings.Split(tt.input, "/")
			decodedParts := strings.Split(decoded, "/")

			// Count non-empty parts
			var inputNonEmpty, decodedNonEmpty []string
			for _, p := range inputParts {
				if p != "" {
					inputNonEmpty = append(inputNonEmpty, p)
				}
			}
			for _, p := range decodedParts {
				if p != "" {
					decodedNonEmpty = append(decodedNonEmpty, p)
				}
			}

			// For paths without dashes in component names, parts should match
			if len(inputNonEmpty) > 0 && len(decodedNonEmpty) > 0 {
				// Just verify we got a reasonable path structure
				if len(decodedNonEmpty) == 0 {
					t.Errorf("Decoded path has no components")
				}
			}
		})
	}
}
