package tbclient

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/tingly-dev/tingly-box/internal/config"
)

func TestNewTBClient(t *testing.T) {
	cfg := &config.AppConfig{}

	// We can't easily mock the dependencies without interfaces,
	// so we'll test with nil values and check the constructor works
	client := NewTBClient(cfg, nil, nil, "localhost", 8080)

	assert.NotNil(t, client)
	assert.Equal(t, cfg, client.config)
	assert.Equal(t, "localhost", client.serverHost)
	assert.Equal(t, 8080, client.serverPort)
	assert.Equal(t, "http://localhost:12580/tingly/claude_code", client.defaultBaseURL)
}

func TestTBClient_DefaultBaseURL(t *testing.T) {
	cfg := &config.AppConfig{}
	client := NewTBClient(cfg, nil, nil, "localhost", 8080)

	assert.Equal(t, "http://localhost:12580/tingly/claude_code", client.defaultBaseURL)
}

func TestTBClient_ServerConfig(t *testing.T) {
	cfg := &config.AppConfig{}
	client := NewTBClient(cfg, nil, nil, "example.com", 9090)

	assert.Equal(t, "example.com", client.serverHost)
	assert.Equal(t, 9090, client.serverPort)
}

func TestProviderInfo_Structure(t *testing.T) {
	info := ProviderInfo{
		UUID:     "test-uuid",
		Name:     "test-provider",
		APIBase:  "https://api.test.com",
		APIStyle: "anthropic",
		Enabled:  true,
		Models:   []string{"model-1", "model-2"},
	}

	assert.Equal(t, "test-uuid", info.UUID)
	assert.Equal(t, "test-provider", info.Name)
	assert.Equal(t, "https://api.test.com", info.APIBase)
	assert.Equal(t, "anthropic", info.APIStyle)
	assert.True(t, info.Enabled)
	assert.Equal(t, []string{"model-1", "model-2"}, info.Models)
}

func TestServiceInfo_Structure(t *testing.T) {
	info := ServiceInfo{
		ProviderID: "prov-1",
		Model:      "model-1",
	}

	assert.Equal(t, "prov-1", info.ProviderID)
	assert.Equal(t, "model-1", info.Model)
}

func TestModelSelectionRequest_Structure(t *testing.T) {
	req := ModelSelectionRequest{
		ProviderUUID: "prov-1",
		ServiceName:  "test-service",
		ModelID:      "model-1",
	}

	assert.Equal(t, "prov-1", req.ProviderUUID)
	assert.Equal(t, "test-service", req.ServiceName)
	assert.Equal(t, "model-1", req.ModelID)
}

func TestModelConfig_Structure(t *testing.T) {
	config := ModelConfig{
		ProviderUUID: "prov-1",
		ModelID:      "model-1",
		BaseURL:      "https://api.test.com",
		APIKey:       "test-key",
		APIStyle:     "anthropic",
	}

	assert.Equal(t, "prov-1", config.ProviderUUID)
	assert.Equal(t, "model-1", config.ModelID)
	assert.Equal(t, "https://api.test.com", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "anthropic", config.APIStyle)
}

func TestConnectionConfig_Structure(t *testing.T) {
	config := ConnectionConfig{
		BaseURL: "http://localhost:12580/tingly/claude_code",
		APIKey:  "test-key",
	}

	assert.Equal(t, "http://localhost:12580/tingly/claude_code", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
}

func TestDefaultServiceConfig_Structure(t *testing.T) {
	config := DefaultServiceConfig{
		ProviderUUID: "prov-1",
		ProviderName: "Test Provider",
		ModelID:      "model-1",
		BaseURL:      "http://localhost:12580/tingly/claude_code",
		APIKey:       "test-key",
		APIStyle:     "anthropic",
	}

	assert.Equal(t, "prov-1", config.ProviderUUID)
	assert.Equal(t, "Test Provider", config.ProviderName)
	assert.Equal(t, "model-1", config.ModelID)
	assert.Equal(t, "http://localhost:12580/tingly/claude_code", config.BaseURL)
	assert.Equal(t, "test-key", config.APIKey)
	assert.Equal(t, "anthropic", config.APIStyle)
}

func TestTBClient_Types(t *testing.T) {
	// Test that TBClientImpl implements TBClient interface
	var _ TBClient = (*TBClientImpl)(nil)
}
