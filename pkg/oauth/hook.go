package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
)

// RequestHook defines preprocessing and postprocessing hooks for OAuth requests.
// Implementations can modify request parameters before they are sent and fetch additional metadata after token is obtained.
type RequestHook interface {
	// BeforeAuth is called before building the authorization URL.
	// The params map contains URL query parameters that can be modified or extended.
	BeforeAuth(params map[string]string) error

	// BeforeToken is called before sending any token-related HTTP request.
	// This covers: token exchange, refresh token, device code request, and device token polling.
	// The body map contains request body parameters, header is the HTTP headers.
	BeforeToken(body map[string]string, header http.Header) error

	// AfterToken is called after successful token exchange to fetch additional metadata.
	// Returns additional metadata to be stored with the token (email, project_id, api_key, etc).
	// Can return nil map if no additional metadata is needed.
	AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error)
}

// NoopHook is a default hook that does nothing.
// Used when no custom behavior is needed.
type NoopHook struct{}

func (h *NoopHook) BeforeAuth(params map[string]string) error {
	return nil
}

func (h *NoopHook) BeforeToken(body map[string]string, header http.Header) error {
	return nil
}

func (h *NoopHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	return nil, nil
}

// AnthropicHook implements Anthropic Claude Code OAuth specific behavior.
type AnthropicHook struct{}

func (h *AnthropicHook) BeforeAuth(params map[string]string) error {
	params["code"] = "true"
	params["response_type"] = "code"
	return nil
}

func (h *AnthropicHook) BeforeToken(body map[string]string, header http.Header) error {
	header.Set("Content-Type", "application/json")
	header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36")
	header.Set("Accept", "application/json, text/plain, */*")
	header.Set("Accept-Language", "en-US,en;q=0.9")
	header.Set("Referer", "https://claude.ai/")
	header.Set("Origin", "https://claude.ai")
	return nil
}

func (h *AnthropicHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	return nil, nil
}

// GeminiHook implements Gemini CLI OAuth specific behavior.
type GeminiHook struct{}

func (h *GeminiHook) BeforeAuth(params map[string]string) error {
	params["access_type"] = "offline"
	params["prompt"] = "consent"
	return nil
}

func (h *GeminiHook) BeforeToken(body map[string]string, header http.Header) error {
	return nil
}

func (h *GeminiHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	// Fetch user email from Google userinfo endpoint
	type userInfo struct {
		Email string `json:"email"`
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil
	}

	var info userInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, err
	}

	metadata := make(map[string]any)
	if info.Email != "" {
		metadata["email"] = info.Email
	}
	return metadata, nil
}

// AntigravityHook implements Antigravity OAuth specific behavior.
type AntigravityHook struct{}

func (h *AntigravityHook) BeforeAuth(params map[string]string) error {
	params["access_type"] = "offline"
	params["prompt"] = "consent"
	params["include_granted_scopes"] = "true"
	return nil
}

func (h *AntigravityHook) BeforeToken(body map[string]string, header http.Header) error {
	return nil
}

