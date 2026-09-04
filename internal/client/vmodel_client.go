package client

import (
	"errors"
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/typ"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
)

var errNoVModelTransport = errors.New("no virtual-model transport configured for this ClientPool")

// NewVModelOpenAIClient builds an OpenAI client for a virtual-model provider
// over base, the transport that dials this process's virtualserver
// (vmodelclient.NewTransport). It layers vmodel/client on the generic OpenAI
// client the way the Azure constructor layers the azure adapter: SDK,
// retries-off, request timeout and the rule-flag / advisor / logging chain are
// the generic ones; only the base URL (vmodel:// → http://vmodel.internal/...)
// and the base transport differ. No pooled network transport is created.
func NewVModelOpenAIClient(provider *typ.Provider, base http.RoundTripper) (*OpenAIClient, error) {
	if base == nil {
		return nil, errNoVModelTransport
	}
	return newOpenAIClientWithTransport(provider,
		vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle),
		providerTransportChain(base, provider))
}

// NewVModelAnthropicClient is the Anthropic counterpart of NewVModelOpenAIClient.
func NewVModelAnthropicClient(provider *typ.Provider, base http.RoundTripper) (*AnthropicClient, error) {
	if base == nil {
		return nil, errNoVModelTransport
	}
	return newAnthropicClientWithTransport(provider,
		vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle),
		providerTransportChain(base, provider))
}
