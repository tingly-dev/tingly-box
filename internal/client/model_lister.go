package client

import (
	"context"
	"fmt"
)

// ErrModelsEndpointNotSupported is returned when the provider does not support the models endpoint
type ErrModelsEndpointNotSupported struct {
	Provider string
	Reason   string
}

func (e *ErrModelsEndpointNotSupported) Error() string {
	return fmt.Sprintf("models endpoint not supported for provider %s: %s", e.Provider, e.Reason)
}

// IsModelsEndpointNotSupported checks if an error is ErrModelsEndpointNotSupported
func IsModelsEndpointNotSupported(err error) bool {
	_, ok := err.(*ErrModelsEndpointNotSupported)
	return ok
}

// ModelLister defines the interface for fetching model lists from provider APIs
type ModelLister interface {
	// ListModels returns the list of available models from the provider API
	// Returns ErrModelsEndpointNotSupported if the provider does not support the models endpoint
	ListModels(ctx context.Context) ([]string, error)
}
