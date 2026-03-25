package hooks

import (
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// ClientPoolInvalidator defines the interface for client pool invalidation
type ClientPoolInvalidator interface {
	InvalidateProvider(providerUUID string)
}

// ClientPoolInvalidationHook invalidates client pool cache when provider credentials change
type ClientPoolInvalidationHook struct {
	clientPool ClientPoolInvalidator
}

// NewClientPoolInvalidationHook creates a new client pool invalidation hook
func NewClientPoolInvalidationHook(pool ClientPoolInvalidator) *ClientPoolInvalidationHook {
	return &ClientPoolInvalidationHook{
		clientPool: pool,
	}
}

// OnProviderUpdate is called when a provider is updated
func (h *ClientPoolInvalidationHook) OnProviderUpdate(provider *typ.Provider) {
	if h.clientPool != nil {
		h.clientPool.InvalidateProvider(provider.UUID)
		logrus.WithFields(logrus.Fields{
			"provider_uuid": provider.UUID,
			"provider_name": provider.Name,
			"auth_type":     provider.AuthType,
		}).Debug("Invalidated client pool cache after provider update")
	}
}

// OnProviderDelete is called when a provider is deleted
func (h *ClientPoolInvalidationHook) OnProviderDelete(uuid string) {
	if h.clientPool != nil {
		h.clientPool.InvalidateProvider(uuid)
		logrus.WithField("provider_uuid", uuid).
			Debug("Invalidated client pool cache after provider deletion")
	}
}
