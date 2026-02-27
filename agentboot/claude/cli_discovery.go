package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
)

const (
	// Environment variable names
	EnvClaudePath  = "CLAUDE_CLI_PATH"
	EnvUseBundled  = "CLAUDE_USE_BUNDLED"
	EnvUseGlobal   = "CLAUDE_USE_GLOBAL"
	EnvClaudeHome  = "CLAUDE_HOME"
	EnvBunEnv      = "BUN_INSTALL"
	EnvNodePath    = "NODE_PATH"
	EnvBunVersions = "BUN_VERSIONS"
	EnvBunInstall  = "BUN_INSTALL"

	// Default paths
	DefaultBundledPathLinux   = "/opt/claude-code/dist/claude"
	DefaultBundledPathDarwin  = "/Applications/Claude.app/Contents/MacOS/claude"
	DefaultBundledPathWindows = `C:\Program Files\Claude\claude.exe`

	// Claude version for bundled fallback
	DefaultBundledVersion = "1.0.0"
)

// CLIVariant represents a discovered Claude CLI installation
type CLIVariant struct {
	Path    string
	Version string
	Source  string // "global", "bundled", "custom", "env"
}

// CLIDiscovery handles Claude CLI path discovery and version checking
type CLIDiscovery struct {
	mu              sync.RWMutex
	cachedVariant   *CLIVariant
	cachedEnv       []string
	forceRediscover bool
}

// NewCLIDiscovery creates a new CLI discovery instance
func NewCLIDiscovery() *CLIDiscovery {
	return &CLIDiscovery{}
}

// FindClaudeCLI finds the best available Claude CLI installation
func (d *CLIDiscovery) FindClaudeCLI(ctx context.Context) (*CLIVariant, error) {
	d.mu.Lock()
	defer d.mu.Unlock()

	// Return cached variant if available and not forced to rediscover
	if d.cachedVariant != nil && !d.forceRediscover {
		return d.cachedVariant, nil
	}

	d.forceRediscover = false
	variant, err := d.discoverCLI(ctx)
	if err != nil {
		return nil, err
	}

	d.cachedVariant = variant
	return variant, nil
}

// InvalidateCache clears the cached CLI variant
func (d *CLIDiscovery) InvalidateCache() {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.cachedVariant = nil
	d.cachedEnv = nil
	d.forceRediscover = true
}

// GetCleanEnv returns a clean environment for running Claude CLI
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

// discoverCLI implements the CLI discovery logic
func (d *CLIDiscovery) discoverCLI(ctx context.Context) (*CLIVariant, error) {
	// 1. Check explicit override via environment variable
	if path := os.Getenv(EnvClaudePath); path != "" {
		if d.isExecutable(path) {
			variant := &CLIVariant{
				Path:   path,
				Source: "custom",
			}
			// Try to get version
			if version, err := d.getClaudeVersion(ctx, path); err == nil {
				variant.Version = version
			}
			return variant, nil
		}
		return nil, fmt.Errorf("CLAUDE_CLI_PATH set but not executable: %s", path)
	}

	// 2. Check forced flags
	if os.Getenv(EnvUseBundled) == "1" {
		bundled := d.getBundledVariant()
		if bundled != nil {
			return bundled, nil
		}
		return nil, fmt.Errorf("CLAUDE_USE_BUNDLED set but no bundled CLI found")
	}

	if os.Getenv(EnvUseGlobal) == "1" {
		global, err := d.findGlobalVariant(ctx)
		if err != nil {
			return nil, fmt.Errorf("CLAUDE_USE_GLOBAL set but global CLI not found: %w", err)
		}
		return global, nil
	}

	// 3. Find global variant
	global, err := d.findGlobalVariant(ctx)
	if err == nil && global != nil {
		// Compare with bundled if available
		bundled := d.getBundledVariant()
		if bundled != nil && global.Version != "" && bundled.Version != "" {
			// Use newer version
			if compareVersions(global.Version, bundled.Version) < 0 {
				logrus.Debugf("Using bundled CLI (newer): %s vs %s", bundled.Version, global.Version)
				return bundled, nil
			}
			logrus.Debugf("Using global CLI (newer): %s vs %s", global.Version, bundled.Version)
		}
		return global, nil
	}

	// 4. Fallback to bundled
	bundled := d.getBundledVariant()
	if bundled != nil {
		return bundled, nil
	}

	return nil, fmt.Errorf("no Claude CLI installation found")
}

