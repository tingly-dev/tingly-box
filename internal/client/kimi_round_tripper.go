package client

import (
	"net/http"
	"os"
	"runtime"

	"github.com/tingly-dev/tingly-box/ai/oauth"
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
type kimiRoundTripper struct {
	http.RoundTripper
}

func (t *kimiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", kimiCLIUserAgent)
	req.Header.Set("X-Msh-Platform", kimiCLIPlatform)
	req.Header.Set("X-Msh-Version", kimiCLIVersion)
	req.Header.Set("X-Msh-Device-Name", kimiDeviceName())
	req.Header.Set("X-Msh-Device-Model", kimiDeviceModel())
	req.Header.Set("X-Msh-Device-Id", oauth.GetKimiDeviceID())
	return t.RoundTripper.RoundTrip(req)
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