func (h *AntigravityHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	metadata := make(map[string]any)

	// Fetch user email
	type userInfo struct {
		Email string `json:"email"`
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	if err != nil {
		return metadata, nil
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := httpClient.Do(req)
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusOK && resp.StatusCode < http.StatusMultipleChoices {
			var info userInfo
			if json.NewDecoder(resp.Body).Decode(&info) == nil && info.Email != "" {
				metadata["email"] = info.Email
			}
		}
	}

	// Fetch project ID via loadCodeAssist
	projectID, err := fetchAntigravityProjectID(ctx, accessToken, httpClient)
	if err == nil && projectID != "" {
		metadata["project_id"] = projectID
	}

	return metadata, nil
}

// Antigravity API constants for project discovery
const (
	antigravityAPIEndpoint  = "https://cloudcode-pa.googleapis.com"
	antigravityAPIVersion   = "v1internal"
	antigravityAPIUserAgent = "antigravity/1.11.9 windows/amd64"
)

// fetchAntigravityProjectID retrieves the project ID for the authenticated user via loadCodeAssist.
func fetchAntigravityProjectID(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	loadReqBody := map[string]any{
		"metadata": map[string]string{
			"ideType": "ANTIGRAVITY",
		},
	}

	rawBody, err := json.Marshal(loadReqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request body: %w", err)
	}

	endpointURL := fmt.Sprintf("%s/%s:loadCodeAssist", antigravityAPIEndpoint, antigravityAPIVersion)
	req, err := http.NewRequestWithContext(ctx, "POST", endpointURL, strings.NewReader(string(rawBody)))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", antigravityAPIUserAgent)
	req.Header.Set("Host", "cloudcode-pa.googleapis.com")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("execute request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var loadResp map[string]any
	if err := json.Unmarshal(bodyBytes, &loadResp); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}

	// Extract projectID from response
	projectID := ""
	if id, ok := loadResp["cloudaicompanionProject"].(string); ok {
		projectID = strings.TrimSpace(id)
	}
	if projectID == "" {
		if projectMap, ok := loadResp["cloudaicompanionProject"].(map[string]any); ok {
			if id, okID := projectMap["id"].(string); okID {
				projectID = strings.TrimSpace(id)
			}
		}
	}

	if projectID == "" {
		return "", fmt.Errorf("no cloudaicompanionProject in response")
	}

	return projectID, nil
}

// QwenHook implements Qwen Device Code OAuth specific behavior.
type QwenHook struct{}

func (h *QwenHook) BeforeAuth(params map[string]string) error {
	return nil
}

func (h *QwenHook) BeforeToken(body map[string]string, header http.Header) error {
	header.Set("x-request-id", uuid.New().String())
	return nil
}

func (h *QwenHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	return nil, nil
}

// IFlowHook implements iFlow OAuth specific behavior.
type IFlowHook struct {
	ClientID     string
	ClientSecret string
}

func (h *IFlowHook) BeforeAuth(params map[string]string) error {
	params["loginMethod"] = "phone"
	params["type"] = "phone"
	return nil
}

func (h *IFlowHook) BeforeToken(body map[string]string, header http.Header) error {
	// Set Basic Auth header
	basic := base64.StdEncoding.EncodeToString([]byte(h.ClientID + ":" + h.ClientSecret))
	header.Set("Authorization", "Basic "+basic)
	header.Set("Accept", "application/json")
	return nil
}

func (h *IFlowHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	// Fetch user info and API key from iFlow
	type userInfoResponse struct {
		Success bool `json:"success"`
		Data    struct {
			APIKey string `json:"apiKey"`
			Email  string `json:"email"`
			Phone  string `json:"phone"`
		} `json:"data"`
	}

	endpoint := fmt.Sprintf("https://iflow.cn/api/oauth/getUserInfo?accessToken=%s", accessToken)
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("iflow user info: status %d: %s", resp.StatusCode, string(body))
	}

	var result userInfoResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, err
	}

	if !result.Success {
		return nil, fmt.Errorf("iflow user info: request not successful")
	}

	metadata := make(map[string]any)
	if result.Data.APIKey != "" {
		metadata["api_key"] = result.Data.APIKey
	}
	if result.Data.Email != "" {
		metadata["email"] = result.Data.Email
	} else if result.Data.Phone != "" {
		metadata["email"] = result.Data.Phone
	}
	return metadata, nil
}

// CodexHook implements Codex (OpenAI) OAuth specific behavior.
type CodexHook struct{}

func (h *CodexHook) BeforeAuth(params map[string]string) error {
	// Emulate OpenAI Codex CLI by adding the exact parameters it uses
	params["id_token_add_organizations"] = "true"
	params["codex_cli_simplified_flow"] = "true"
	params["originator"] = "codex_cli_rs"
	return nil
}

func (h *CodexHook) BeforeToken(body map[string]string, header http.Header) error {
	header.Set("Content-Type", "application/x-www-form-urlencoded")
	header.Set("Accept", "application/json")
	return nil
}

func (h *CodexHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	// For OpenAI Codex, user information is in the ID token (JWT)
	// Since we only receive the access token here, we'll try to fetch user info
	// from OpenAI's userinfo endpoint if available
	//
	// Note: The ID token parsing for email/account_id should be done
	// at the token handling level since the ID token contains the claims
	//
	// For now, we return nil metadata - the token manager should handle
	// ID token parsing separately

	// Try calling OpenAI userinfo endpoint (may not be publicly available)
	type userInfo struct {
		Email string `json:"email"`
		Name  string `json:"name"`
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.openai.com/v1/user", nil)
	if err != nil {
		return nil, nil // Return nil metadata on error, don't fail the auth flow
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, nil
	}

	var info userInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, nil
	}

	metadata := make(map[string]any)
	if info.Email != "" {
		metadata["email"] = info.Email
	}
	if info.Name != "" {
		metadata["name"] = info.Name
	}
	return metadata, nil
}
