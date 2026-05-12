package anthropic

import (
	"github.com/tingly-dev/tingly-box/vmodel"
)

// Registry holds Anthropic-protocol virtual models. It is a thin alias around
// vmodel.GenericRegistry instantiated for the Anthropic VirtualModel
// sub-interface — all behavior lives in the generic implementation.
type Registry = vmodel.GenericRegistry[VirtualModel]

// NewRegistry creates a new Anthropic virtual model registry.
func NewRegistry() *Registry {
	return vmodel.NewGenericRegistry[VirtualModel]()
}
