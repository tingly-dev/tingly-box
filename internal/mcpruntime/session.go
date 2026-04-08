package mcpruntime

import (
	"context"
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
			Command:          cmd,
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
	default:
		return nil, nil, &sourceError{sourceID: source.ID, msg: "unsupported transport: " + transport}
	}

	client := mcp.NewClient(&mcp.Implementation{
		Name:    "tingly-box",
		Version: "dev",
	}, nil)

	// Connect with context timeout; the SDK's Connect spawns goroutines
	// that are scoped to the returned session, not the connect context.
	connectCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	session, connErr := client.Connect(connectCtx, t, nil)
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

// sessionError wraps a source-specific session error.
type sessionError struct {
	sourceID string
	msg      string
}

func (e *sessionError) Error() string {
	return e.msg
}
