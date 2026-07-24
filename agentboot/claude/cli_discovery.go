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
	"time"

	"github.com/sirupsen/logrus"
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

	claudeCLIProbeTimeout = 5 * time.Second
)

// Deprecated bundled fallback constants retained so existing agentboot
// consumers continue to compile. Packaged Claude Code paths are installation
// relative and are now discovered dynamically; no fixed path/version is safe.
const (
	DefaultBundledPathLinux   = ""
	DefaultBundledPathDarwin  = ""
	DefaultBundledPathWindows = ""
	DefaultBundledVersion     = ""
)

// CLIVariant represents a discovered Claude CLI installation
type CLIVariant struct {
	Path    string
	Version string
	Source  string // "global", "bundled", "native", "custom"
}

// CLIDiscovery handles Claude CLI path discovery and version checking
type CLIDiscovery struct {
	mu              sync.RWMutex
	cachedVariant   *CLIVariant
	cachedEnv       []string
	forceRediscover bool
	forceBundled    bool
	forceGlobal     bool
}

// NewCLIDiscovery creates a new CLI discovery instance
func NewCLIDiscovery() *CLIDiscovery {
	return &CLIDiscovery{}
}

// SetPreference configures a discovery source preference. A bundled CLI means
// a Claude Code executable packaged alongside Tingly-Box; it never means the
// Claude Desktop application.
func (d *CLIDiscovery) SetPreference(useBundled, useGlobal bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.forceBundled = useBundled
	d.forceGlobal = useGlobal
	d.cachedVariant = nil
	d.forceRediscover = true
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
		variant, err := d.validateVariant(ctx, path, "custom")
		if err != nil {
			return nil, fmt.Errorf("%s does not point to a usable Claude Code CLI: %w", EnvClaudePath, err)
		}
		return variant, nil
	}

	// 2. Check forced flags
	forceBundled := d.forceBundled || os.Getenv(EnvUseBundled) == "1"
	forceGlobal := d.forceGlobal || os.Getenv(EnvUseGlobal) == "1"
	if forceBundled && forceGlobal {
		return nil, fmt.Errorf("%s and %s cannot both be enabled", EnvUseBundled, EnvUseGlobal)
	}
	if forceBundled {
		bundled, err := d.findBundledVariant(ctx)
		if err == nil {
			return bundled, nil
		}
		return nil, fmt.Errorf("%s set but no packaged Claude Code CLI found: %w", EnvUseBundled, err)
	}

	if forceGlobal {
		global, err := d.findGlobalVariant(ctx)
		if err != nil {
			return nil, fmt.Errorf("%s set but Claude Code CLI was not found in PATH: %w", EnvUseGlobal, err)
		}
		return global, nil
	}

	// 3. Probe every supported Claude Code installation source and choose the
	// newest verified CLI. PATH wins ties because it reflects the user's
	// explicit shell selection.
	var variants []*CLIVariant
	if global, err := d.findGlobalVariant(ctx); err == nil {
		variants = append(variants, global)
	}
	if bundled, err := d.findBundledVariant(ctx); err == nil {
		variants = append(variants, bundled)
	}
	if native, err := d.findKnownVariant(ctx); err == nil {
		variants = append(variants, native)
	}

	if len(variants) == 0 {
		return nil, fmt.Errorf("no verified Claude Code CLI installation found")
	}
	best := variants[0]
	for _, candidate := range variants[1:] {
		if candidate.Path != best.Path && compareVersions(candidate.Version, best.Version) > 0 {
			best = candidate
		}
	}
	logrus.Debugf("Using Claude Code CLI %s from %s: %s", best.Version, best.Source, best.Path)
	return best, nil
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

		variant, err := d.validateVariant(ctx, path, "global")
		if err != nil {
			continue
		}
		return variant, nil
	}

	return nil, fmt.Errorf("verified Claude Code CLI not found in PATH")
}

// findBundledVariant returns a verified Claude Code CLI packaged alongside
// Tingly-Box. Claude Desktop is intentionally not a candidate.
func (d *CLIDiscovery) findBundledVariant(ctx context.Context) (*CLIVariant, error) {
	var variants []*CLIVariant
	for _, path := range d.bundledCandidatePaths() {
		variant, err := d.validateVariant(ctx, path, "bundled")
		if err == nil {
			variants = append(variants, variant)
		}
	}
	if len(variants) > 0 {
		return newestVariant(variants), nil
	}
	return nil, fmt.Errorf("no verified application-local Claude Code CLI")
}

func (d *CLIDiscovery) bundledCandidatePaths() []string {
	executable, err := os.Executable()
	if err != nil {
		return nil
	}
	dir := filepath.Dir(executable)
	name := claudeExecutableName()
	return uniquePaths([]string{
		filepath.Join(dir, name),
		filepath.Clean(filepath.Join(dir, "..", "Resources", name)),
		filepath.Join(dir, "claude-code", name),
		filepath.Clean(filepath.Join(dir, "..", "Resources", "claude-code", name)),
	})
}

