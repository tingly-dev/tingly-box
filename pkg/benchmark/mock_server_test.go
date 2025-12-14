package benchmark

import (
	"net/http"
	"testing"
)

func ExampleWithApiKey() {
	// Create a mock server with authentication
	server := NewMockServer(
		WithPort(8081),
		WithApiKey("test-api-key-123"),
		WithOpenAIDefaults(),
	)

	// The auth middleware will be automatically applied when the server starts
	// Clients must provide either:
	// - Authorization: Bearer test-api-key-123
	// - x-api-key: test-api-key-123
	//
	// If WithapiKey is not called or the key is empty, no authentication is required.

	server.Start()
}

func ExampleMockServer() {
	// Create a mock server without authentication (default behavior)
	server := NewMockServer(
		WithPort(8082),
		WithOpenAIDefaults(),
		// No WithapiKey called - all requests allowed
	)

	server.Start()
}

func ExampleMockServer_UseAuthMiddleware() {
	// Create a mock server
	server := NewMockServer(
		WithPort(8083),
		WithApiKey("my-secret-key"),
	)

	// Manually apply auth middleware (useful if you want to apply it conditionally)
	server.UseAuthMiddleware()

	server.Start()
}

// This example shows how the auth middleware handles different scenarios
func TestAuthMiddleware(t *testing.T) {
	tests := []struct {
		name           string
		apiKey        string
		headers        map[string]string
		expectedStatus int
	}{
		{
			name:           "No auth key configured - should allow all",
			apiKey:        "",
			headers:        map[string]string{},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Auth key configured with valid Bearer token",
			apiKey: "test-key",
			headers: map[string]string{
				"Authorization": "Bearer test-key",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Auth key configured with valid x-api-key",
			apiKey: "test-key",
			headers: map[string]string{
				"x-api-key": "test-key",
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:    "Auth key configured with invalid token",
			apiKey: "test-key",
			headers: map[string]string{
				"Authorization": "Bearer wrong-key",
			},
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "Auth key configured with no auth headers",
			apiKey:        "test-key",
			headers:        map[string]string{},
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// This would require setting up a test server
			// Implementation omitted for brevity
		})
	}
}
