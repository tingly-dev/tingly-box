package bot

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/tingly-dev/tingly-box/internal/remote_control/smart_guide"
	"github.com/tingly-dev/tingly-box/internal/tbclient"
)

// TestSmartGuideFallback tests the SmartGuide auto-handoff when agent creation fails
func TestSmartGuideFallback(t *testing.T) {
	t.Run("CanCreateAgent_InvalidConfiguration", func(t *testing.T) {
		// Test CanCreateAgent with various invalid configurations
		testCases := []struct {
			name           string
			tbClient       tbclient.TBClient
			provider       string
			model          string
			expectedResult bool
			description    string
		}{
			{
				name:           "NilTBClient",
				tbClient:       nil,
				provider:       "test-provider",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when TBClient is nil",
			},
			{
				name:           "EmptyProvider",
				tbClient:       new(tbclient.MockTBClient),
				provider:       "",
				model:          "test-model",
				expectedResult: false,
				description:    "Should return false when provider is empty",
			},
			{
				name:           "EmptyModel",
				tbClient:       new(tbclient.MockTBClient),
				provider:       "test-provider",
				model:          "",
				expectedResult: false,
				description:    "Should return false when model is empty",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := smart_guide.CanCreateAgent(tc.tbClient, tc.provider, tc.model)
				assert.Equal(t, tc.expectedResult, result, tc.description)
			})
		}
	})
}

// TestSmartGuideConfigurationValidation tests various configuration scenarios
func TestSmartGuideConfigurationValidation(t *testing.T) {
	t.Run("NilTBClient", func(t *testing.T) {
		result := smart_guide.CanCreateAgent(nil, "provider-123", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when TBClient is nil")
	})

	t.Run("MissingProvider", func(t *testing.T) {
		mockClient := new(tbclient.MockTBClient)
		result := smart_guide.CanCreateAgent(mockClient, "", "claude-sonnet-4-6")
		assert.False(t, result, "Should return false when provider is empty")
	})

	t.Run("MissingModel", func(t *testing.T) {
		mockClient := new(tbclient.MockTBClient)
		result := smart_guide.CanCreateAgent(mockClient, "provider-123", "")
		assert.False(t, result, "Should return false when model is empty")
	})

	t.Run("SelectModelFails", func(t *testing.T) {
		mockClient := new(tbclient.MockTBClient)
		mockClient.On("SelectModel", mock.Anything, mock.Anything).Return(nil, mockTestError("provider not found"))
		result := smart_guide.CanCreateAgent(mockClient, "invalid-provider", "test-model")
		assert.False(t, result, "Should return false when SelectModel fails")
		mockClient.AssertExpectations(t)
	})

	t.Run("ValidConfiguration", func(t *testing.T) {
		mockClient := new(tbclient.MockTBClient)
		mockClient.On("SelectModel", mock.Anything, mock.Anything).Return(&tbclient.ModelConfig{
			ProviderUUID: "valid-provider",
			ModelID:      "valid-model",
			APIKey:       "test-key",
			BaseURL:      "http://localhost:8080",
		}, nil)
		result := smart_guide.CanCreateAgent(mockClient, "valid-provider", "valid-model")
		assert.True(t, result, "Should return true when all validations pass")
		mockClient.AssertExpectations(t)
	})
}

// mockTestError is a simple error type for testing
type mockTestError string

func (e mockTestError) Error() string {
	return string(e)
}
