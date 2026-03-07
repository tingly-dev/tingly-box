package claude

import (
	"path/filepath"
	"strings"
)

// resolveProjectPath converts project path to Claude's encoded format
// /root/tingly-polish -> -root-tingly-polish
func (s *Store) resolveProjectPath(projectPath string) string {
	if projectPath == "" {
		return ""
	}

	encoded := encodeProjectPath(projectPath)
	return filepath.Join(s.projectsDir, encoded)
}

// encodeProjectPath encodes a project path for Claude's format
// This is a separate function for easier testing
// /root/tingly-polish -> -root-tingly-polish
func encodeProjectPath(projectPath string) string {
	absPath, err := filepath.Abs(projectPath)
	if err != nil {
		return projectPath
	}

	encoded := absPath
	if strings.HasPrefix(encoded, "/") {
		// Remove leading / and replace all remaining / with -
		encoded = "-" + strings.ReplaceAll(strings.TrimPrefix(encoded, "/"), "/", "-")
	} else if len(encoded) >= 2 && encoded[1] == ':' {
		// Windows path: C:\path -> -C-path
		drive := string(encoded[0])
		rest := strings.ReplaceAll(encoded[2:], "\\", "/")
		rest = strings.ReplaceAll(rest, "/", "-")
		encoded = "-" + drive + "-" + rest
	}

	return encoded
}
