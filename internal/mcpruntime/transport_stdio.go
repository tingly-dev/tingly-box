package mcpruntime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// buildCommand builds an exec.Cmd for an MCP stdio source, reusing the cwd
// search and environment variable expansion logic from the original runtime.go.
// NOTE: The command must not be tied to the request context (ctx) because
// the MCP session persists across requests. Using CommandContext with the
// request context would kill the subprocess when the request completes.
// The ctx parameter is kept for API consistency but intentionally unused.
func buildCommand(_ context.Context, source typ.MCPSourceConfig) (*exec.Cmd, error) {
	if strings.TrimSpace(source.Command) == "" {
		return nil, &sourceError{sourceID: source.ID, msg: "empty command"}
	}

	// Use context.Background() for the subprocess lifecycle.
	// The session manages process lifetime explicitly via close(), not via context cancellation.
	cmd := exec.Command(source.Command, source.Args...)

	// buildDefaultSearchDirs returns directories to search when no explicit cwd is given
	// or when the configured cwd doesn't contain the script.
	buildDefaultSearchDirs := func() []string {
		dirs := []string{"~/.tingly-box/mcp"}
		if execPath, err := os.Executable(); err == nil {
			if !strings.Contains(execPath, "/go-build") {
				execDir := filepath.Dir(execPath)
				// Also check the "bundle" layout: binary is one level above scripts/.
				// e.g. /usr/local/bin/tingly-box + /usr/local/bin/../scripts/mcp_web_tools.py
				dirs = append(dirs, execDir, filepath.Dir(execDir))
			}
		}
		return dirs
	}

	// findScriptInDirs searches for the MCP script in a list of directories.
	// For each dir, checks if any relative arg resolves to an existing file.
	// Returns the directory containing the script, or "" if not found.
	findScriptInDirs := func(dirs []string) string {
		for _, d := range dirs {
			resolved := d
			if strings.HasPrefix(d, "~/") {
				if home, err := os.UserHomeDir(); err == nil {
					resolved = filepath.Join(home, d[2:])
				}
			}
			for _, arg := range source.Args {
				if filepath.IsAbs(arg) {
					continue
				}
				scriptPath := filepath.Join(resolved, arg)
				if _, err := os.Stat(scriptPath); err == nil {
					return resolved
				}
			}
		}
		return ""
	}

	cwd := strings.TrimSpace(source.Cwd)
	if cwd == "" {
		// No cwd configured: search for the script in likely locations.
		// os.Executable() returns the go-run temp binary when running via `go run`,
		// so we skip it when it contains "/go-build".
		if found := findScriptInDirs(buildDefaultSearchDirs()); found != "" {
			cwd = found
			logrus.Debugf("mcp: found script in cwd=%s", cwd)
		} else {
			logrus.Debugf("mcp: no cwd configured, script not found in search dirs: %v", buildDefaultSearchDirs())
		}
	} else {
		// User configured a cwd: expand ~ and validate script exists.
		// If not found, fall back to default search dirs.
		if strings.HasPrefix(cwd, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				cwd = filepath.Join(home, cwd[2:])
			}
		}
		if findScriptInDirs([]string{cwd}) == "" {
			logrus.Warnf("mcp: script not found in configured cwd %s, searching fallback dirs...", source.Cwd)
			if found := findScriptInDirs(buildDefaultSearchDirs()); found != "" {
				cwd = found
				logrus.Debugf("mcp: found script in fallback cwd=%s", cwd)
			} else {
				cwd = ""
			}
		}
	}
	if cwd != "" {
		cmd.Dir = cwd
	}

	// Also expand ~ in args (e.g. python3 ~/.tingly-box/mcp/scripts/...)
	for i, arg := range cmd.Args[1:] {
		if strings.HasPrefix(arg, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				cmd.Args[i+1] = filepath.Join(home, arg[2:])
			}
		}
	}

	env := os.Environ()
	envVarPattern := regexp.MustCompile(`\$\{([^}]+)\}`)
	for k, v := range source.Env {
		if strings.TrimSpace(k) != "" {
			// Expand ${VAR} syntax to actual environment variable value.
			expandedValue := envVarPattern.ReplaceAllStringFunc(v, func(match string) string {
				varName := match[2 : len(match)-1] // Extract VAR from ${VAR}
				return os.Getenv(varName)
			})
			env = append(env, k+"="+expandedValue)
		}
	}
	cmd.Env = env

	return cmd, nil
}

// sourceError wraps a source-specific error with its source ID.
type sourceError struct {
	sourceID string
	msg      string
}

func (e *sourceError) Error() string {
	return e.msg
}
