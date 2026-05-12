package openai

import (
	"github.com/tingly-dev/tingly-box/vmodel"
)

// Registry holds OpenAI Chat-protocol virtual models. It is a thin alias around
// vmodel.GenericRegistry instantiated for the OpenAI VirtualModel
// sub-interface.
type Registry = vmodel.GenericRegistry[VirtualModel]

// NewRegistry creates a new OpenAI virtual model registry.
func NewRegistry() *Registry {
	return vmodel.NewGenericRegistry[VirtualModel]()
}
