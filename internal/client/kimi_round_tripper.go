package client

import (
	"net/http"

	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Kimi-cli impersonation values sent with every inference request.
// Reference: CLIProxyAPI internal/runtime/executor/kimi_executor.go.
const (
	kimiCLIUserAgent = "KimiCLI/1.10.6"
	kimiCLIPlatform  = "kimi_cli"
	kimiCLIVersion   = "1.10.6"
)

// kimiRoundTripper layers kimi-cli impersonation headers on an inner
// transport. The Authorization Bearer is set by the OpenAI SDK.
//
// All header values are bound at construction so per-request RoundTrip stays
// allocation-free: device id (per-credential, persisted in OAuthDetail.DeviceID),
// hostname (one os.Hostname syscall), and the GOOS/GOARCH-derived model.
type kimiRoundTripper struct {
	http.RoundTripper
	deviceID    string
	deviceName  string
	deviceModel string
	osVersion   string
}

func newKimiRoundTripper(inner http.RoundTripper, provider *typ.Provider) *kimiRoundTripper {
	var deviceID string
	if provider != nil && provider.OAuthDetail != nil {
		deviceID = provider.OAuthDetail.DeviceID
	}
	return &kimiRoundTripper{
		RoundTripper: inner,
		deviceID:     deviceID,
		deviceName:   oauth.KimiDeviceName(),
		deviceModel:  oauth.KimiDeviceModel(),
		osVersion:    oauth.KimiOsVersion(),
	}
}

func (t *kimiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", kimiCLIUserAgent)
	req.Header.Set("X-Msh-Platform", kimiCLIPlatform)
	req.Header.Set("X-Msh-Version", kimiCLIVersion)
	req.Header.Set("X-Msh-Device-Name", t.deviceName)
	req.Header.Set("X-Msh-Device-Model", t.deviceModel)
	req.Header.Set("X-Msh-Os-Version", t.osVersion)
	if t.deviceID != "" {
		req.Header.Set("X-Msh-Device-Id", t.deviceID)
	}

	return t.RoundTripper.RoundTrip(req)
}
