package vmodel

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSharedDefaultMocksIncludesErrorModels(t *testing.T) {
	specs := SharedDefaultMocks()

	// Find error models in SharedDefaultMocks
	errorModels := findErrorModels(specs)

	// Should have 4 basic error models
	require.Len(t, errorModels, 4, "Should have 4 basic error models in SharedDefaultMocks")

	// Test virtual-fail-429 (renamed from virtual-fail-precontent-429)
	spec429 := findSpecByID(errorModels, "virtual-fail-429")
	require.NotNil(t, spec429)
	assert.Equal(t, ErrorCategoryRateLimit, spec429.ErrorCategory)
	assert.True(t, spec429.IsRetryable, "429 should be retryable")
	assert.Equal(t, "medium", spec429.Severity)
	require.NotNil(t, spec429.Error)
	assert.Equal(t, ErrorStagePreContent, spec429.Error.Stage)
	assert.Equal(t, 429, spec429.Error.Status)

	// Test virtual-fail-500 (renamed from virtual-fail-precontent-500)
	spec500 := findSpecByID(errorModels, "virtual-fail-500")
	require.NotNil(t, spec500)
	assert.Equal(t, ErrorCategoryUpstream, spec500.ErrorCategory)
	assert.True(t, spec500.IsRetryable, "500 should be retryable")
	assert.Equal(t, "high", spec500.Severity)

	// Test virtual-fail-midstream-close
	specClose := findSpecByID(errorModels, "virtual-fail-midstream-close")
	require.NotNil(t, specClose)
	assert.Equal(t, ErrorCategoryNetwork, specClose.ErrorCategory)
	assert.False(t, specClose.IsRetryable, "Mid-stream close should NOT be retryable")
	assert.Equal(t, ErrorStageMidStream, specClose.Error.Stage)

	// Test virtual-fail-midstream-event
	specEvent := findSpecByID(errorModels, "virtual-fail-midstream-event")
	require.NotNil(t, specEvent)
	assert.Equal(t, ErrorCategoryUpstream, specEvent.ErrorCategory)
	assert.False(t, specEvent.IsRetryable, "Mid-stream error should NOT be retryable")
}

func TestExtendedErrorSpecs(t *testing.T) {
	specs := ExtendedErrorSpecs()
	require.Len(t, specs, 6, "Should have 6 extended error specs")

	// Test authentication error
	auth401 := findSpec(t, specs, "virtual-fail-auth-401")
	require.NotNil(t, auth401)
	assert.Equal(t, ErrorCategoryAuth, auth401.ErrorCategory)
	assert.False(t, auth401.IsRetryable, "Auth errors should NOT be retryable")
	assert.Equal(t, 401, auth401.Error.Status)

	// Test 502 Bad Gateway
	badGateway := findSpec(t, specs, "virtual-fail-502")
	require.NotNil(t, badGateway)
	assert.Equal(t, ErrorCategoryUpstream, badGateway.ErrorCategory)
	assert.True(t, badGateway.IsRetryable, "502 should be retryable")
	assert.Equal(t, 502, badGateway.Error.Status)

	// Test 503 Service Unavailable
	unavailable := findSpec(t, specs, "virtual-fail-503")
	require.NotNil(t, unavailable)
	assert.Equal(t, ErrorCategoryOverloaded, unavailable.ErrorCategory)
	assert.True(t, unavailable.IsRetryable, "503 should be retryable")

	// Test 529 Anthropic overloaded
	overloaded := findSpec(t, specs, "virtual-fail-529")
	require.NotNil(t, overloaded)
	assert.Equal(t, ErrorCategoryOverloaded, overloaded.ErrorCategory)
	assert.True(t, overloaded.IsRetryable, "529 should be retryable")
	assert.Equal(t, 529, overloaded.Error.Status)

	// Test invalid request
	invalid := findSpec(t, specs, "virtual-fail-400")
	require.NotNil(t, invalid)
	assert.Equal(t, ErrorCategoryInvalid, invalid.ErrorCategory)
	assert.False(t, invalid.IsRetryable, "400 should NOT be retryable")
	assert.Equal(t, "low", invalid.Severity)

	// Test mid-stream timeout
	timeout := findSpec(t, specs, "virtual-fail-timeout")
	require.NotNil(t, timeout)
	assert.Equal(t, ErrorCategoryTimeout, timeout.ErrorCategory)
	assert.False(t, timeout.IsRetryable, "Mid-stream timeout should NOT be retryable")
	assert.Equal(t, ErrorStageMidStream, timeout.Error.Stage)
}

func TestAllErrorSpecs(t *testing.T) {
	// Total error models = 4 (basic in SharedDefaultMocks) + 6 (extended)
	basicErrorModels := findErrorModels(SharedDefaultMocks())
	extendedSpecs := ExtendedErrorSpecs()

	// Verify we have the expected counts
	assert.Len(t, basicErrorModels, 4, "Should have 4 basic error models in SharedDefaultMocks")
	assert.Len(t, extendedSpecs, 6, "Should have 6 extended error specs")

	// Verify basic specs are in SharedDefaultMocks
	assert.Contains(t, specIDs(basicErrorModels), "virtual-fail-429")
	assert.Contains(t, specIDs(basicErrorModels), "virtual-fail-midstream-close")

	// Verify extended specs
	assert.Contains(t, specIDs(extendedSpecs), "virtual-fail-auth-401")
	assert.Contains(t, specIDs(extendedSpecs), "virtual-fail-502")
	assert.Contains(t, specIDs(extendedSpecs), "virtual-fail-timeout")
}

func TestErrorCategoryString(t *testing.T) {
	categories := []ErrorCategory{
		ErrorCategoryRateLimit,
		ErrorCategoryUpstream,
		ErrorCategoryTimeout,
		ErrorCategoryOverloaded,
		ErrorCategoryInvalid,
		ErrorCategoryAuth,
		ErrorCategoryNetwork,
		ErrorCategoryMalformed,
	}

	for _, cat := range categories {
		assert.NotEmpty(t, string(cat), "Category should have string representation")
	}
}

// Helper functions

func findSpecByID(specs []SharedMockSpec, id string) *SharedMockSpec {
	for i := range specs {
		if specs[i].ID == id {
			return &specs[i]
		}
	}
	return nil
}

func findSpec(t *testing.T, specs []SharedMockSpec, id string) *SharedMockSpec {
	t.Helper()
	return findSpecByID(specs, id)
}

func findErrorModels(specs []SharedMockSpec) []SharedMockSpec {
	var errorModels []SharedMockSpec
	for _, spec := range specs {
		if spec.Error != nil {
			errorModels = append(errorModels, spec)
		}
	}
	return errorModels
}

func specIDs(specs []SharedMockSpec) []string {
	ids := make([]string, len(specs))
	for i, spec := range specs {
		ids[i] = spec.ID
	}
	return ids
}
