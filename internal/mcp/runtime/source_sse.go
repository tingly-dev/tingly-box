package runtime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SSEToolSource implements ToolSource for SSE-based MCP servers.
// This is a persistent connection type with health monitoring.
type SSEToolSource struct {
	*BaseToolSource
	sourceConfig      typ.MCPSourceConfig
	sessionCache      *sessionCache
	session           *sourceSession
	healthMonitor     *HealthMonitor
	reconnectStrategy *ExponentialBackoffStrategy
	lastHealthCheck   time.Time
	mu                sync.RWMutex
}

// NewSSEToolSource creates a new SSE tool source.
func NewSSEToolSource(sourceConfig typ.MCPSourceConfig, sc *sessionCache) (*SSEToolSource, error) {
	base := NewBaseToolSource(sourceConfig.ID, TransportSSE)

	return &SSEToolSource{
		BaseToolSource:    base,
		sourceConfig:      sourceConfig,
		sessionCache:      sc,
		reconnectStrategy: NewExponentialBackoffStrategy(),
	}, nil
}

// Connect establishes an SSE connection to the MCP server.
func (s *SSEToolSource) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setState(StateConnecting, nil)

	logrus.Debugf("mcp: connecting sse source=%s endpoint=%s", s.GetSourceID(), s.sourceConfig.Endpoint)

	// Create session using existing session cache
	ss, _, err := s.sessionCache.getOrCreate(ctx, s.sourceConfig, 30*time.Second)
	if err != nil {
		s.setState(StateError, err)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "failed to create session", Err: err}
	}

	s.session = ss
	s.setState(StateConnected, nil)

	logrus.Debugf("mcp: sse source=%s connected", s.GetSourceID())
	return nil
}

// Disconnect closes the SSE connection.
func (s *SSEToolSource) Disconnect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop health monitoring if running
	if s.healthMonitor != nil {
		s.healthMonitor.Stop(ctx)
		s.healthMonitor = nil
	}

	if s.session != nil {
		s.session.mu.Lock()
		s.session.close()
		s.session.mu.Unlock()
		s.session = nil
	}

	s.setState(StateDisconnected, nil)
	logrus.Debugf("mcp: sse source=%s disconnected", s.GetSourceID())
	return nil
}

// IsConnected returns whether the SSE connection is active.
func (s *SSEToolSource) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session != nil && s.session.session != nil
}

// ListTools returns all tools from the SSE MCP server.
func (s *SSEToolSource) ListTools(ctx context.Context) ([]ToolDefinition, error) {
	if !s.IsConnected() {
		return nil, &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	s.mu.RLock()
	ss := s.session
	s.mu.RUnlock()

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

	logrus.Debugf("mcp: sse source=%s listed %d tools", s.GetSourceID(), len(result))
	return result, nil
}

// CallTool executes a tool from the SSE MCP server.
func (s *SSEToolSource) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
	if !s.IsConnected() {
		return "", &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	s.mu.RLock()
	ss := s.session
	s.mu.RUnlock()

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
		}).Debug("mcp: sse tool execution failed")
		return "", err
	}

	logrus.WithFields(logrus.Fields{
		"source": s.GetSourceID(),
		"tool":   toolName,
	}).Debug("mcp: sse tool executed successfully")

	return result, nil
}

// GetSourceConfig returns the source configuration.
func (s *SSEToolSource) GetSourceConfig() interface{} {
	return s.sourceConfig
}

// HealthCheck verifies the SSE connection is still active.
func (s *SSEToolSource) HealthCheck(ctx context.Context) error {
	if !s.IsConnected() {
		return &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	// Avoid hitting ListTools on every monitor tick; reconnect path still handles errors.
	const minListToolsProbeInterval = 2 * time.Minute

	s.mu.Lock()
	if !s.lastHealthCheck.IsZero() && time.Since(s.lastHealthCheck) < minListToolsProbeInterval {
		s.mu.Unlock()
		return nil
	}
	s.lastHealthCheck = time.Now()
	s.mu.Unlock()

	_, err := s.ListTools(ctx)
	if err != nil {
		return &ConnectionError{Source: s.GetSourceID(), Reason: "health check failed", Err: err}
	}

	return nil
}

// EnableHealthCheck starts periodic health monitoring.
func (s *SSEToolSource) EnableHealthCheck(ctx context.Context, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.healthMonitor != nil {
		return // Already monitoring
	}

	errorClassifier := &DefaultErrorClassifier{}
	s.healthMonitor = NewHealthMonitor(s, errorClassifier)
	s.healthMonitor.Start(ctx, interval)

	logrus.Debugf("mcp: sse source=%s health monitoring enabled interval=%v", s.GetSourceID(), interval)
}

// DisableHealthCheck stops health monitoring.
func (s *SSEToolSource) DisableHealthCheck(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.healthMonitor != nil {
		s.healthMonitor.Stop(ctx)
		s.healthMonitor = nil
	}

	logrus.Debugf("mcp: sse source=%s health monitoring disabled", s.GetSourceID())
}

// Reconnect implements ReconnectableSource interface.
func (s *SSEToolSource) Reconnect(ctx context.Context) error {
	s.mu.Lock()
	status := s.GetConnectionStatus()
	s.mu.Unlock()

	// Check if we should retry
	if !s.reconnectStrategy.ShouldRetry(status.RetryCount) {
		logrus.WithFields(logrus.Fields{
			"source":     s.GetSourceID(),
			"retryCount": status.RetryCount,
		}).Error("mcp: sse max retries reached")

		s.Disconnect(ctx)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "max retries reached"}
	}

	// Calculate backoff delay
	delay := s.reconnectStrategy.NextRetry(status.RetryCount)
	logrus.WithFields(logrus.Fields{
		"source":     s.GetSourceID(),
		"retryCount": status.RetryCount,
		"delay":      delay,
	}).Debug("mcp: sse scheduling reconnection")

	// Wait for backoff delay
	select {
	case <-time.After(delay):
		// Continue with reconnection
	case <-ctx.Done():
		return ctx.Err()
	}

	// Disconnect first
	s.Disconnect(ctx)

	// Increment retry count
	s.mu.Lock()
	s.status.RetryCount++
	s.mu.Unlock()

	// Reconnect
	if err := s.Connect(ctx); err != nil {
		return err
	}

	logrus.WithFields(logrus.Fields{
		"source":     s.GetSourceID(),
		"retryCount": status.RetryCount,
	}).Info("mcp: sse reconnected successfully")

	return nil
}

// buildSSEHTTPClient creates an HTTP client for the SSE transport.
func buildSSEHTTPClient(source typ.MCPSourceConfig) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // SSE connections should be secure by default
		},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		// Configure dialer for better connection handling
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		// Force HTTP/2
		ForceAttemptHTTP2: true,
	}

	// Apply proxy configuration
	transport.Proxy = http.ProxyFromEnvironment
	if strings.TrimSpace(source.ProxyURL) != "" {
		if proxyURL, err := url.Parse(source.ProxyURL); err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	return &http.Client{
		Transport: &headerInjectRoundTripper{Transport: transport, Headers: source.Headers},
		Timeout:   120 * time.Second,
	}
}
