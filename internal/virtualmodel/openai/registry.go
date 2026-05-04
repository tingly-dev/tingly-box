package openai

import (
	"github.com/tingly-dev/tingly-box/internal/virtualmodel"
)

// Registry holds OpenAI Chat-protocol virtual models. It is a thin alias around
// virtualmodel.GenericRegistry instantiated for the OpenAI VirtualModel
// sub-interface.
type Registry = virtualmodel.GenericRegistry[VirtualModel]

// NewRegistry creates a new OpenAI virtual model registry.
func NewRegistry() *Registry {
	return virtualmodel.NewGenericRegistry[VirtualModel]()
}
