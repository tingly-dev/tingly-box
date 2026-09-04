package ai

import (
	"net/url"
	"strings"
)

// VModelScheme is the URL scheme of a virtual-model provider's APIBase.
//
// A vmodel provider is dispatched like every other provider: through the
// official SDK and the standard transport chain. The only vmodel-specific step
// is the dialer — the transport recognises this scheme and connects to the
// private virtualserver listener instead of the network. The host part names
// the protocol root on that server:
//
//	vmodel://openai     → <virtualserver>/openai/v1/...
//	vmodel://anthropic  → <virtualserver>/anthropic/v1/...
//
// See .design/vmodel-transport.md.
const VModelScheme = "vmodel"

// VModelHost is the placeholder host the vmodel dialer accepts. It never
// resolves on any network; the dialer ignores it and connects to the private
// listener directly.
const VModelHost = "vmodel.internal"

// VModelAPIBase returns the canonical APIBase for a vmodel provider that
// serves the given protocol style.
func VModelAPIBase(style APIStyle) string {
	return VModelScheme + "://" + string(style)
}

// IsVModelAPIBase reports whether apiBase carries the vmodel:// scheme.
func IsVModelAPIBase(apiBase string) bool {
	u, err := url.Parse(strings.TrimSpace(apiBase))
	return err == nil && u.Scheme == VModelScheme
}

// VModelHTTPBase translates a vmodel provider's APIBase into the plain http
// base URL the SDKs are pointed at, e.g. http://vmodel.internal/openai/v1.
// The /v1 suffix follows the convention real providers use for APIBase: the
// OpenAI SDK consumes it as-is, the Anthropic constructor strips it.
//
// The protocol root is taken from the URL host. Legacy rows carrying the old
// "vmodel://local" sentinel, or an unrecognised host, fall back to the
// provider's APIStyle so existing configurations keep working without a
// migration step.
func VModelHTTPBase(apiBase string, style APIStyle) string {
	root := ""
	if u, err := url.Parse(strings.TrimSpace(apiBase)); err == nil && u.Scheme == VModelScheme {
		switch APIStyle(u.Host) {
		case APIStyleOpenAI, APIStyleAnthropic:
			root = u.Host
		}
	}
	if root == "" {
		root = string(style)
		if root == "" {
			root = string(APIStyleOpenAI)
		}
	}
	return "http://" + VModelHost + "/" + root + "/v1"
}
