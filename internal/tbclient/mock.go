package tbclient

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// MockTBClient is a mock implementation of TBClient interface for testing
// This mock can be shared across all packages that need to test TBClient functionality
type MockTBClient struct {
	mock.Mock
}

// GetProviders mocks the GetProviders method
func (m *MockTBClient) GetProviders(ctx context.Context) ([]ProviderInfo, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]ProviderInfo), args.Error(1)
}

// GetDefaultRule mocks the GetDefaultRule method
func (m *MockTBClient) GetDefaultRule(ctx context.Context) (*typ.Rule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*typ.Rule), args.Error(1)
}

// GetDefaultService mocks the GetDefaultService method
func (m *MockTBClient) GetDefaultService(ctx context.Context) (*DefaultServiceConfig, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*DefaultServiceConfig), args.Error(1)
}

// GetConnectionConfig mocks the GetConnectionConfig method
func (m *MockTBClient) GetConnectionConfig(ctx context.Context) (*ConnectionConfig, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ConnectionConfig), args.Error(1)
}

// GetDefaultRuleForScenario mocks the GetDefaultRuleForScenario method
func (m *MockTBClient) GetDefaultRuleForScenario(ctx context.Context, scenario typ.RuleScenario) (*typ.Rule, error) {
	args := m.Called(ctx, scenario)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*typ.Rule), args.Error(1)
}

// GetHTTPEndpointForScenario mocks the GetHTTPEndpointForScenario method
func (m *MockTBClient) GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error) {
	args := m.Called(ctx, scenario)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*HTTPEndpointConfig), args.Error(1)
}

// EnsureSmartGuideRule mocks the EnsureSmartGuideRule method
// Deprecated: Use EnsureSmartGuideRuleForBot for bot-specific rules
func (m *MockTBClient) EnsureSmartGuideRule(ctx context.Context, providerUUID, modelID string) error {
	args := m.Called(ctx, providerUUID, modelID)
	return args.Error(0)
}

// EnsureSmartGuideRuleForBot mocks the EnsureSmartGuideRuleForBot method
func (m *MockTBClient) EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error {
	args := m.Called(ctx, botUUID, botName, providerUUID, modelID)
	return args.Error(0)
}

// SelectModel mocks the SelectModel method
func (m *MockTBClient) SelectModel(ctx context.Context, req ModelSelectionRequest) (*ModelConfig, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ModelConfig), args.Error(1)
}

// GetDataDir mocks the GetDataDir method
func (m *MockTBClient) GetDataDir() string {
	args := m.Called()
	return args.String(0)
}
