package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// newClient builds an Anthropic SDK client pointed at the tingly-box gateway.
//
// The gateway authenticates with an x-api-key header (Anthropic's native auth),
// so WithAPIKey is correct here rather than WithAuthToken.
func newClient(baseURL, apiKey string) anthropic.Client {
	return anthropic.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
}