// findKnownVariant searches the install locations used by Claude Code's
// native/npm installers when shell PATH setup is unavailable.
func (d *CLIDiscovery) findKnownVariant(ctx context.Context) (*CLIVariant, error) {
	var variants []*CLIVariant
	for _, path := range d.knownCandidatePaths() {
		variant, err := d.validateVariant(ctx, path, "native")
		if err == nil {
			variants = append(variants, variant)
		}
	}
	if len(variants) > 0 {
		return newestVariant(variants), nil
	}
	return nil, fmt.Errorf("Claude Code CLI not found in standard install locations")
}

func (d *CLIDiscovery) knownCandidatePaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	name := claudeExecutableName()
	paths := []string{
		filepath.Join(home, ".local", "bin", name),
		filepath.Join(home, ".npm-global", "bin", name),
		filepath.Join(home, ".claude", "local", name),
		filepath.Join(home, "node_modules", ".bin", name),
		filepath.Join(home, ".yarn", "bin", name),
	}
	if runtime.GOOS != "windows" {
		paths = append(paths,
			filepath.Join("/usr", "local", "bin", name),
			filepath.Join("/opt", "homebrew", "bin", name),
		)
	}
	return uniquePaths(paths)
}

func claudeExecutableName() string {
	if runtime.GOOS == "windows" {
		return "claude.exe"
	}
	return "claude"
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	unique := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		if _, ok := seen[path]; ok {
			continue
		}
		seen[path] = struct{}{}
		unique = append(unique, path)
	}
	return unique
}

func newestVariant(variants []*CLIVariant) *CLIVariant {
	best := variants[0]
	for _, candidate := range variants[1:] {
		if compareVersions(candidate.Version, best.Version) > 0 {
			best = candidate
		}
	}
	return best
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

	if runtime.GOOS == "windows" {
		return true
	}

	// Check executable bit on Unix-like platforms.
	return info.Mode().Perm()&0111 != 0
}

func (d *CLIDiscovery) validateVariant(ctx context.Context, path, source string) (*CLIVariant, error) {
	version, err := d.ValidateClaudeCodeCLI(ctx, path)
	if err != nil {
		return nil, err
	}
	return &CLIVariant{
		Path:    path,
		Version: version,
		Source:  source,
	}, nil
}

// ValidateClaudeCodeCLI verifies executable identity and returns its real
// version. Generic Claude/Anthropic desktop binaries are rejected: the version
// banner must identify Claude Code or the legacy Claude CLI name.
func (d *CLIDiscovery) ValidateClaudeCodeCLI(ctx context.Context, path string) (string, error) {
	if !d.isExecutable(path) {
		return "", fmt.Errorf("not an executable file: %s", path)
	}
	output, err := d.runVersionProbe(ctx, path)
	if err != nil {
		return "", err
	}
	banner := strings.TrimSpace(output)
	lower := strings.ToLower(banner)
	if !strings.Contains(lower, "claude code") && !strings.Contains(lower, "claude cli") {
		return "", fmt.Errorf("unexpected --version banner %q", banner)
	}
	version := parseVersion(banner)
	if version == "" || version == "unknown" || version[0] < '0' || version[0] > '9' {
		return "", fmt.Errorf("Claude Code version missing from banner %q", banner)
	}
	return version, nil
}

func (d *CLIDiscovery) runVersionProbe(ctx context.Context, path string) (string, error) {
	probeCtx, cancel := context.WithTimeout(ctx, claudeCLIProbeTimeout)
	defer cancel()

	cleanEnv, _ := d.buildCleanEnv(probeCtx)
	cmd := exec.CommandContext(probeCtx, path, "--version")
	cmd.Env = cleanEnv
	output, err := cmd.CombinedOutput()
	if err != nil {
		if probeCtx.Err() != nil {
			return "", fmt.Errorf("Claude Code version probe timed out: %w", probeCtx.Err())
		}
		return "", fmt.Errorf("Claude Code version probe failed: %w", err)
	}
	return string(output), nil
}

// buildCleanEnv creates a clean environment for running Claude CLI
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
		if strings.HasPrefix(part, "v") && len(part) > 1 && part[1] >= '0' && part[1] <= '9' {
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
func StreamToStdin(ctx context.Context, stdin io.WriteCloser, messages <-chan any) error {
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
	writer io.Writer
	closed bool
	mu     sync.Mutex
}

// NewStreamWriter creates a new stream writer
func NewStreamWriter(w io.Writer) *StreamWriter {
	return &StreamWriter{
		writer: w,
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
