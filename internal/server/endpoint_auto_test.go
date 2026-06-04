package server

import (
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

func TestIsNonRetryableForProtocolFallback(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, true},
		{"auth 401", errors.New("status 401 Unauthorized"), true},
		{"auth 403", errors.New("403 Forbidden"), true},
		{"rate limit 429", errors.New("status 429 Too Many Requests"), true},
		{"rate limit text", errors.New("rate limit exceeded"), true},
		{"rate limit 1302", errors.New("error code 1302"), true},
		{"context length", errors.New("context_length_exceeded"), true},
		{"content policy", errors.New("content_policy_violation"), true},
		{"invalid api key", errors.New("invalid_api_key"), true},
		{"model not found", errors.New("model_not_found"), true},
		{"model not found spaces", errors.New("model not found"), true},

		// Retryable cases
		{"404 not found", errors.New("status 404 Not Found"), false},
		{"500 internal", errors.New("status 500 Internal Server Error"), false},
		{"502 bad gateway", errors.New("502 Bad Gateway"), false},
		{"connection refused", errors.New("connection refused"), false},
		{"unknown error", errors.New("something went wrong"), false},
		{"empty error", errors.New(""), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNonRetryableForProtocolFallback(tt.err)
			if got != tt.want {
				t.Errorf("isNonRetryableForProtocolFallback(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestAlternateOpenAIProtocol(t *testing.T) {
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIChat); got != protocol.TypeOpenAIResponses {
		t.Errorf("alternate of chat = %v, want responses", got)
	}
	if got := alternateOpenAIProtocol(protocol.TypeOpenAIResponses); got != protocol.TypeOpenAIChat {
		t.Errorf("alternate of responses = %v, want chat", got)
	}
}

func TestIncomingToTarget(t *testing.T) {
	if got := incomingToTarget(IncomingAPIChat); got != protocol.TypeOpenAIChat {
		t.Errorf("incoming chat → %v, want chat", got)
	}
	if got := incomingToTarget(IncomingAPIResponses); got != protocol.TypeOpenAIResponses {
		t.Errorf("incoming responses → %v, want responses", got)
	}
}
