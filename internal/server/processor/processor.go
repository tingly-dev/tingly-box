package processor

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
)

// RegisterAll builds the op-level processors this package owns. Called once
// during server boot after the ClientPool and config (provider resolver) are
// constructed.
//
// It returns the VisionProxyProcessor, which the server invokes directly via
// the vision proxy plugin (rule-level and scenario-level; see
// internal/server/vision_proxy.go). The proxy no longer registers a
// smart-routing op — that path was removed in favor of the rule/scenario
// flags.
func RegisterAll(pool *client.ClientPool, resolver providerResolver, logger *logrus.Logger) *VisionProxyProcessor {
	return &VisionProxyProcessor{
		Client:   NewPoolVisionClient(pool, resolver, logger),
		Resolver: resolver,
	}
}
