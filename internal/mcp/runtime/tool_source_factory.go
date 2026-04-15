package runtime

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ToolSourceFactory creates tool sources based on configuration.
type ToolSourceFactory struct {
	sessionCache *sessionCache
}

// NewToolSourceFactory creates a new tool source factory.
func NewToolSourceFactory(sc *sessionCache) *ToolSourceFactory {
	return &ToolSourceFactory{
		sessionCache: sc,
	}
}

// CreateToolSource creates a tool source based on the transport type.
func (f *ToolSourceFactory) CreateToolSource(sourceConfig typ.MCPSourceConfig) (ToolSource, error) {
	transport := sourceConfig.Transport
	if transport == "" {
		transport = "stdio"
	}

	logrus.Debugf("mcp: creating tool source id=%s transport=%s", sourceConfig.ID, transport)

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
