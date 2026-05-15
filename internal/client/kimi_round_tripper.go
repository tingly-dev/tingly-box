package client

import (
	"net/http"
	"os"
	"runtime"

	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Kimi CLI client identification headers used when calling Kimi's coding API.
// Reference: https://github.com/router-for-me/CLIProxyAPI internal/runtime/executor/kimi_executor.go
const (
	kimiCLIUserAgent = "KimiCLI/1.10.6"
	kimiCLIPlatform  = "kimi_cli"
	kimiCLIVersion   = "1.10.6"
)

// kimiRoundTripper layers the kimi-cli impersonation headers on top of an
// inner transport. The Authorization Bearer header is set by the OpenAI SDK
// from the provider's access token.
//
// deviceID is bound at construction time from provider.OAuthDetail.ExtraFields
// so each Kimi account uses the same id it was OAuth'd with — matching
// CLIProxyAPI's per-token device-id binding, persisted to our credential DB
// instead of a kimi-cli file path.
type kimiRoundTripper struct {
	http.RoundTripper
	deviceID string
}

func newKimiRoundTripper(inner http.RoundTripper, provider *typ.Provider) *kimiRoundTripper {
	return &kimiRoundTripper{
		RoundTripper: inner,
		deviceID:     kimiDeviceIDForProvider(provider),
	}
}

func (t *kimiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", kimiCLIUserAgent)
	req.Header.Set("X-Msh-Platform", kimiCLIPlatform)
	req.Header.Set("X-Msh-Version", kimiCLIVersion)
	req.Header.Set("X-Msh-Device-Name", kimiDeviceName())
	req.Header.Set("X-Msh-Device-Model", kimiDeviceModel())
	if t.deviceID != "" {
		req.Header.Set("X-Msh-Device-Id", t.deviceID)
	}
	return t.RoundTripper.RoundTrip(req)
}

// kimiDeviceIDForProvider returns the device id persisted with the Kimi
// credential at OAuth time. The OAuth flow stores it on the token metadata,
// which the provider creation path copies into OAuthDetail.ExtraFields.
func kimiDeviceIDForProvider(provider *typ.Provider) string {
	if provider == nil || provider.OAuthDetail == nil || provider.OAuthDetail.ExtraFields == nil {
		return ""
	}
	if v, ok := provider.OAuthDetail.ExtraFields[oauth.KimiDeviceIDMetadataKey].(string); ok {
		return v
	}
	return ""
}

func kimiDeviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "unknown"
}

func kimiDeviceModel() string {
	goos := runtime.GOOS
	switch goos {
	case "darwin":
		goos = "macOS"
	case "linux":
		goos = "Linux"
	case "windows":
		goos = "Windows"
	}
	return goos + " " + runtime.GOARCH
}