// findGlobalVariant searches for globally installed Claude CLI
func (d *CLIDiscovery) findGlobalVariant(ctx context.Context) (*CLIVariant, error) {
	// Try standard command names
	names := []string{"claude", "anthropic"}

	for _, name := range names {
		path, err := d.whichCommand(name)
		if err != nil {
			continue
		}

		// Verify it's actually Claude CLI
		if !d.verifyClaudeCLI(ctx, path) {
			continue
		}

		variant := &CLIVariant{
			Path:   path,
			Source: "global",
		}

		// Get version
		if version, err := d.getClaudeVersion(ctx, path); err == nil {
			variant.Version = version
		}

		return variant, nil
	}

	return nil, fmt.Errorf("global Claude CLI not found in PATH")
}

// getBundledVariant returns the bundled Claude CLI if available
func (d *CLIDiscovery) getBundledVariant() *CLIVariant {
	path := d.getBundledPath()
	if path == "" || !d.isExecutable(path) {
		return nil
	}

	return &CLIVariant{
		Path:    path,
		Version: DefaultBundledVersion,
		Source:  "bundled",
	}
}

// getBundledPath returns the bundled CLI path for the current platform
func (d *CLIDiscovery) getBundledPath() string {
	switch runtime.GOOS {
	case "darwin":
		return DefaultBundledPathDarwin
	case "linux":
		return DefaultBundledPathLinux
	case "windows":
		return DefaultBundledPathWindows
	default:
		return ""
	}
}

// whichCommand finds a command in PATH
func (d *CLIDiscovery) whichCommand(name string) (string, error) {
	path, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}

	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return path, nil // Return original if resolution fails
	}

	return resolved, nil
}

// isExecutable checks if a path is executable
func (d *CLIDiscovery) isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}

	// Check if it's a regular file
	if !info.Mode().IsRegular() {
		return false
	}

	// Check executable bit
	return info.Mode().Perm()&0111 != 0
}

// verifyClaudeCLI verifies that a binary is actually Claude CLI
func (d *CLIDiscovery) verifyClaudeCLI(ctx context.Context, path string) bool {
	// Run with --version flag to verify
	cleanEnv, _ := d.buildCleanEnv(ctx)
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = cleanEnv

	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}

	// Check if output looks like Claude version
	outputStr := string(output)
	return strings.Contains(outputStr, "Claude") ||
		strings.Contains(outputStr, "Anthropic")
}

// getClaudeVersion gets the version string from Claude CLI
func (d *CLIDiscovery) getClaudeVersion(ctx context.Context, path string) (string, error) {
	cleanEnv, _ := d.buildCleanEnv(ctx)
	cmd := exec.CommandContext(ctx, path, "--version")
	cmd.Env = cleanEnv

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return parseVersion(string(output)), nil
}

// buildCleanEnv creates a clean environment for running Claude CLI
func (d *CLIDiscovery) buildCleanEnv(ctx context.Context) ([]string, error) {
	env := os.Environ()

	// Remove problematic environment variables
	cleanEnv := make([]string, 0, len(env))

	for _, e := range env {
		// Skip local node_modules paths
		if strings.HasPrefix(e, "NODE_PATH=") {
			continue
		}

		// Skip Bun-specific paths that might interfere
		if strings.HasPrefix(e, "BUN_VERSIONS=") ||
			strings.HasPrefix(e, "BUN_INSTALL=") {
			continue
		}

		cleanEnv = append(cleanEnv, e)
	}

	// Ensure PATH doesn't contain local node_modules
	cleanEnv = d.cleanPATH(cleanEnv)

	return cleanEnv, nil
}

// cleanPATH removes local node_modules directories from PATH
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

// parseVersion extracts version from CLI output
func parseVersion(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	if len(lines) == 0 {
		return "unknown"
	}

	// First line usually contains version
	firstLine := strings.TrimSpace(lines[0])

	// Try to extract version number (e.g., "1.0.0" from "Claude CLI v1.0.0")
	parts := strings.Fields(firstLine)
	for _, part := range parts {
		if strings.HasPrefix(part, "v") {
			return strings.TrimPrefix(part, "v")
		}
		// Check if it looks like a version (starts with digit)
		if len(part) > 0 && part[0] >= '0' && part[0] <= '9' {
			return part
		}
	}

	return firstLine
}

