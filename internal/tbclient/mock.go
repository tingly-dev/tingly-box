package tbclient

import (
	"context"

	"github.com/stretchr/testify/mock"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// MockTBClient is a mock implementation of TBClient interface for testing.
type MockTBClient struct {
	mock.Mock
}

func (m *MockTBClient) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockTBClient) GetClaudeCodeSettingsPathForProfile(ctx context.Context, profileID string) (string, error) {
	args := m.Called(ctx, profileID)
	return args.String(0), args.Error(1)
}

func (m *MockTBClient) GetHTTPEndpointForScenario(ctx context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error) {
	args := m.Called(ctx, scenario)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*HTTPEndpointConfig), args.Error(1)
}

func (m *MockTBClient) EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error {
	args := m.Called(ctx, botUUID, botName, providerUUID, modelID)
	return args.Error(0)
}

func (m *MockTBClient) DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error {
	args := m.Called(ctx, botUUID)
	return args.Error(0)
}

func (m *MockTBClient) GetDataDir() string {
	args := m.Called()
	return args.String(0)
}
