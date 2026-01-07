package llmclient

import (
	"tingly-box/internal/llmclient/httpclient"
	"tingly-box/internal/typ"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/sirupsen/logrus"
)

// OpenAIClient wraps the OpenAI SDK client
type OpenAIClient struct {
	client   openai.Client
	provider *typ.Provider
}

// defaultNewOpenAIClient creates a new OpenAI client wrapper
func defaultNewOpenAIClient(provider *typ.Provider) (*OpenAIClient, error) {
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
	}

	// Add proxy if configured
	if provider.ProxyURL != "" {
		httpClient := httpclient.CreateHTTPClientWithProxy(provider.ProxyURL)
		options = append(options, option.WithHTTPClient(httpClient))
		logrus.Infof("Using proxy for OpenAI client: %s", provider.ProxyURL)
	}

	openaiClient := openai.NewClient(options...)

	return &OpenAIClient{
		client:   openaiClient,
		provider: provider,
	}, nil
}

// ProviderType returns the provider type
func (c *OpenAIClient) ProviderType() ProviderType {
	return ProviderTypeOpenAI
}

// Close closes any resources held by the client
func (c *OpenAIClient) Close() error {
	// OpenAI client doesn't need explicit closing
	return nil
}

// Client returns the underlying OpenAI SDK client
func (c *OpenAIClient) Client() *openai.Client {
	return &c.client
}
