package forwarding

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/tingly-dev/tingly-box/internal/client"
)

// ForwardOpenAIAudio retains the timeout until the downstream consumes and
// closes the body. It never retries a partially emitted audio response.
func ForwardOpenAIAudio(fc *ForwardContext, wrapper client.OpenAIClientInterface, path, contentType string, body io.Reader) (*http.Response, context.CancelFunc, error) {
	audio, ok := wrapper.(interface {
		AudioRequest(context.Context, string, string, io.Reader) (*http.Response, error)
	})
	if !ok {
		return nil, nil, fmt.Errorf("provider does not support the OpenAI Audio API")
	}
	ctx, cancel := fc.PrepareContext(nil)
	response, err := audio.AudioRequest(ctx, path, contentType, body)
	fc.Complete(ctx, response, err)
	return response, cancel, err
}
