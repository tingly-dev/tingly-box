package tbclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/tingly-dev/tingly-box/internal/loadbalance"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// HTTPTBClient implements TBClient via HTTP calls to the tingly-box management
// API. Use this when the consumer runs in a separate process from the server.
type HTTPTBClient struct {
	baseURL    string // e.g. "http://localhost:12580"
	userToken  string // management API bearer token
	modelToken string // gateway-route API key
	httpClient *http.Client
}

// NewHTTPTBClient creates an HTTP-based TBClient. baseURL is the server root
// (scheme + host + port, no trailing slash). userToken authenticates management
// API calls; modelToken is the gateway API key returned in env vars.
func NewHTTPTBClient(baseURL, userToken, modelToken string) *HTTPTBClient {
	return &HTTPTBClient{
		baseURL:    strings.TrimRight(baseURL, "/"),
		userToken:  userToken,
		modelToken: modelToken,
		httpClient: http.DefaultClient,
	}
}

func (c *HTTPTBClient) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	var resp struct {
		Success bool     `json:"success"`
		Env     []string `json:"env"`
		Error   string   `json:"error"`
	}
	if err := c.getJSON(ctx, "/api/v1/config/claude/env", &resp); err != nil {
		return nil, fmt.Errorf("get claude code env: %w", err)
	}
	if !resp.Success {
		return nil, fmt.Errorf("get claude code env: server returned failure: %s", resp.Error)
	}
	return resp.Env, nil
}

func (c *HTTPTBClient) GetClaudeCodeSettingsPathForProfile(ctx context.Context, profileID string) (string, error) {
	if profileID == "" {
		return "", nil
	}
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			SettingsPath   string `json:"settingsPath"`
			SettingsExists bool   `json:"settingsExists"`
		} `json:"data"`
		Error string `json:"error"`
	}
	path := fmt.Sprintf("/api/v1/scenario/claude_code/profiles/%s/claude-config", profileID)
	if err := c.getJSON(ctx, path, &resp); err != nil {
		return "", fmt.Errorf("get profile claude config: %w", err)
	}
	if !resp.Success {
		return "", fmt.Errorf("get profile claude config: %s", resp.Error)
	}
	if !resp.Data.SettingsExists {
		return "", fmt.Errorf("claude code profile %q settings not materialized", profileID)
	}
	return resp.Data.SettingsPath, nil
}

func (c *HTTPTBClient) GetHTTPEndpointForScenario(_ context.Context, scenario typ.RuleScenario) (*HTTPEndpointConfig, error) {
	scenarioPath := GetScenarioEndpointPath(scenario)
	return &HTTPEndpointConfig{
		BaseURL: c.baseURL + scenarioPath,
		APIKey:  c.modelToken,
	}, nil
}

func (c *HTTPTBClient) EnsureSmartGuideRuleForBot(ctx context.Context, botUUID, botName, providerUUID, modelID string) error {
	ruleUUID := SmartGuideRuleUUID(botUUID)

	displayName := botName
	if displayName == "" {
		displayName = botUUID
	}

	rule := typ.Rule{
		UUID:          ruleUUID,
		Scenario:      typ.ScenarioSmartGuide,
		RequestModel:  botUUID,
		ResponseModel: modelID,
		Description:   fmt.Sprintf("Auto-generated rule for SmartGuide bot '%s' (DO NOT EDIT)", displayName),
		Services: []*loadbalance.Service{
			{Provider: providerUUID, Model: modelID, Active: true},
		},
		LBTactic: typ.Tactic{
			Type:   loadbalance.TacticRandom,
			Params: typ.DefaultRandomParams(),
		},
		Active:       true,
		SmartEnabled: false,
	}

	body, err := json.Marshal(rule)
	if err != nil {
		return fmt.Errorf("marshal rule: %w", err)
	}

	path := fmt.Sprintf("/api/v1/rule/%s", ruleUUID)
	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := c.postJSON(ctx, path, body, &resp); err != nil {
		return fmt.Errorf("ensure smart guide rule: %w", err)
	}
	if !resp.Success {
		msg := resp.Message
		if resp.Error != "" {
			msg = resp.Error
		}
		return fmt.Errorf("ensure smart guide rule: %s", msg)
	}
	return nil
}

func (c *HTTPTBClient) DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error {
	ruleUUID := SmartGuideRuleUUID(botUUID)
	path := fmt.Sprintf("/api/v1/rule/%s", ruleUUID)

	var resp struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := c.deleteJSON(ctx, path, &resp); err != nil {
		return fmt.Errorf("delete smart guide rule: %w", err)
	}
	if !resp.Success {
		msg := resp.Message
		if resp.Error != "" {
			msg = resp.Error
		}
		return fmt.Errorf("delete smart guide rule: %s", msg)
	}
	return nil
}

func (c *HTTPTBClient) GetDataDir() string {
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ConfigDir string `json:"config_dir"`
		} `json:"data"`
	}
	if err := c.getJSON(context.Background(), "/api/v1/info/config", &resp); err != nil || !resp.Success {
		return filepath.Join(".", "data")
	}
	if resp.Data.ConfigDir == "" {
		return filepath.Join(".", "data")
	}
	return filepath.Join(resp.Data.ConfigDir, "data")
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

func (c *HTTPTBClient) doRequest(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}
	if c.userToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.userToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return c.httpClient.Do(req)
}

func (c *HTTPTBClient) getJSON(ctx context.Context, path string, out any) error {
	resp, err := c.doRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTPTBClient) postJSON(ctx context.Context, path string, body []byte, out any) error {
	resp, err := c.doRequest(ctx, http.MethodPost, path, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *HTTPTBClient) deleteJSON(ctx context.Context, path string, out any) error {
	resp, err := c.doRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(b))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
