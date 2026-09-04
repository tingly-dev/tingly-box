package tbclient

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"

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
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// apiResponse is the common envelope for management API responses that carry
// only success/error/message (rule CRUD, etc.).
type apiResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Error   string `json:"error"`
}

func (r apiResponse) err(prefix string) error {
	if r.Success {
		return nil
	}
	return fmt.Errorf("%s: %s", prefix, cmp.Or(r.Error, r.Message, "unknown error"))
}

func (c *HTTPTBClient) GetClaudeCodeEnv(ctx context.Context) ([]string, error) {
	var resp struct {
		Success bool     `json:"success"`
		Env     []string `json:"env"`
		Error   string   `json:"error"`
	}
	if err := c.doJSON(ctx, http.MethodGet, "/api/v1/config/claude/env", nil, &resp); err != nil {
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
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
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
	ruleUUID := serverconfig.SmartGuideRuleUUID(botUUID)

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
	var resp apiResponse
	if err := c.doJSON(ctx, http.MethodPost, path, body, &resp); err != nil {
		return fmt.Errorf("ensure smart guide rule: %w", err)
	}
	return resp.err("ensure smart guide rule")
}

func (c *HTTPTBClient) DeleteSmartGuideRuleForBot(ctx context.Context, botUUID string) error {
	ruleUUID := serverconfig.SmartGuideRuleUUID(botUUID)
	path := fmt.Sprintf("/api/v1/rule/%s", ruleUUID)

	var resp apiResponse
	if err := c.doJSON(ctx, http.MethodDelete, path, nil, &resp); err != nil {
		return fmt.Errorf("delete smart guide rule: %w", err)
	}
	return resp.err("delete smart guide rule")
}

func (c *HTTPTBClient) GetDataDir() string {
	var resp struct {
		Success bool `json:"success"`
		Data    struct {
			ConfigDir string `json:"config_dir"`
		} `json:"data"`
	}
	if err := c.doJSON(context.Background(), http.MethodGet, "/api/v1/info/config", nil, &resp); err != nil || !resp.Success {
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

// doJSON executes an HTTP request and JSON-decodes the response into out.
// Pass nil body for bodyless requests (GET, DELETE).
func (c *HTTPTBClient) doJSON(ctx context.Context, method, path string, body []byte, out any) error {
	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}

	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return err
	}
	if c.userToken != "" {
		req.Header.Set("Authorization", "Bearer "+c.userToken)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
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
