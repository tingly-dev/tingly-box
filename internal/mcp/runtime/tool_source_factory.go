package runtime

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ToolSourceFactory creates tool sources based on configuration.
type ToolSourceFactory struct {
	sessionCache *sessionCache
	clientPool   *client.ClientPool
}

// NewToolSourceFactory creates a new tool source factory.
// The cp parameter is kept for API compatibility but is no longer used.
func NewToolSourceFactory(sc *sessionCache, cp interface{}) *ToolSourceFactory {
	_ = cp // cp is no longer used but kept for API compatibility
	return &ToolSourceFactory{
		sessionCache: sc,
		clientPool:   nil, // No longer used
	}
}

// SetClientPool sets the client pool on the factory.
func (f *ToolSourceFactory) SetClientPool(cp *client.ClientPool) {
	f.clientPool = cp
}

// CreateToolSource creates a tool source based on the transport type.
func (f *ToolSourceFactory) CreateToolSource(sourceConfig typ.MCPSourceConfig) (ToolSource, error) {
	transport := sourceConfig.Transport
	if transport == "" {
		transport = "stdio"
	}

	logrus.Debugf("mcp: creating tool source id=%s transport=%s", sourceConfig.ID, transport)

	// In-process advisor tool source takes precedence over transport matching.
	if sourceConfig.Advisor != nil || transport == "advisor" {
		return NewAdvisorToolSource(sourceConfig, f.clientPool)
	}

	switch transport {
	case "stdio":
		return NewStdioToolSource(sourceConfig, f.sessionCache)
	case "http":
		return NewHTTPToolSource(sourceConfig, f.sessionCache)
	case "sse":
		return NewSSEToolSource(sourceConfig, f.sessionCache)
	default:
		return nil, &UnsupportedTransportError{Transport: transport}
	}
}
