package client

import (
	"context"
	"io"
	"net/http"

	"github.com/openai/openai-go/v3/option"
)

// AudioRequest preserves the audio wire format and unknown vendor parameters.
// Returning the response body lets the gateway stream binary audio and SSE
// without buffering. The SDK still owns auth, proxy, rule headers and errors.
// Only the standard OpenAI client implements this capability: agent OAuth
// backends (Codex/Kimi) do not expose the public Audio API.
func (c *OpenAIClient) AudioRequest(ctx context.Context, path, contentType string, body io.Reader) (*http.Response, error) {
	var response *http.Response
	err := c.client.Post(ctx, path, nil, &response, option.WithRequestBody(contentType, body))
	return response, err
}
