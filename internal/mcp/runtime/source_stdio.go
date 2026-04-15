package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const (
	// maxStartupRetries is the maximum number of retries for tool listing during startup
	maxStartupRetries = 5
	// startupRetryDelay is the delay between retries during startup
	startupRetryDelay = 100 * time.Millisecond
	// startupReadyCheckTimeout is the maximum time to wait for server to be ready
	startupReadyCheckTimeout = 5 * time.Second
)

// StdioToolSource implements ToolSource for stdio-based MCP servers.
// This is an on-demand connection type - the subprocess is started when needed.
type StdioToolSource struct {
	*BaseToolSource
	sourceConfig   typ.MCPSourceConfig
	sessionCache   *sessionCache
	session        *sourceSession
	mu             sync.RWMutex
	ready          bool        // Track if server is ready
	readyMu        sync.Mutex  // Protect ready state
	startupRetries int         // Retry counter during startup
}

// NewStdioToolSource creates a new stdio tool source.
func NewStdioToolSource(sourceConfig typ.MCPSourceConfig, sc *sessionCache) (*StdioToolSource, error) {
	base := NewBaseToolSource(sourceConfig.ID, TransportStdio)

	return &StdioToolSource{
		BaseToolSource: base,
		sourceConfig:   sourceConfig,
		sessionCache:   sc,
		ready:          false,
		startupRetries: 0,
	}, nil
}

// Connect establishes a stdio connection to the MCP server.
func (s *StdioToolSource) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setState(StateConnecting, nil)

	logrus.Debugf("mcp: connecting stdio source=%s", s.GetSourceID())

	// Create session using existing session cache
	ss, _, err := s.sessionCache.getOrCreate(ctx, s.sourceConfig, 30*time.Second)
	if err != nil {
		s.setState(StateError, err)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "failed to create session", Err: err}
	}

	s.session = ss

	// Wait for server to be ready (especially important for builtin servers)
	if err := s.waitForServerReady(ctx); err != nil {
		s.setState(StateError, err)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "server not ready", Err: err}
	}

	s.setState(StateConnected, nil)

	logrus.Debugf("mcp: stdio source=%s connected and ready", s.GetSourceID())
	return nil
}

// waitForServerReady waits for the MCP server to be ready to handle requests.
// This is especially important for builtin servers that need time to initialize.
func (s *StdioToolSource) waitForServerReady(ctx context.Context) error {
	s.readyMu.Lock()
	defer s.readyMu.Unlock()

	// Check if already ready
	if s.ready {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, startupReadyCheckTimeout)
	defer cancel()

	// Try to list tools with retries
	for attempt := 0; attempt < maxStartupRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Try to list tools as a readiness check
		s.session.mu.RLock()
		tools, err := s.session.listTools(ctx)
		s.session.mu.RUnlock()

		if err == nil && len(tools) > 0 {
			s.ready = true
			s.startupRetries = attempt
			logrus.WithField("source", s.GetSourceID()).WithField("attempt", attempt+1).
				Debug("mcp: server is ready")
			return nil
		}

		// If not ready, wait before retry
		if attempt < maxStartupRetries-1 {
			logrus.WithField("source", s.GetSourceID()).WithField("attempt", attempt+1).
				Debug("mcp: server not ready, retrying...")

			select {
			case <-time.After(startupRetryDelay):
				// Continue retry
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	return &ConnectionError{
		Source: s.GetSourceID(),
		Reason: "server not ready after retries",
		Err:    nil,
	}
}

// Disconnect closes the stdio connection.
// For stdio, this terminates the subprocess.
func (s *StdioToolSource) Disconnect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.session != nil {
		s.session.mu.Lock()
		s.session.close()
		s.session.mu.Unlock()
		s.session = nil
	}

	s.setState(StateDisconnected, nil)
	logrus.Debugf("mcp: stdio source=%s disconnected", s.GetSourceID())
	return nil
}

// IsConnected returns whether the stdio connection is active.
func (s *StdioToolSource) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session != nil && s.session.session != nil
}

// ListTools returns all tools from the stdio MCP server.
func (s *StdioToolSource) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	if !s.IsConnected() {
		return nil, &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	s.mu.RLock()
	ss := s.session
	s.mu.RUnlock()

	ss.mu.RLock()
	defer ss.mu.RUnlock()

	tools, err := ss.listTools(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]ToolDefinition, len(tools))
	for i, tool := range tools {
		result[i] = ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			InputSchema: tool.schema(),
		}
	}

	logrus.Debugf("mcp: stdio source=%s listed %d tools", s.GetSourceID(), len(result))
	return result, nil
}

// CallTool executes a tool from the stdio MCP server.
func (s *StdioToolSource) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
	if !s.IsConnected() {
		return "", &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	s.mu.RLock()
	ss := s.session
	s.mu.RUnlock()

	ss.mu.RLock()
	defer ss.mu.RUnlock()

	var argsMap map[string]interface{}
	if strings.TrimSpace(arguments) != "" {
		if err := json.Unmarshal([]byte(arguments), &argsMap); err != nil {
			return "", &ToolExecutionError{ToolName: toolName, Message: "invalid arguments: " + err.Error()}
		}
	}

	if argsMap == nil {
		argsMap = map[string]interface{}{}
	}

	result, err := ss.callTool(ctx, toolName, argsMap)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"source": s.GetSourceID(),
			"tool":   toolName,
			"error":  err.Error(),
		}).Debug("mcp: stdio tool execution failed")
		return "", err
	}

	logrus.WithFields(logrus.Fields{
		"source": s.GetSourceID(),
		"tool":   toolName,
	}).Debug("mcp: stdio tool executed successfully")

	return result, nil
}

// GetSourceConfig returns the source configuration.
func (s *StdioToolSource) GetSourceConfig() interface{} {
	return s.sourceConfig
}

// HealthCheck verifies the stdio connection is still active.
func (s *StdioToolSource) HealthCheck(ctx context.Context) error {
	if !s.IsConnected() {
		return &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	// Try to list tools as a simple health check
	_, err := s.ListTools(ctx)
	if err != nil {
		return &ConnectionError{Source: s.GetSourceID(), Reason: "health check failed", Err: err}
	}

	return nil
}

// EnableHealthCheck is a no-op for on-demand connections.
func (s *StdioToolSource) EnableHealthCheck(ctx context.Context, interval time.Duration) {
	// No periodic health monitoring for on-demand connections
}

// DisableHealthCheck is a no-op for on-demand connections.
func (s *StdioToolSource) DisableHealthCheck(ctx context.Context) {
	// No periodic health monitoring for on-demand connections
}