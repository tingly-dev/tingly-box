package oauth

import (
	"github.com/google/uuid"

	"github.com/tingly-dev/tingly-box/ai/oauth"
)

// kimiDeviceIDHeader is the HTTP header Kimi's auth and inference endpoints
// use to bind a credential to a specific device.
const kimiDeviceIDHeader = "X-Msh-Device-Id"

// newKimiDeviceID returns a fresh device id for a new Kimi OAuth flow.
// Matches CLIProxyAPI's per-Auth instance generation, but our generated id
// rides along with the credential in the DB rather than a kimi-cli file.
func newKimiDeviceID() string {
	return uuid.New().String()
}

// WithKimiDeviceID returns an oauth.Option that pins X-Msh-Device-Id on
// every token-related request made through the OAuth manager during a flow.
// Exported so the background refresher can rehydrate the same id on refresh
// without duplicating the header name.
func WithKimiDeviceID(deviceID string) oauth.Option {
	return oauth.WithExtraHeader(kimiDeviceIDHeader, deviceID)
}

// KimiDeviceIDFromExtraFields safely extracts the persisted Kimi device id
// from OAuthDetail.ExtraFields. Returns "" when missing or malformed.
func KimiDeviceIDFromExtraFields(extra map[string]interface{}) string {
	if extra == nil {
		return ""
	}
	if v, ok := extra[oauth.KimiDeviceIDMetadataKey].(string); ok {
		return v
	}
	return ""
}
