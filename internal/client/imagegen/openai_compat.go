package imagegen

import (
	"context"
	"net/http"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// openAICompatClient handles every provider that exposes the OpenAI
// /images/generations contract. It wraps the openai-go SDK and simply
// translates between the normalized Request/Response and the SDK types.
type openAICompatClient struct {
	provider   *typ.Provider
	client     openai.Client
	httpClient *http.Client
}

func newOpenAICompatClient(provider *typ.Provider) (*openAICompatClient, error) {
	httpClient := &http.Client{Transport: http.DefaultTransport}
	options := []option.RequestOption{
		option.WithAPIKey(provider.GetAccessToken()),
		option.WithBaseURL(provider.APIBase),
		option.WithMaxRetries(0),
		option.WithHTTPClient(httpClient),
	}
	return &openAICompatClient{
		provider:   provider,
		client:     openai.NewClient(options...),
		httpClient: httpClient,
	}, nil
}

func (c *openAICompatClient) Provider() *typ.Provider { return c.provider }

func (c *openAICompatClient) Vendor() Vendor { return VendorOpenAICompat }

func (c *openAICompatClient) Close() error {
	if c.httpClient != nil && c.httpClient != http.DefaultClient {
		c.httpClient.CloseIdleConnections()
	}
	return nil
}

func (c *openAICompatClient) Generate(ctx context.Context, req *Request) (*Response, error) {
	params := req.ToOpenAIParams()
	resp, err := c.client.Images.Generate(ctx, params)
	if err != nil {
		return nil, err
	}
	return ResponseFromOpenAI(req.Model, resp), nil
}

// ResponseFromOpenAI normalizes an OpenAI SDK ImagesResponse. It is exported so
// callers that still go through the OpenAI client wrapper (e.g. the Codex
// Responses-API path) can converge on the same normalized Response type.
func ResponseFromOpenAI(model string, resp *openai.ImagesResponse) *Response {
	if resp == nil {
		return &Response{Model: model}
	}
	out := &Response{
		Created: resp.Created,
		Model:   model,
		Usage: Usage{
			InputTokens:  resp.Usage.InputTokens,
			OutputTokens: resp.Usage.OutputTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		},
	}
	out.Data = make([]Image, 0, len(resp.Data))
	for _, img := range resp.Data {
		out.Data = append(out.Data, Image{
			URL:           img.URL,
			B64JSON:       img.B64JSON,
			RevisedPrompt: img.RevisedPrompt,
		})
	}
	return out
}
