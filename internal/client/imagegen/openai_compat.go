package imagegen

import (
	"context"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// delegateClient wraps an externally-provided Generate function. It lets
// callers inject a transport-bound client (e.g. the pool's OpenAI client)
// without imagegen importing internal/client, avoiding an import cycle.
type delegateClient struct {
	provider *typ.Provider
	vendor   Vendor
	fn       func(ctx context.Context, req *Request) (*Response, error)
}

// NewDelegate creates a Client that delegates Generate to fn. Use it when
// the caller already holds a suitable client (e.g. for VendorOpenAICompat
// after New returns ErrDelegateRequired) rather than having imagegen
// construct a new transport client from scratch.
func NewDelegate(provider *typ.Provider, vendor Vendor, fn func(ctx context.Context, req *Request) (*Response, error)) Client {
	return &delegateClient{provider: provider, vendor: vendor, fn: fn}
}

func (c *delegateClient) Provider() *typ.Provider { return c.provider }
func (c *delegateClient) Vendor() Vendor          { return c.vendor }
func (c *delegateClient) Close() error            { return nil }

func (c *delegateClient) Generate(ctx context.Context, req *Request) (*Response, error) {
	return c.fn(ctx, req)
}

// ResponseFromOpenAI normalizes an OpenAI SDK ImagesResponse into the imagegen
// Response type. Exported so callers that go through the OpenAI client wrapper
// (e.g. the Codex Responses-API path and the OpenAI-compat delegate) can
// converge on the same normalized type.
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
