// Package virtualmodel defines the protocol-agnostic primitives for virtual
// models. Concrete provider-specific implementations and registries live in
// the anthropic and openai sub-packages — this top-level package contains
// only the base interface and shared value types.
//
// These primitives back the production /virtual/v1/* endpoint (onboarding,
// demos, dry-runs without a real upstream provider) and are also reused as
// an in-process LLM substitute by test packages such as server_validate
// and protocol_validate. The production endpoint is the package's primary
// surface; test consumers are secondary and use GenericRegistry directly
// rather than RegisterDefaults — see README.md "Positioning & registration
// discipline".
package vmodel

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
