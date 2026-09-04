package client

import (
	"net/http"
	"strings"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
	openaiOption "github.com/openai/openai-go/v3/option"

	"github.com/tingly-dev/tingly-box/internal/typ"
	vmodelclient "github.com/tingly-dev/tingly-box/vmodel/client"
)

// NewVModelOpenAIClient builds an OpenAI client for a virtual-model provider.
// It layers vmodel/client on the generic OpenAI client, exactly like the Azure
// constructor layers the azure adapter: the SDK, retries-off, request timeout
// and rule-flag / advisor / logging transport chain are the generic ones; only
// the base URL (vmodel:// → http://vmodel.internal/...) and the dialer (the
// private in-process virtualserver listener) come from vmodel/client.
func NewVModelOpenAIClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*OpenAIClient, error) {
	return NewOpenAIClient(provider, model, sessionID,
		openaiOption.WithBaseURL(vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle)),
		openaiOption.WithHTTPClient(&http.Client{Transport: vmodelTransportChain(provider)}),
	)
}

// NewVModelAnthropicClient is the Anthropic counterpart of NewVModelOpenAIClient.
// The Anthropic SDK appends /v1 itself, so the /v1 suffix of HTTPBase is
// dropped here the same way NewAnthropicClient drops it from a real APIBase.
func NewVModelAnthropicClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*AnthropicClient, error) {
	base := strings.TrimSuffix(vmodelclient.HTTPBase(provider.APIBase, provider.APIStyle), "/v1")
	return NewAnthropicClient(provider, model, sessionID,
		anthropicOption.WithBaseURL(base),
		anthropicOption.WithHTTPClient(&http.Client{Transport: vmodelTransportChain(provider)}),
	)
}

// vmodelTransportChain is the generic provider chain (rule flags, advisor
// loopback stamp, logging) over the vmodel dialer instead of a pooled network
// transport.
func vmodelTransportChain(provider *typ.Provider) http.RoundTripper {
	return wrapWithLogging(wrapWithAdvisorLoopback(wrapWithRuleFlags(vmodelclient.Transport(), provider, true)), provider)
}
