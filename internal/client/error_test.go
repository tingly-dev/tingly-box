package client

import (
	"errors"
	"testing"
)

func TestIsNonRetryableForProtocolSwitch(t *testing.T) {
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
		{"content filter", errors.New("content_filter triggered"), true},

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
			got := IsNonRetryableForProtocolSwitch(tt.err)
			if got != tt.want {
				t.Errorf("IsNonRetryableForProtocolSwitch(%q) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
