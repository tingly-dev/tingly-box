package protocol

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"google.golang.org/genai"
)

// UpstreamStatus extracts the HTTP status code that an upstream provider
// returned, so the gateway can propagate it to the client instead of flattening
// every forwarding failure into a 500. It understands the error types returned
// by each vendor SDK (OpenAI / Anthropic share apierror.Error; google-genai
// uses genai.APIError). When the error does not carry a usable upstream status
// (e.g. a transport-level failure with no HTTP response), it returns fallback.
func UpstreamStatus(err error, fallback int) int {
	if err == nil {
		return fallback
	}

	var oaiErr *openai.Error
	if errors.As(err, &oaiErr) && oaiErr.StatusCode >= 400 {
		return oaiErr.StatusCode
	}

	var anthropicErr *anthropic.Error
	if errors.As(err, &anthropicErr) && anthropicErr.StatusCode >= 400 {
		return anthropicErr.StatusCode
	}

	var genaiErr genai.APIError
	if errors.As(err, &genaiErr) && genaiErr.Code >= 400 {
		return genaiErr.Code
	}

	return fallback
}
