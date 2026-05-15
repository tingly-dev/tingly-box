package oauth

import "github.com/tingly-dev/tingly-box/ai/oauth"

// kimiDeviceIDHeader is the HTTP header Kimi's auth and inference endpoints
// use to bind a credential to a specific device.
const kimiDeviceIDHeader = "X-Msh-Device-Id"

// WithKimiDeviceID pins X-Msh-Device-Id on every token-related request the
// OAuth manager makes during a flow. Exported so the background refresher
// can rehydrate the same id on refresh.
func WithKimiDeviceID(deviceID string) oauth.Option {
	return oauth.WithExtraHeader(kimiDeviceIDHeader, deviceID)
}
