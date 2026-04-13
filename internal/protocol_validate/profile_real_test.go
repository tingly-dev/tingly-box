package protocol_validate_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	pt "github.com/tingly-dev/tingly-box/internal/protocol_validate"
)

// TestSetupRealProfile_ClaudeCode verifies that SetupRealProfile correctly wires
// the gateway to an upstream provider. We use a virtual server as the "real"
// upstream so no actual API keys are needed.
//
// Flow:
//   NewProfileTestEnv  →  gateway + virtual server
//   SetupRealProfile   →  provider points at virtual server URL (acting as real)
//   HTTP POST          →  gateway /tingly/claude_code/v1/messages?beta=true
//   Assert             →  200, Anthropic message response with content
func TestSetupRealProfile_ClaudeCode(t *testing.T) {
	env, err := pt.NewProfileTestEnv(pt.ProfileTypeClaudeCode)
	require.NoError(t, err)
	defer env.Close(false)

	// Point the "real" provider at the virtual server — simulates a real provider
	// without needing actual credentials.
	err = env.SetupRealProfile(
		pt.ProfileTypeClaudeCode,
		"virtual-as-real",
		"claude-3-5-sonnet-20241022",
		env.VirtualServerURL(),
		"test-real-key",
		"anthropic",
	)
	require.NoError(t, err)

	// Build a minimal Anthropic v1 messages request (same format claude CLI sends)
	body := map[string]interface{}{
		"model":      "tingly/cc", // must match built-in-cc RequestModel
		"max_tokens": 256,
		"messages": []map[string]interface{}{
			{"role": "user", "content": "Hello"},
		},
		"stream": false,
	}
	bodyBytes, err := json.Marshal(body)
	require.NoError(t, err)

	url := env.BaseURL() + "/tingly/claude_code/v1/messages?beta=true"
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", env.ModelToken())

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	require.Equal(t, http.StatusOK, resp.StatusCode, "unexpected status: %s", string(respBody))

	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(respBody, &parsed), "response must be valid JSON: %s", string(respBody))

	assert.Equal(t, "message", parsed["type"], "response type should be message")
	content, ok := parsed["content"].([]interface{})
	require.True(t, ok && len(content) > 0, "response must have content blocks")
}

// TestSetupRealProfile_LoadConfig verifies YAML config loading and api_style
// auto-detection without starting any servers.
func TestSetupRealProfile_LoadConfig(t *testing.T) {
	t.Run("anthropic auto-detected from baseurl", func(t *testing.T) {
		entry := pt.RealModelEntry{
			Name:    "test",
			BaseURL: "https://api.anthropic.com",
			APIKey:  "sk-ant-xxx",
			Model:   "claude-3-5-sonnet-20241022",
		}
		assert.Equal(t, "anthropic", pt.ResolveAPIStyle(entry))
	})

	t.Run("openai auto-detected from baseurl", func(t *testing.T) {
		entry := pt.RealModelEntry{
			Name:    "test",
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "sk-xxx",
			Model:   "gpt-4o",
		}
		assert.Equal(t, "openai", pt.ResolveAPIStyle(entry))
	})

	t.Run("explicit api_style overrides detection", func(t *testing.T) {
		entry := pt.RealModelEntry{
			Name:     "test",
			BaseURL:  "https://api.openai.com/v1",
			APIKey:   "sk-xxx",
			Model:    "gpt-4o",
			APIStyle: "anthropic",
		}
		assert.Equal(t, "anthropic", pt.ResolveAPIStyle(entry))
	})
}

// TestLoadRealModelsConfig_YAML verifies YAML config loading.
func TestLoadRealModelsConfig_YAML(t *testing.T) {
	content := `
models:
  - name: provider-a
    baseurl: https://api.anthropic.com
    apikey: sk-ant-aaa
    model: claude-3-5-sonnet-20241022
  - name: provider-b
    baseurl: https://api.openai.com/v1
    apikey: sk-bbb
    model: gpt-4o
    api_style: openai
`
	f := writeTempFile(t, "models-*.yaml", content)
	cfg, err := pt.LoadRealModelsConfig(f)
	require.NoError(t, err)
	require.Len(t, cfg.Models, 2)
	assert.Equal(t, "provider-a", cfg.Models[0].Name)
	assert.Equal(t, "claude-3-5-sonnet-20241022", cfg.Models[0].Model)
	assert.Equal(t, "openai", cfg.Models[1].APIStyle)
}

// TestLoadRealModelsConfig_CSV verifies CSV config loading.
func TestLoadRealModelsConfig_CSV(t *testing.T) {
	t.Run("with api_style column", func(t *testing.T) {
		content := "name,baseurl,apikey,model,api_style\n" +
			"provider-a,https://api.anthropic.com,sk-ant-aaa,claude-3-5-sonnet-20241022,anthropic\n" +
			"provider-b,https://api.openai.com/v1,sk-bbb,gpt-4o,openai\n"
		f := writeTempFile(t, "models-*.csv", content)
		cfg, err := pt.LoadRealModelsConfig(f)
		require.NoError(t, err)
		require.Len(t, cfg.Models, 2)
		assert.Equal(t, "provider-a", cfg.Models[0].Name)
		assert.Equal(t, "anthropic", cfg.Models[0].APIStyle)
		assert.Equal(t, "sk-bbb", cfg.Models[1].APIKey)
	})

	t.Run("without api_style column", func(t *testing.T) {
		content := "name,baseurl,apikey,model\n" +
			"provider-a,https://api.anthropic.com,sk-ant-aaa,claude-3-5-sonnet-20241022\n"
		f := writeTempFile(t, "models-*.csv", content)
		cfg, err := pt.LoadRealModelsConfig(f)
		require.NoError(t, err)
		require.Len(t, cfg.Models, 1)
		assert.Equal(t, "", cfg.Models[0].APIStyle)
		assert.Equal(t, "anthropic", pt.ResolveAPIStyle(cfg.Models[0]))
	})

	t.Run("missing required column", func(t *testing.T) {
		content := "name,baseurl,apikey\nprovider-a,https://api.anthropic.com,sk-ant-aaa\n"
		f := writeTempFile(t, "models-*.csv", content)
		_, err := pt.LoadRealModelsConfig(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `missing required column "model"`)
	})

	t.Run("empty required field in row", func(t *testing.T) {
		content := "name,baseurl,apikey,model\nprovider-a,,sk-ant-aaa,claude-3\n"
		f := writeTempFile(t, "models-*.csv", content)
		_, err := pt.LoadRealModelsConfig(f)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "missing required field")
	})

	t.Run("column order independent", func(t *testing.T) {
		content := "model,apikey,name,baseurl\n" +
			"gpt-4o,sk-xxx,my-openai,https://api.openai.com/v1\n"
		f := writeTempFile(t, "models-*.csv", content)
		cfg, err := pt.LoadRealModelsConfig(f)
		require.NoError(t, err)
		require.Len(t, cfg.Models, 1)
		assert.Equal(t, "my-openai", cfg.Models[0].Name)
		assert.Equal(t, "gpt-4o", cfg.Models[0].Model)
	})
}

func writeTempFile(t *testing.T, pattern, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", pattern)
	require.NoError(t, err)
	t.Cleanup(func() { os.Remove(f.Name()) })
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
