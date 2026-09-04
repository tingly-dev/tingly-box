package client

import (
	"strings"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	openaiOption "github.com/openai/openai-go/v3/option"

	"github.com/tingly-dev/tingly-box/internal/typ"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
)

// NewVModelOpenAIClient builds an OpenAI client for a virtual-model provider.
// It layers vmodel/client on the generic OpenAI client, the way the Azure
// constructor layers the azure adapter: SDK, retries-off, request timeout and
// the rule-flag / advisor / logging transport chain are the generic ones; only
// the base URL (vmodel:// → http://vmodel.internal/...) and the base transport
// (the private in-process virtualserver listener) come from vmodel/client. No
// pooled network transport is created for the provider.
func NewVModelOpenAIClient(provider *typ.Provider, _ string, _ typ.SessionID) (*OpenAIClient, error) {
	return newOpenAIClientWithTransport(provider,
		openAITransportChain(vmodelclient.Transport(), provider),
		openaiOption.WithBaseURL(vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle)),
	)
}

// NewVModelAnthropicClient is the Anthropic counterpart of NewVModelOpenAIClient.
// The Anthropic SDK appends /v1 itself, so the /v1 suffix of HTTPBase is
// dropped here the same way NewAnthropicClient drops it from a real APIBase.
func NewVModelAnthropicClient(provider *typ.Provider, _ string, _ typ.SessionID) (*AnthropicClient, error) {
	base := strings.TrimSuffix(vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle), "/v1")
	return newAnthropicClientWithTransport(provider,
		anthropicTransportChain(vmodelclient.Transport(), provider),
		anthropicOption.WithBaseURL(base),
	)
}
