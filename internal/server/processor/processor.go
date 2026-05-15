package processor

import (
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/client"
	smartrouting "github.com/tingly-dev/tingly-box/internal/smart_routing"
)

// RegisterAll wires every op-level processor in this package into the
// shared smart-routing registry. Called once during server boot after the
// ClientPool and config (provider resolver) are constructed. Idempotent —
// re-registration silently replaces, so config reloads are safe.
func RegisterAll(pool *client.ClientPool, resolver providerResolver, logger *logrus.Logger) {
	visionProc := &VisionProxyProcessor{
		Client:   NewPoolVisionClient(pool, resolver, logger),
		Resolver: resolver,
		Logger:   logger,
	}
	smartrouting.RegisterProcessor(
		smartrouting.PositionProxyVision,
		smartrouting.OpProxyVisionEnabled,
		visionProc,
	)
}
