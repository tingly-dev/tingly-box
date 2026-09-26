// Package upstream provides the terminal Protocol Stage endpoints: one per
// chain protocol, each forwarding a native request to the selected provider
// through the existing internal/forwarding functions.
//
// Endpoints only forward. Request preparation (vendor transforms, field
// cleanup, consistency) runs in stages before the terminal, exactly as the
// transform chain runs before forwarding today, and response shaping for the
// client (model rewrite, usage stripping) belongs to the client edge.
//
// Anthropic is Beta inside the chain. A provider reached over the V1 wire is
// served by the same Beta endpoint with AnthropicWireV1: the request is
// downgraded just before forwarding and the response upgraded right after, so
// the provider sees exactly what the V1 path sends today.
package upstream

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Clients resolves provider clients; *client.ClientPool implements it.
type Clients interface {
	GetAnthropicClient(ctx context.Context, provider *typ.Provider, model string) client.AnthropicClientInterface
	GetOpenAIClient(ctx context.Context, provider *typ.Provider, model string) client.OpenAIClientInterface
}

// Config selects the provider and provider-bound model for one attempt.
type Config struct {
	Clients  Clients
	Provider *typ.Provider
	// Model is the provider-bound model used to resolve the client. It is also
	// the fallback Response.Model when the provider reports none.
	Model string
	// StreamOnly marks a provider that only streams (Codex): a complete call
	// is sent as a stream and answered with the stream's final response.
	StreamOnly bool
}

func (c Config) validate() error {
	if c.Clients == nil {
		return fmt.Errorf("upstream endpoint: clients are required")
	}
	if c.Provider == nil {
		return fmt.Errorf("upstream endpoint: provider is required")
	}
	return nil
}

func (c Config) forwardContext(ctx context.Context) *forwarding.ForwardContext {
	return forwarding.NewForwardContext(ctx, c.Provider)
}

func (c Config) model(reported string) string {
	if reported != "" {
		return reported
	}
	return c.Model
}
