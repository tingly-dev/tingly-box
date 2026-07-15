package client

import (
	"fmt"

	"github.com/openai/openai-go/v3/azure"
	openaiOption "github.com/openai/openai-go/v3/option"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// NewAzureClient builds an OpenAI-compatible client that targets Azure OpenAI.
// Azure uses the same Chat/Responses contract as OpenAI but with a deployment-
// shaped URL, an api-version query parameter, and an api-key header instead of a
// bearer token. The openai-go/azure adapter handles that rewrite; this
// constructor supplies endpoint/version/key from the stored bundle and layers it
// on the generic OpenAI client (which keeps proxy, User-Agent, logging, timeout).
func NewAzureClient(provider *typ.Provider, model string, sessionID typ.SessionID) (*OpenAIClient, error) {
	opts, err := azureOptions(provider)
	if err != nil {
		return nil, err
	}
	return NewOpenAIClient(provider, model, sessionID, opts...)
}

// azureOptions resolves the Azure adapter RequestOptions from a provider's
// Azure credential bundle.
func azureOptions(provider *typ.Provider) ([]openaiOption.RequestOption, error) {
	if provider.Credential == nil {
		return nil, fmt.Errorf("provider %q: missing credential bundle for auth type %s", provider.Name, provider.AuthType)
	}
	if err := ai.ValidateCredential(provider.AuthType, provider.Credential.Fields); err != nil {
		return nil, err
	}
	return []openaiOption.RequestOption{
		azure.WithEndpoint(
			provider.Credential.Field(ai.CredFieldAzureEndpoint),
			provider.Credential.Field(ai.CredFieldAzureAPIVersion),
		),
		azure.WithAPIKey(provider.Credential.Field(ai.CredFieldAzureAPIKey)),
	}, nil
}
