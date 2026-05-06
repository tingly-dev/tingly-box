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

// HTTPToolSource implements ToolSource for HTTP-based MCP servers.
// This is a persistent connection type with health monitoring.
type HTTPToolSource struct {
	*BaseToolSource
	sourceConfig      typ.MCPSourceConfig
	sessionCache      *sessionCache
	session           *sourceSession
	healthMonitor     *HealthMonitor
	reconnectStrategy *ExponentialBackoffStrategy
	lastHealthCheck   time.Time
	mu                sync.RWMutex
}

// NewHTTPToolSource creates a new HTTP tool source.
func NewHTTPToolSource(sourceConfig typ.MCPSourceConfig, sc *sessionCache) (*HTTPToolSource, error) {
	base := NewBaseToolSource(sourceConfig.ID, TransportHTTP)

	return &HTTPToolSource{
		BaseToolSource:    base,
		sourceConfig:      sourceConfig,
		sessionCache:      sc,
		reconnectStrategy: NewExponentialBackoffStrategy(),
	}, nil
}

// Connect establishes an HTTP connection to the MCP server.
func (s *HTTPToolSource) Connect(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.setState(StateConnecting, nil)

	logrus.Debugf("mcp: connecting http source=%s endpoint=%s", s.GetSourceID(), s.sourceConfig.Endpoint)

	// Create session using existing session cache
	ss, _, err := s.sessionCache.getOrCreate(ctx, s.sourceConfig, 30*time.Second)
	if err != nil {
		s.setState(StateError, err)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "failed to create session", Err: err}
	}

	s.session = ss
	s.setState(StateConnected, nil)

	logrus.Debugf("mcp: http source=%s connected", s.GetSourceID())
	return nil
}

// Disconnect closes the HTTP connection.
func (s *HTTPToolSource) Disconnect(ctx context.Context) error {
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
	logrus.Debugf("mcp: http source=%s disconnected", s.GetSourceID())
	return nil
}

// IsConnected returns whether the HTTP connection is active.
func (s *HTTPToolSource) IsConnected() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.session != nil && s.session.session != nil
}

// ListTools returns all tools from the HTTP MCP server.
func (s *HTTPToolSource) ListTools(ctx context.Context) ([]ToolDefinition, error) {
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

	logrus.Debugf("mcp: http source=%s listed %d tools", s.GetSourceID(), len(result))
	return result, nil
}

// CallTool executes a tool from the HTTP MCP server.
func (s *HTTPToolSource) CallTool(ctx context.Context, toolName string, arguments string) (string, error) {
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
		}).Debug("mcp: http tool execution failed")
		return "", err
	}

	logrus.WithFields(logrus.Fields{
		"source": s.GetSourceID(),
		"tool":   toolName,
	}).Debug("mcp: http tool executed successfully")

	return result, nil
}

// GetSourceConfig returns the source configuration.
func (s *HTTPToolSource) GetSourceConfig() interface{} {
	return s.sourceConfig
}

// HealthCheck verifies the HTTP connection is still active.
func (s *HTTPToolSource) HealthCheck(ctx context.Context) error {
	if !s.IsConnected() {
		return &ConnectionError{Source: s.GetSourceID(), Reason: "not connected"}
	}

	// Avoid hitting ListTools on every monitor tick; reconnect path still handles errors.
	const minListToolsProbeInterval = 1 * time.Minute

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
func (s *HTTPToolSource) EnableHealthCheck(ctx context.Context, interval time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.healthMonitor != nil {
		return // Already monitoring
	}

	errorClassifier := &DefaultErrorClassifier{}
	s.healthMonitor = NewHealthMonitor(s, errorClassifier)
	s.healthMonitor.Start(ctx, interval)

	logrus.Debugf("mcp: http source=%s health monitoring enabled interval=%v", s.GetSourceID(), interval)
}

// DisableHealthCheck stops health monitoring.
func (s *HTTPToolSource) DisableHealthCheck(ctx context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.healthMonitor != nil {
		s.healthMonitor.Stop(ctx)
		s.healthMonitor = nil
	}

	logrus.Debugf("mcp: http source=%s health monitoring disabled", s.GetSourceID())
}

// Reconnect implements ReconnectableSource interface.
func (s *HTTPToolSource) Reconnect(ctx context.Context) error {
	s.mu.Lock()
	status := s.GetConnectionStatus()
	s.mu.Unlock()

	// Check if we should retry
	if !s.reconnectStrategy.ShouldRetry(status.RetryCount) {
		logrus.WithFields(logrus.Fields{
			"source":     s.GetSourceID(),
			"retryCount": status.RetryCount,
		}).Error("mcp: http max retries reached")

		s.Disconnect(ctx)
		return &ConnectionError{Source: s.GetSourceID(), Reason: "max retries reached"}
	}

	// Calculate backoff delay
	delay := s.reconnectStrategy.NextRetry(status.RetryCount)
	logrus.WithFields(logrus.Fields{
		"source":     s.GetSourceID(),
		"retryCount": status.RetryCount,
		"delay":      delay,
	}).Debug("mcp: http scheduling reconnection")

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
	}).Info("mcp: http reconnected successfully")

	return nil
}

// buildHTTPClient creates an HTTP client for the HTTP transport.
func buildHTTPClient(source typ.MCPSourceConfig) *http.Client {
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: false, // Default to secure connections
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

// headerInjectRoundTripper injects custom headers into outgoing requests.
type headerInjectRoundTripper struct {
	Transport *http.Transport
	Headers   map[string]string
}

func (rt *headerInjectRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	for k, v := range rt.Headers {
		if strings.TrimSpace(k) != "" {
			req.Header.Set(k, v)
		}
	}
	return rt.Transport.RoundTrip(req)
}
