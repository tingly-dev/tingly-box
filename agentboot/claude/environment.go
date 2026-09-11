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

		if isInheritedSessionIdentity(e) {
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

// inheritedSessionIdentityKeys are variables a running Claude Code session
// exports to describe ITSELF: that it is a remote / host-managed session and
// which identity it authenticates with. When the host running agentboot is
// itself inside such a session (a `tb start` from a Claude Code terminal, a
// Claude Code remote container), a child `claude` inheriting them adopts the
// parent's provider and credentials and silently ignores the routing it was
// launched with (ANTHROPIC_BASE_URL / ANTHROPIC_AUTH_TOKEN, --settings).
//
// Only identity is dropped. User configuration that legitimately lives in
// the shell (CLAUDE_CONFIG_DIR, CLAUDE_CODE_MAX_OUTPUT_TOKENS, ...) passes
// through unchanged.
var inheritedSessionIdentityKeys = map[string]bool{
	"CLAUDE_CODE_PROVIDER_MANAGED_BY_HOST": true,
	"CLAUDE_CODE_REMOTE":                   true,
	"CLAUDE_SESSION_INGRESS_TOKEN_FILE":    true,
	"SESSION_INGRESS_URL":                  true,
	// The parent's own session: a child that inherits CLAUDE_CODE_SESSION_ID
	// writes into the parent's transcript, and CLAUDECODE / the entrypoint
	// make it behave as a nested session of the parent's kind.
	"CLAUDE_CODE_SESSION_ID":                        true,
	"CLAUDE_CODE_CHILD_SESSION":                     true,
	"CLAUDE_CODE_ENTRYPOINT":                        true,
	"CLAUDECODE":                                    true,
	"CLAUDE_PID":                                    true,
	"CLAUDE_CODE_ACCOUNT_UUID":                      true,
	"CLAUDE_CODE_ORGANIZATION_UUID":                 true,
	"CLAUDE_CODE_USER_EMAIL":                        true,
	"CLAUDE_CODE_HOLD_UNANSWERED_PARKED_PERMISSION": true,
}

// Prefixes that only a running session exports about itself.
var inheritedSessionIdentityPrefixes = []string{
	"CLAUDE_CODE_REMOTE_",
	"CLAUDE_CODE_MESSAGING_",
	"CLAUDE_SESSION_INGRESS_",
}

// isInheritedSessionIdentity reports whether a KEY=VALUE entry names the
// parent session's identity rather than user configuration.
func isInheritedSessionIdentity(kv string) bool {
	key := kv
	if i := strings.IndexByte(kv, '='); i >= 0 {
		key = kv[:i]
	}
	if inheritedSessionIdentityKeys[key] {
		return true
	}
	for _, p := range inheritedSessionIdentityPrefixes {
		if strings.HasPrefix(key, p) {
			return true
		}
	}
	return false
}
