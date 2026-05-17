package server

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// EndpointOverride is the typed value of the openai_endpoint_override rule
// flag. It forces an OpenAI request onto a specific endpoint, bypassing the
// adaptive router's capability probe.
type EndpointOverride string

const (
	OverrideAuto      EndpointOverride = "auto"
	OverrideChat      EndpointOverride = "chat"
	OverrideResponses EndpointOverride = "responses"
)

// ParseEndpointOverride coerces a raw rule-flag string to a known
// EndpointOverride. Empty, "auto" and any unrecognized value map to
// OverrideAuto so misconfigured rules degrade safely.
func ParseEndpointOverride(s string) EndpointOverride {
	switch s {
	case string(OverrideChat):
		return OverrideChat
	case string(OverrideResponses):
		return OverrideResponses
	default:
		return OverrideAuto
	}
}

// logResponsesOnlyOverrideIgnored emits the warning that a "chat" override
// against a provider declared responses_only (e.g. Codex) was discarded.
func logResponsesOnlyOverrideIgnored(provider *typ.Provider) {
	uuid := ""
	if provider != nil {
		uuid = provider.UUID
	}
	logrus.Warnf("rule openai_endpoint_override=chat ignored: provider %s is responses_only", uuid)
}