// compareVersions compares two version strings
// Returns: -1 if v1 < v2, 0 if v1 == v2, 1 if v1 > v2
func compareVersions(v1, v2 string) int {
	// Handle "unknown" versions
	if v1 == "unknown" && v2 == "unknown" {
		return 0
	}
	if v1 == "unknown" {
		return -1
	}
	if v2 == "unknown" {
		return 1
	}

	// Split by dots
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var n1, n2 int

		if i < len(parts1) {
			n1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			n2, _ = strconv.Atoi(parts2[i])
		}

		if n1 < n2 {
			return -1
		}
		if n1 > n2 {
			return 1
		}
	}

	return 0
}

// FindClaudeCLI is a convenience function that uses a default discovery instance
var defaultDiscovery = NewCLIDiscovery()

func FindClaudeCLI(ctx context.Context) (*CLIVariant, error) {
	return defaultDiscovery.FindClaudeCLI(ctx)
}

// GetCleanEnv is a convenience function for getting clean environment
func GetCleanEnv(ctx context.Context) ([]string, error) {
	return defaultDiscovery.GetCleanEnv(ctx)
}

// InvalidateDiscoveryCache invalidates the global discovery cache
func InvalidateDiscoveryCache() {
	defaultDiscovery.InvalidateCache()
}

// StreamToStdin streams messages to the stdin of a running process
func StreamToStdin(ctx context.Context, stdin io.WriteCloser, messages <-chan map[string]interface{}) error {
	logrus.Debugln("[StreamToStdin] Starting to stream messages to stdin")

	// Use buffered writer for efficient I/O and ensure data is flushed
	writer := bufio.NewWriter(stdin)

	encoder := json.NewEncoder(writer)

	messageCount := 0
	for {
		select {
		case <-ctx.Done():
			logrus.Debugln("[StreamToStdin] Context cancelled, stopping")
			return ctx.Err()
		case msg, ok := <-messages:
			if !ok {
				// Channel closed, flush any remaining data and return
				// IMPORTANT: Do NOT close stdin here - it will be managed by the Query
				writer.Flush()
				logrus.Debugf("[StreamToStdin] Message channel closed after sending %d messages", messageCount)
				return nil
			}

			messageCount++
			logrus.Debugf("[StreamToStdin] Sending message #%d", messageCount)

			if err := encoder.Encode(msg); err != nil {
				return fmt.Errorf("encode message: %w", err)
			}
			// Flush immediately after each message to ensure prompt delivery
			if err := writer.Flush(); err != nil {
				return fmt.Errorf("flush message: %w", err)
			}
		}
	}
}

// StreamReader reads line-delimited JSON from a reader
type StreamReader struct {
	scanner *bufio.Scanner
}

// NewStreamReader creates a new stream reader
func NewStreamReader(r io.Reader) *StreamReader {
	return &StreamReader{
		scanner: bufio.NewScanner(r),
	}
}

// Next reads the next JSON object from the stream
func (r *StreamReader) Next() (map[string]interface{}, error) {
	if !r.scanner.Scan() {
		if err := r.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	line := r.scanner.Bytes()
	var data map[string]interface{}
	if err := json.Unmarshal(line, &data); err != nil {
		return nil, fmt.Errorf("decode json: %w", err)
	}

	return data, nil
}

// ReadAll reads all remaining objects from the stream
func (r *StreamReader) ReadAll() ([]map[string]interface{}, error) {
	var results []map[string]interface{}

	for {
		data, err := r.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		results = append(results, data)
	}

	return results, nil
}

// StreamWriter writes line-delimited JSON to a writer
type StreamWriter struct {
	writer  io.Writer
	encoder *json.Encoder
	closed  bool
	mu      sync.Mutex
}

// NewStreamWriter creates a new stream writer
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{
		writer:  w,
		encoder: json.NewEncoder(w),
	}
}

// Write writes a JSON object to the stream
func (w *StreamWriter) Write(data map[string]interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.closed {
		return fmt.Errorf("stream writer is closed")
	}

	// Encode with newline
	buf, err := json.Marshal(data)
	if err != nil {
		return err
	}

	buf = append(buf, '\n')
	_, err = w.writer.Write(buf)
	return err
}

// Close closes the stream writer
func (w *StreamWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.closed = true
	if closer, ok := w.writer.(io.Closer); ok {
		return closer.Close()
	}
	return nil
}

// FormatEnv formats an environment variable as KEY=VALUE
func FormatEnv(key, value string) string {
	return fmt.Sprintf("%s=%s", key, value)
}

// MergeEnv merges custom environment variables with base environment
// Custom variables override base ones with the same key
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
