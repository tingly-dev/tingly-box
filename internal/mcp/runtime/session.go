package runtime

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// sourceSession holds a persistent SDK client + session for a single MCP source.
type sourceSession struct {
	sourceID string
	client   *mcp.Client
	session  *mcp.ClientSession
	mu       sync.RWMutex
}

// listTools returns the list of tools from the SDK session.
// The caller holds ss.mu.
func (ss *sourceSession) listTools(ctx context.Context) ([]mcpTool, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.session == nil {
		return nil, &sessionError{sourceID: ss.sourceID, msg: "session not connected"}
	}
	result, err := ss.session.ListTools(ctx, nil)
	if err != nil {
		return nil, &sessionError{sourceID: ss.sourceID, msg: "list tools: " + err.Error()}
	}
	return contentBlocksToTools(result.Tools), nil
}

// callTool executes a tool call via the SDK session.
// The caller holds ss.mu.
func (ss *sourceSession) callTool(ctx context.Context, name string, arguments map[string]any) (string, error) {
	ss.mu.RLock()
	defer ss.mu.RUnlock()
	if ss.session == nil {
		return "", &sessionError{sourceID: ss.sourceID, msg: "session not connected"}
	}
	result, err := ss.session.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return "", &sessionError{sourceID: ss.sourceID, msg: "call tool " + name + ": " + err.Error()}
	}
	raw, err := contentBlocksToResult(result.Content)
	if err != nil {
		return "", &sessionError{sourceID: ss.sourceID, msg: "marshal tool result: " + err.Error()}
	}
	return string(raw), nil
}

// close closes the SDK session and client.
// The caller holds ss.mu.
func (ss *sourceSession) close() {
	if ss.session != nil {
		_ = ss.session.Close()
		ss.session = nil
	}
	ss.client = nil
}

// sessionCache maps source ID → sourceSession.
type sessionCache struct {
	mu       sync.RWMutex
	sessions map[string]*sourceSession
}

func newSessionCache() *sessionCache {
	return &sessionCache{sessions: make(map[string]*sourceSession)}
}

// getOrCreate returns the session for the given source, creating one if needed.
// It returns (session, cleanupFunc, error).
func (sc *sessionCache) getOrCreate(ctx context.Context, source typ.MCPSourceConfig, timeout time.Duration) (*sourceSession, func(), error) {
	// Fast path: read-locked lookup.
	sc.mu.RLock()
	ss := sc.sessions[source.ID]
	sc.mu.RUnlock()
	if ss != nil {
		ss.mu.RLock()
		connected := ss.session != nil
		ss.mu.RUnlock()
		if connected {
			return ss, func() {}, nil
		}
	}

	// Slow path: write-locked creation with double-check.
	sc.mu.Lock()
	defer sc.mu.Unlock()
	ss = sc.sessions[source.ID]
	if ss == nil {
		ss = &sourceSession{sourceID: source.ID}
		sc.sessions[source.ID] = ss
	} else {
		// Someone else created it between our fast path and write lock.
		ss.mu.RLock()
		connected := ss.session != nil
		ss.mu.RUnlock()
		if connected {
			return ss, func() {}, nil
		}
	}

	// Build transport and connect.
	transport := strings.TrimSpace(source.Transport)
	if transport == "" {
		transport = "stdio"
	}
	logrus.Debugf("mcp: creating transport for source=%s transport=%s", source.ID, transport)

	var t mcp.Transport
	switch transport {
	case "stdio":
		cmd, cmdErr := buildCommand(ctx, source)
		if cmdErr != nil {
			return nil, nil, cmdErr
		}
		t = &mcp.CommandTransport{
			Command:           cmd,
			TerminateDuration: 5 * time.Second,
		}
	case "http":
		if strings.TrimSpace(source.Endpoint) == "" {
			return nil, nil, &sourceError{sourceID: source.ID, msg: "empty endpoint"}
		}
		httpClient := buildHTTPClient(source)
		t = &mcp.StreamableClientTransport{
			Endpoint:             source.Endpoint,
			HTTPClient:           httpClient,
			MaxRetries:           3,
			DisableStandaloneSSE: true, // Some MCP servers don't support standalone SSE GET endpoint
		}
	case "sse":
		if strings.TrimSpace(source.Endpoint) == "" {
			return nil, nil, &sourceError{sourceID: source.ID, msg: "empty endpoint"}
		}
		httpClient := buildSSEHTTPClient(source)
		t = &mcp.SSEClientTransport{
			Endpoint:   source.Endpoint,
			HTTPClient: httpClient,
		}
	default:
		return nil, nil, &sourceError{sourceID: source.ID, msg: "unsupported transport: " + transport}
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "tingly-box",
		Version: "dev",
	}, nil)

	// The SDK binds the JSON-RPC connection lifetime to the context passed to
	// Connect. Do not pass a request-scoped or timeout context here, or long-lived
	// transports such as SSE will be closed as soon as Connect returns.
	sessionCtx := context.WithoutCancel(ctx)
	session, connErr := client.Connect(sessionCtx, t, nil)
	if connErr != nil {
		// Transport was started but session failed. The SDK's session.Close handles cleanup.
		return nil, nil, &sessionError{sourceID: source.ID, msg: "connect: " + connErr.Error()}
	}

	ss.mu.Lock()
	ss.client = client
	ss.session = session
	ss.mu.Unlock()

	logrus.Debugf("mcp: session created for source=%s transport=%s", source.ID, transport)
	return ss, func() {}, nil
}

// closeAll closes all sessions in the cache.
func (sc *sessionCache) closeAll() {
	sc.mu.Lock()
	defer sc.mu.Unlock()
	for _, ss := range sc.sessions {
		ss.mu.Lock()
		ss.close()
		ss.mu.Unlock()
	}
	sc.sessions = make(map[string]*sourceSession)
}

// buildCommand builds an exec.Cmd for an MCP stdio source.
// It handles working directory resolution, script searching, and environment variable expansion.
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
				// e.g. /usr/local/bin/tingly-box + /usr/local/bin/../scripts/custom_tool.py
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

// sessionError wraps a source-specific session error.
type sessionError struct {
	sourceID string
	msg      string
}

func (e *sessionError) Error() string {
	return e.msg
}
