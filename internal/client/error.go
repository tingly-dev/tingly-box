package client

import "fmt"

// ErrModelsEndpointNotSupported is returned when the provider does not support the models endpoint
type ErrModelsEndpointNotSupported struct {
	Provider string
	Reason   string
}

func (e *ErrModelsEndpointNotSupported) Error() string {
	return fmt.Sprintf("models endpoint not supported for provider %s: %s", e.Provider, e.Reason)
}

// ErrCodexNotSupported is returned when attempting to use an OpenAI API that is not supported by Codex
type ErrCodexNotSupported struct {
	Operation string
	Reason    string
}

func (e *ErrCodexNotSupported) Error() string {
	return fmt.Sprintf("Codex does not support %s: %s", e.Operation, e.Reason)
}

// ErrKimiNotSupported is returned when attempting to use an OpenAI API that is not supported by Kimi Code
type ErrKimiNotSupported struct {
	Operation string
	Reason    string
}

func (e *ErrKimiNotSupported) Error() string {
	return fmt.Sprintf("Kimi Code does not support %s: %s", e.Operation, e.Reason)
}
