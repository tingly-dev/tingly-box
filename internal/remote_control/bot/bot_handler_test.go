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
				apiKey:         "test-key",
				provider:       "test-provider",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when baseURL is empty",
			},
			{
				name:           "EmptyAPIKey",
				baseURL:        "http://localhost:12580",
				apiKey:         "",
				provider:       "test-provider",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when apiKey is empty",
			},
			{
				name:           "EmptyProvider",
				baseURL:        "http://localhost:12580",
				apiKey:         "test-key",
				provider:       "",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when provider is empty",
			},
			{
				name:           "EmptyModel",
				baseURL:        "http://localhost:12580",
				apiKey:         "test-key",
				provider:       "test-provider",
				model:          "",
				expectedResult: false,
				description:    "Should return false when model is empty",
			},
			{
				name:           "ValidConfiguration",
				baseURL:        "http://localhost:12580",
				apiKey:         "test-key",
				provider:       "test-provider",
				model:          "test-model",
				expectedResult: true,
				description:    "Should return true when all parameters are valid",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := smart_guide.CanCreateAgent(tc.baseURL, tc.apiKey, tc.provider, tc.model)
				assert.Equal(t, tc.expectedResult, result, tc.description)
			})
		}
	})
}

// TestSmartGuideConfigurationValidation tests various configuration scenarios
func TestSmartGuideConfigurationValidation(t *testing.T) {
	t.Run("EmptyBaseURL", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("", "test-key", "provider-123", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when baseURL is empty")
	})

	t.Run("EmptyAPIKey", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:12580", "", "provider-123", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when apiKey is empty")
	})

	t.Run("MissingProvider", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:12580", "test-key", "", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when provider is empty")
	})

	t.Run("MissingModel", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:12580", "test-key", "provider-123", "")
		assert.False(t, result, "Should return false when model is empty")
	})

	t.Run("ValidConfiguration", func(t *testing.T) {
		result := smart_guide.CanCreateAgent("http://localhost:12580", "test-key", "valid-provider", "valid-model")
		assert.True(t, result, "Should return true when all validations pass")
	})
}

// mockTestError is a simple error type for testing
type mockTestError string

func (e mockTestError) Error() string {
	return string(e)
}
