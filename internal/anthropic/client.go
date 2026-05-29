package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// newClient builds an Anthropic SDK client pointed at the tingly-box gateway.
//
// option.WithAPIKey sets the X-Api-Key header (SDK option/requestoption.go), and
// the gateway's ModelAuthMiddleware accepts both Authorization and X-Api-Key
// (internal/server/middleware/auth.go), so WithAPIKey authenticates correctly.
// Do not "fix" this to WithAuthToken.
func newClient(baseURL, apiKey string) anthropic.Client {
	return anthropic.NewClient(
		option.WithBaseURL(baseURL),
		option.WithAPIKey(apiKey),
	)
}
