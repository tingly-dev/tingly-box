// Package virtualmodel defines the protocol-agnostic primitives for virtual
// models. Concrete provider-specific implementations and registries live in
// the anthropic and openai sub-packages — this top-level package contains
// only the base interface and shared value types.
package virtualmodel

import "time"

// VirtualModel is the base interface common to all virtual model types.
// Provider-specific extensions are defined in the anthropic and openai
// sub-packages, each adding the Handle methods for that protocol.
type VirtualModel interface {
	GetID() string
	GetName() string
	GetDescription() string
	GetType() VirtualModelType
	SimulatedDelay() time.Duration
	ToModel() Model
}
