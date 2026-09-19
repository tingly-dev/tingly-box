package forwarding

import (
	"context"
	"fmt"
	"iter"

	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/client"
	"google.golang.org/genai"
)

// ForwardGoogle sends a non-streaming Google Generative AI request.
func ForwardGoogle(fc *ForwardContext, wrapper *client.GoogleClient, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (*genai.GenerateContentResponse, context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Google client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(nil)
	resp, err := wrapper.GenerateContent(ctx, model, contents, config)
	fc.Complete(ctx, resp, err)
	return resp, cancel, err
}

// ForwardGoogleStream sends a streaming Google Generative AI request.
// Note: Pass request context (c.Request.Context()) as baseCtx in NewForwardContext for client cancellation support.
func ForwardGoogleStream(fc *ForwardContext, wrapper *client.GoogleClient, model string, contents []*genai.Content, config *genai.GenerateContentConfig) (iter.Seq2[*genai.GenerateContentResponse, error], context.CancelFunc, error) {
	if wrapper == nil {
		return nil, nil, fmt.Errorf("failed to get Google client for provider: %s", fc.Provider.Name)
	}

	ctx, cancel := fc.PrepareContext(nil)
	logrus.Debugln("Creating Google streaming request")
	stream := wrapper.GenerateContentStream(ctx, model, contents, config)
	return stream, cancel, nil
}
