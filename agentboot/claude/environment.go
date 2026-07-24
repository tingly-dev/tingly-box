package claude

import (
	"context"
	"fmt"
	"os"
	"strings"
)

const (
	// Environment variable names
	EnvClaudePath  = "CLAUDE_CLI_PATH"
	EnvUseBundled  = "CLAUDE_USE_BUNDLED"
	EnvUseGlobal   = "CLAUDE_USE_GLOBAL"
	EnvClaudeHome  = "CLAUDE_HOME"
	EnvNodePath    = "NODE_PATH"
	EnvBunVersions = "BUN_VERSIONS"
	EnvBunInstall  = "BUN_INSTALL"

	// EnvBunEnv is retained for source compatibility.
	// Deprecated: use EnvBunInstall.
	EnvBunEnv = EnvBunInstall
)

// GetCleanEnv returns a clean environment for running Claude CLI.
func (d *CLIDiscovery) GetCleanEnv(ctx context.Context) ([]string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.cachedEnv != nil && !d.forceRediscover {
		return d.cachedEnv, nil
	}

	d.forceRediscover = false
	env, err := d.buildCleanEnv(ctx)
	if err != nil {
		return nil, err
	}

	d.cachedEnv = env
	return env, nil
}

// buildCleanEnv creates a clean environment for running Claude CLI.
func (d *CLIDiscovery) buildCleanEnv(ctx context.Context) ([]string, error) {
	env := os.Environ()

	// Remove problematic environment variables
	cleanEnv := make([]string, 0, len(env))

	for _, e := range env {
		// Skip local node_modules paths
		if strings.HasPrefix(e, EnvNodePath+"=") {
			continue
		}

		// Skip Bun-specific paths that might interfere
		if strings.HasPrefix(e, EnvBunVersions+"=") ||
			strings.HasPrefix(e, EnvBunInstall+"=") {
			continue
		}

		cleanEnv = append(cleanEnv, e)
	}

	// Ensure PATH doesn't contain local node_modules
	cleanEnv = d.cleanPATH(cleanEnv)

	return cleanEnv, nil
}

// cleanPATH removes local node_modules directories from PATH.
func (d *CLIDiscovery) cleanPATH(env []string) []string {
	for i, e := range env {
		if strings.HasPrefix(e, "PATH=") {
			pathValue := e[5:]
			paths := strings.Split(pathValue, string(os.PathListSeparator))

			cleanPaths := make([]string, 0, len(paths))
			for _, p := range paths {
				// Skip node_modules paths
				if strings.Contains(p, "node_modules") ||
					strings.Contains(p, ".bun") {
					continue
				}
				cleanPaths = append(cleanPaths, p)
			}

			env[i] = "PATH=" + strings.Join(cleanPaths, string(os.PathListSeparator))
			break
		}
	}

	return env
}

// GetCleanEnv is a convenience function for getting clean environment.
func GetCleanEnv(ctx context.Context) ([]string, error) {
	return defaultDiscovery.GetCleanEnv(ctx)
}

// FormatEnv formats an environment variable as KEY=VALUE.
func FormatEnv(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// MergeEnv merges custom environment variables with base environment.
// Custom variables override base ones with the same key.
func MergeEnv(base []string, custom []string) []string {
	result := make([]string, 0, len(base)+len(custom))
	baseMap := make(map[string]string)

	// Build map from base
	for _, e := range base {
		if idx := strings.IndexByte(e, '='); idx > 0 {
			baseMap[e[:idx]] = e[idx+1:]
		} else {
			result = append(result, e)
		}
	}

	// Add base env (will be overridden by custom if key matches)
	for k, v := range baseMap {
		result = append(result, FormatEnv(k, v))
	}

	// Add custom env (overrides)
	for _, e := range custom {
		result = append(result, e)
	}

	return result
}
