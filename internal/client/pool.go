package client

import (
	"context"
	"runtime"

	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/ai"
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
//
// Virtual-model providers are not special-cased here: they are constructed by
// the same NewOpenAIClient / NewAnthropicClient and differ only in the dialer
// their transport uses (see vmodel_transport.go).
type ClientPool struct{}

// NewClientPool creates a new ClientPool with default settings.
func NewClientPool() *ClientPool {
	return &ClientPool{}
}

// GetOpenAIClient returns an OpenAI client wrapper for the specified provider.
// For Codex OAuth providers, returns a CodexClient with special handling.
// For Kimi Code OAuth providers, returns a KimiClient with special handling.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetOpenAIClient(ctx context.Context, provider *typ.Provider, model string) OpenAIClientInterface {
	sessionID := typ.GetSessionID(ctx)
	logrus.WithContext(ctx).Debugf("Creating OpenAI client for provider: %s, session: %s", provider.Name, sessionID.Value)

	var client OpenAIClientInterface
	var err error

	// Check if this is a Codex OAuth provider
	if provider.IsOAuth() && provider.OAuthDetail != nil {
		switch issuer := provider.OAuthDetail.GetIssuer(); issuer {
		case ai.IssuerCodex:
			client, err = NewCodexClient(provider, model, sessionID)
			if err != nil {
				logrus.WithContext(ctx).Errorf("Failed to create Codex client for provider %s: %v", provider.Name, err)
				return nil
			}
		case ai.IssuerKimiCode:
			// Check if this is a Kimi Code OAuth provider
			client, err = NewKimiClient(provider, model, sessionID)
			if err != nil {
				logrus.WithContext(ctx).Errorf("Failed to create Kimi client for provider %s: %v", provider.Name, err)
				return nil
			}
		default:
			logrus.Errorf("Unsupported oauth issuer: %s", issuer)
			return nil
		}
	} else if provider.AuthType == typ.AuthTypeAzureKey {
		// GPT / o-series on Azure OpenAI (api-key auth).
		client, err = NewAzureClient(provider, model, sessionID)
		if err != nil {
			logrus.WithContext(ctx).Errorf("Failed to create Azure client for provider %s: %v", provider.Name, err)
			return nil
		}
	} else {
		client, err = NewOpenAIClient(provider, model, sessionID)
		if err != nil {
			logrus.WithContext(ctx).Errorf("Failed to create OpenAI client for provider %s: %v", provider.Name, err)
			return nil
		}
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	// This ensures idle connections are closed without requiring explicit Close() calls.
	// Capture only the name: the closure outlives the request, and holding the
	// provider would pin per-request ResolveStyle clones until finalization.
	providerName := provider.Name
	runtime.SetFinalizer(client, func(c OpenAIClientInterface) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed OpenAI client for provider: %s via finalizer", providerName)
		}
	})

	return client
}

// GetAnthropicClient returns an Anthropic client wrapper for the specified provider.
// For Claude Code OAuth providers, returns a ClaudeClient with special handling.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetAnthropicClient(ctx context.Context, provider *typ.Provider, model string) AnthropicClientInterface {
	sessionID := typ.GetSessionID(ctx)
	logrus.WithContext(ctx).Debugf("Creating Anthropic client for provider: %s, session: %s", provider.Name, sessionID.Value)

	var client AnthropicClientInterface
	var err error

	// Check if this is a Claude Code OAuth provider
	switch {
	case provider.IsClaudeCodeProvider():
		client, err = NewClaudeClient(ctx, provider, model, sessionID)
	case provider.AuthType == typ.AuthTypeAWSSigV4:
		// Claude on Amazon Bedrock (SigV4 / Bedrock bearer token).
		client, err = NewBedrockClient(provider, model, sessionID)
	case provider.AuthType == typ.AuthTypeGCPVertex:
		// Claude on GCP Vertex AI (service-account OAuth2).
		client, err = NewVertexAnthropicClient(provider, model, sessionID)
	default:
		client, err = NewAnthropicClient(provider, model, sessionID)
	}
	if err != nil {
		logrus.WithContext(ctx).Errorf("Failed to create Anthropic client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	// Name-only capture, same rationale as GetOpenAIClient.
	providerName := provider.Name
	runtime.SetFinalizer(client, func(c AnthropicClientInterface) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed Anthropic client for provider: %s via finalizer", providerName)
		}
	})

	return client
}

// GetGoogleClient returns a Google client wrapper for the specified provider.
// For Gemini CLI / Antigravity OAuth providers it dispatches to the dedicated
// xxx_client constructors, which layer the Code Assist envelope transport.
// sessionID is resolved from ctx via typ.GetSessionID; pass context.Background() when no session is available.
func (p *ClientPool) GetGoogleClient(ctx context.Context, provider *typ.Provider, model string) *GoogleClient {
	sessionID := typ.GetSessionID(ctx)
	logrus.WithContext(ctx).Debugf("Creating Google client for provider: %s, session: %s", provider.Name, sessionID.Value)

	client, err := newGoogleClientForProvider(provider, model, sessionID)
	if err != nil {
		logrus.WithContext(ctx).Errorf("Failed to create Google client for provider %s: %v", provider.Name, err)
		return nil
	}

	// Set finalizer for automatic cleanup when GC collects the client.
	// Name-only capture, same rationale as GetOpenAIClient.
	providerName := provider.Name
	runtime.SetFinalizer(client, func(c *GoogleClient) {
		if c != nil {
			c.Close()
			logrus.Debugf("Auto-closed Google client for provider: %s via finalizer", providerName)
		}
	})

	return client
}

// newGoogleClientForProvider selects the right Google-flavored constructor
// based on the provider's OAuth issuer. Returns *GoogleClient so existing
// forwarding code keeps working unchanged — the provider-specific transport
// is already baked into the embedded client.
func newGoogleClientForProvider(provider *typ.Provider, model string, sessionID typ.SessionID) (*GoogleClient, error) {
	switch provider.OAuthIssuer() {
	case ai.IssuerGemini:
		c, err := NewGeminiClient(provider, model, sessionID)
		if err != nil {
			return nil, err
		}
		return c.GoogleClient, nil
	case ai.IssuerAntigravity:
		c, err := NewAntigravityClient(provider, model, sessionID)
		if err != nil {
			return nil, err
		}
		return c.GoogleClient, nil
	}
	return NewGoogleClient(provider, model, sessionID)
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
	GetGlobalTransportPool().InvalidateProvider(providerUUID)
	logrus.Infof("Invalidated transport pool entries for provider UUID: %s", providerUUID)
}

// Stats provides statistics about the client pool and transport pool.
func (p *ClientPool) Stats() map[string]interface{} {
	// Return transport pool stats since that's where the real pooling happens
	transportStats := GetGlobalTransportPool().Stats()

	return map[string]interface{}{
		"mode":           "once",
		"transport_pool": transportStats,
	}
}
