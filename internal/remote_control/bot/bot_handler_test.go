package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
)

// TestSmartGuideFallback tests the SmartGuide auto-handoff when agent creation fails
func TestSmartGuideFallback(t *testing.T) {
	t.Run("CanCreateAgent_InvalidConfiguration", func(t *testing.T) {
		// Test CanCreateAgent with various invalid configurations
		testCases := []struct {
			name           string
			baseURL        string
			apiKey         string
			provider       string
			model          string
			expectedResult bool
			description    string
		}{
			{
				name:           "EmptyBaseURL",
				baseURL:        "",
				apiKey:         "",
				provider:       "test-provider",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when baseURL is empty",
			},
			{
				name:           "EmptyProvider",
				baseURL:        "http://localhost:8080",
				apiKey:         "",
				provider:       "",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when provider is empty",
			},
			{
				name:           "EmptyModel",
				baseURL:        "http://localhost:8080",
				apiKey:         "",
				provider:       "test-provider",
				model:          "",
				expectedResult: false,
				description:    "Should return false when model is empty",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := smart_guide.CanCreateAgent("", "", tc.provider, tc.model)
				assert.Equal(t, tc.expectedResult, result, tc.description)
			})
		}
	})
}

// TestSmartGuideConfigurationValidation tests various configuration scenarios
func TestSmartGuideConfigurationValidation(t *testing.T) {
	t.Run("NilBaseURL", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("", "", "provider-123", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when baseURL is empty")
	})

	t.Run("MissingProvider", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:8080", "", "", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when provider is empty")
	})

	t.Run("MissingModel", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:8080", "", "provider-123", "")
		assert.False(t, result, "Should return false when model is empty")
	})

	t.Run("ValidConfiguration", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:8080", "", "valid-provider", "valid-model")
		assert.True(t, result, "Should return true when all validations pass")
	})
}
