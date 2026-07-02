package client

import (
	"fmt"
	"strings"
)

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

// IsNonRetryableForProtocolSwitch reports whether err represents a condition
// where switching the OpenAI endpoint protocol (Chat ↔ Responses) would not
// help. Returns true for nil errors (nothing to retry), auth failures,
// rate-limiting, and content/model errors.
func IsNonRetryableForProtocolSwitch(err error) bool {
	if err == nil {
		return true
	}
	s := strings.ToLower(err.Error())

	for _, kw := range []string{
		"401", "403", "unauthorized", "forbidden",
		"429", "rate limit", "ratelimit", "1302",
		"context_length", "content_policy", "content_filter",
		"invalid_api_key", "model_not_found", "model not found",
	} {
		if strings.Contains(s, kw) {
			return true
		}
	}
	return false
}
