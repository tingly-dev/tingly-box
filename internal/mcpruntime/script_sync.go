package mcpruntime

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var builtinMCPToolScripts = []string{
	"mcp_web_tools.py",
	"mcp_weather_tools.py",
}

// EnsureBuiltinScripts copies bundled MCP scripts to <configDir>/mcp.
// This guarantees stdio MCP scripts are available at a stable path for runtime config.
func EnsureBuiltinScripts(configDir string) error {
	if configDir == "" {
		return fmt.Errorf("configDir is empty")
	}
	targetDir := filepath.Join(configDir, "mcp")
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return fmt.Errorf("create mcp dir: %w", err)
	}

	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve executable path: %w", err)
	}
	execDir := filepath.Dir(execPath)

	sourceDirs := []string{
		filepath.Join(execDir, "scripts"),
		filepath.Join(filepath.Dir(execDir), "scripts"),
		filepath.Join(execDir, "..", "scripts"),
		"scripts",
	}

	for _, scriptName := range builtinMCPToolScripts {
		src, findErr := findScriptPath(sourceDirs, scriptName)
		if findErr != nil {
			return findErr
		}
		dst := filepath.Join(targetDir, scriptName)
		if err := copyFile(src, dst, 0o755); err != nil {
			return fmt.Errorf("copy %s: %w", scriptName, err)
		}
	}

	return nil
}

func findScriptPath(dirs []string, scriptName string) (string, error) {
	for _, dir := range dirs {
		path := filepath.Join(dir, scriptName)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path, nil
		}
	}
	return "", fmt.Errorf("builtin mcp script not found: %s", scriptName)
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}
