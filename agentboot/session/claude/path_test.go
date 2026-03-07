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
