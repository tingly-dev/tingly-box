package client

import (
	"context"
	"runtime"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/obs"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClientPool manages client instances for different providers.
// With SessionBoundTransport, connection pooling is handled at the Transport layer,
// so ClientPool simply creates new client instances as needed.
//
// Transports are automatically shared via TransportPool based on:
//
//	providerUUID + sessionID (for OAuth providers)
//
// ProxyURL is used to configure the transport but is NOT part of the key.
//
// Clients are automatically cleaned up via finalizers when garbage collected.
type ClientPool struct {
	recordSink *obs.Sink
}

// ClientPoolBuilder builds a ClientPool with specified configuration.
type ClientPoolBuilder struct {
	recordSink *obs.Sink
}

// NewClientPoolBuilder creates a new builder with default settings.
func NewClientPoolBuilder() *ClientPoolBuilder {
	return &ClientPoolBuilder{}
}

// WithRecordSink sets the record sink for all clients.
func (b *ClientPoolBuilder) WithRecordSink(sink *obs.Sink) *ClientPoolBuilder {
	b.recordSink = sink
	return b
}

// Build creates the ClientPool with configured settings.
func (b *ClientPoolBuilder) Build() *ClientPool {
	return &ClientPool{
		recordSink: b.recordSink,
	}
}

// NewClientPool creates a new ClientPool with default settings.
func NewClientPool() *ClientPool {
	return &ClientPool{}
}

// GetOpenAIClient returns an OpenAI client wrapper for the specified provider.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetOpenAIClient(ctx context.Context, provider *typ.Provider, model string) *OpenAIClient {
	sessionID := typ.GetSessionID(ctx)
	logrus.Debugf("Creating OpenAI client for provider: %s, session: %s", provider.Name, sessionID.Value)

	client, err := NewOpenAIClient(provider, model, sessionID)
	if err != nil {
		logrus.Errorf("Failed to create OpenAI client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	// This ensures idle connections are closed without requiring explicit Close() calls.
	runtime.SetFinalizer(client, func(c *OpenAIClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed OpenAI client for provider: %s via finalizer", provider.Name)
		}
	})

	return client
}

// GetAnthropicClient returns an Anthropic client wrapper for the specified provider.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetAnthropicClient(ctx context.Context, provider *typ.Provider, model string) *AnthropicClient {
	sessionID := typ.GetSessionID(ctx)
	logrus.Debugf("Creating Anthropic client for provider: %s, session: %s", provider.Name, sessionID.Value)

	client, err := NewAnthropicClient(provider, model, sessionID)
	if err != nil {
		logrus.Errorf("Failed to create Anthropic client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	runtime.SetFinalizer(client, func(c *AnthropicClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed Anthropic client for provider: %s via finalizer", provider.Name)
		}
	})

	return client
}

// GetGoogleClient returns a Google client wrapper for the specified provider.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetGoogleClient(ctx context.Context, provider *typ.Provider, model string) *GoogleClient {
	sessionID := typ.GetSessionID(ctx)
	logrus.Debugf("Creating Google client for provider: %s, session: %s", provider.Name, sessionID.Value)

	client, err := NewGoogleClient(provider, model, sessionID)
	if err != nil {
		logrus.Errorf("Failed to create Google client for provider %s: %v", provider.Name, err)
		return nil
	}

	if p.recordSink != nil && p.recordSink.IsEnabled() {
		client.SetRecordSink(p.recordSink)
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	runtime.SetFinalizer(client, func(c *GoogleClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed Google client for provider: %s via finalizer", provider.Name)
		}
	})

	return client
}

// SetRecordSink sets the record sink for the client pool.
// Note: This only affects newly created clients, not existing ones.
func (p *ClientPool) SetRecordSink(sink *obs.Sink) {
	if sink == nil {
		return
	}
	p.recordSink = sink
	if sink.IsEnabled() {
		logrus.Info("Record sink enabled for client pool")
	}
}

// GetRecordSink returns the record sink.
func (p *ClientPool) GetRecordSink() *obs.Sink {
	return p.recordSink
}

// InvalidateSession invalidates transports for a specific session.
// This is useful when a session ends or its OAuth token is revoked.
//
// Note: Since ClientPool no longer caches clients, this only invalidates
// the TransportPool entries. Client instances will be garbage collected naturally.
func (p *ClientPool) InvalidateSession(providerUUID, sessionID string) {
	if sessionID == "" {
		return
	}

	// Invalidate the corresponding transports
	GetGlobalTransportPool().InvalidateSession(providerUUID, sessionID)
	logrus.Infof("Invalidated transport pool entries for provider UUID: %s session: %s", providerUUID, sessionID)
}

// InvalidateProvider invalidates all transports for a specific provider UUID.
// This should be called when provider credentials are updated (e.g., OAuth token refresh).
//
// Note: Since ClientPool no longer caches clients, this only invalidates
// the TransportPool entries. Client instances will be garbage collected naturally.
func (p *ClientPool) InvalidateProvider(providerUUID string) {
	// Since we don't cache clients, we don't need to do anything at the client pool level.
	// The TransportPool manages its own lifecycle, and transports are created on-demand.
	//
	// If a provider's credentials change, existing client instances with stale credentials
	// will be naturally garbage collected. New requests will get fresh clients with updated credentials.
	logrus.Debugf("InvalidateProvider called for provider UUID: %s (no-op in once mode)", providerUUID)
}

// Stats provides statistics about the client pool and transport pool.
func (p *ClientPool) Stats() map[string]interface{} {
	// Return transport pool stats since that's where the real pooling happens
	transportStats := GetGlobalTransportPool().Stats()

	return map[string]interface{}{
		"mode":                "once",
		"transport_pool":      transportStats,
		"record_sink_enabled": p.recordSink != nil && p.recordSink.IsEnabled(),
	}
}
