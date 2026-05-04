package anthropic

import (
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// Registry holds Anthropic-protocol virtual models. It is a thin alias around
// virtualmodel.GenericRegistry instantiated for the Anthropic VirtualModel
// sub-interface — all behavior lives in the generic implementation.
type Registry = virtualmodel.GenericRegistry[VirtualModel]

// NewRegistry creates a new Anthropic virtual model registry.
func NewRegistry() *Registry {
	return virtualmodel.NewGenericRegistry[VirtualModel]()
}
