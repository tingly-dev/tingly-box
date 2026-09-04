package virtualserver

import (
	"net/url"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// Scheme is the URL scheme of a virtual-model provider's APIBase. The host
// names the protocol root served by Server:
//
//	vmodel://openai     → /openai/v1/{models,chat/completions,responses}
//	vmodel://anthropic  → /anthropic/v1/{models,messages}
//
// A provider with such an APIBase is dispatched like any other provider
// (official SDK, standard transport chain); only the dialer differs, see
// Transport. See .design/vmodel-transport.md.
const Scheme = "vmodel"

// Host is the placeholder host the SDKs are pointed at. It never resolves on
// any network; Server's dialer ignores it and connects to the in-memory
// listener directly.
const Host = "vmodel.internal"

// APIBase returns the canonical APIBase for a vmodel provider serving style.
func APIBase(style protocol.APIStyle) string {
	return Scheme + "://" + string(style)
}

// IsAPIBase reports whether apiBase carries the vmodel:// scheme.
func IsAPIBase(apiBase string) bool {
	u, err := url.Parse(strings.TrimSpace(apiBase))
	return err == nil && u.Scheme == Scheme
}

// HTTPBase translates a vmodel APIBase into the http base URL handed to the
// SDKs, e.g. http://vmodel.internal/openai/v1. Like a real provider's APIBase
// it ends in /v1: the OpenAI SDK uses it as-is, the Anthropic client strips it.
//
// The protocol root comes from the URL host. The legacy "vmodel://local"
// sentinel, or any unrecognised host, falls back to style so existing rows
// keep working until the builtin seed rewrites them.
func HTTPBase(apiBase string, style protocol.APIStyle) string {
	root := ""
	if u, err := url.Parse(strings.TrimSpace(apiBase)); err == nil && u.Scheme == Scheme {
		switch protocol.APIStyle(u.Host) {
		case protocol.APIStyleOpenAI, protocol.APIStyleAnthropic:
			root = u.Host
		}
	}
	if root == "" {
		root = string(style)
		if root == "" {
			root = string(protocol.APIStyleOpenAI)
		}
	}
	return "http://" + Host + "/" + root + "/v1"
}
