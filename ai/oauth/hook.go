package oauth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Anthropic token response types
// These types match the structure of Anthropic's OAuth token response

// AnthropicTokenResponse represents the full token response from Anthropic OAuth
type AnthropicTokenResponse struct {
	AccessToken  string                `json:"access_token"`
	RefreshToken string                `json:"refresh_token"`
	TokenType    string                `json:"token_type"`
	ExpiresIn    int                   `json:"expires_in"`
	Organization AnthropicOrganization `json:"organization"`
	Account      AnthropicAccount      `json:"account"`
}

// AnthropicOrganization represents organization information in token response
type AnthropicOrganization struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

// AnthropicAccount represents account information in token response
type AnthropicAccount struct {
	UUID         string `json:"uuid"`
	EmailAddress string `json:"email_address"`
}

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
	type accountResponse struct {
		UUID         string `json:"uuid"`
		EmailAddress string `json:"email_address"`
	}

	// Try to get account info from Anthropic account endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.anthropic.com/v1/account", nil)
	if err != nil {
		return nil, nil
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

	var account accountResponse
	if err := json.NewDecoder(resp.Body).Decode(&account); err != nil {
		return nil, nil
	}

	metadata := make(map[string]any)
	if account.EmailAddress != "" {
		metadata["email"] = account.EmailAddress
	}
	if account.UUID != "" {
		metadata["account_id"] = account.UUID
	}

	return metadata, nil
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
	metadata := make(map[string]any)

	// Fetch user email from Google userinfo endpoint
	type userInfo struct {
		Email string `json:"email"`
	}

	req, err := http.NewRequestWithContext(ctx, "GET", "https://www.googleapis.com/oauth2/v1/userinfo?alt=json", nil)
	if err == nil {
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
	}

	// Discover Code Assist project ID via loadCodeAssist + onboardUser.
	// Gemini CLI calls https://cloudcode-pa.googleapis.com/v1internal:* to obtain
	// the cloudaicompanionProject required by every subsequent generateContent
	// request. Without it the Code Assist API rejects the call.
	if projectID, err := fetchGeminiProjectID(ctx, accessToken, httpClient); err == nil && projectID != "" {
		metadata["project_id"] = projectID
	}

	return metadata, nil
}

// Gemini Code Assist API constants (shared with Antigravity host).
const (
	geminiCodeAssistEndpoint = "https://cloudcode-pa.googleapis.com"
	geminiCodeAssistVersion  = "v1internal"
	geminiCodeAssistUA       = "GeminiCLI/0.1.0 (linux; amd64)"
)

// fetchGeminiProjectID resolves the cloudaicompanionProject for the authenticated
// user. It first calls loadCodeAssist; if the current tier is missing or the
// response does not include a project ID, it falls back to onboardUser to create
// one and waits for the long-running operation to finish.
func fetchGeminiProjectID(ctx context.Context, accessToken string, httpClient *http.Client) (string, error) {
	loadResp, err := geminiCodeAssistCall(ctx, accessToken, httpClient, "loadCodeAssist", map[string]any{
		"metadata": map[string]string{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	})
	if err != nil {
		return "", err
	}

	if id := extractGeminiProjectID(loadResp); id != "" {
		return id, nil
	}

	tierID := "free-tier"
	if tiers, ok := loadResp["allowedTiers"].([]any); ok {
		for _, t := range tiers {
			tier, ok := t.(map[string]any)
			if !ok {
				continue
			}
			if def, _ := tier["isDefault"].(bool); def {
				if id, _ := tier["id"].(string); id != "" {
					tierID = id
				}
				break
			}
		}
	}

	onboardBody := map[string]any{
		"tierId": tierID,
		"metadata": map[string]string{
			"ideType":    "IDE_UNSPECIFIED",
			"platform":   "PLATFORM_UNSPECIFIED",
			"pluginType": "GEMINI",
		},
	}

	for attempt := 0; attempt < 6; attempt++ {
		lroResp, err := geminiCodeAssistCall(ctx, accessToken, httpClient, "onboardUser", onboardBody)
		if err != nil {
			return "", err
		}
		if done, _ := lroResp["done"].(bool); done {
			if response, ok := lroResp["response"].(map[string]any); ok {
				if id := extractGeminiProjectID(response); id != "" {
					return id, nil
				}
			}
			return "", fmt.Errorf("onboardUser completed without project id")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
	return "", fmt.Errorf("onboardUser did not complete in time")
}

func geminiCodeAssistCall(ctx context.Context, accessToken string, httpClient *http.Client, method string, body map[string]any) (map[string]any, error) {
	rawBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal %s body: %w", method, err)
	}

	endpoint := fmt.Sprintf("%s/%s:%s", geminiCodeAssistEndpoint, geminiCodeAssistVersion, method)
	req, err := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(string(rawBody)))
	if err != nil {
		return nil, fmt.Errorf("build %s request: %w", method, err)
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", geminiCodeAssistUA)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute %s: %w", method, err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read %s response: %w", method, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("%s failed with status %d: %s", method, resp.StatusCode, string(respBody))
	}

	parsed := make(map[string]any)
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", method, err)
	}
	return parsed, nil
}

// extractGeminiProjectID pulls cloudaicompanionProject from a loadCodeAssist or
// onboardUser response. It accepts the field as either a raw string or a nested
// object with an "id" key (gemini-cli observes both shapes in the wild).
func extractGeminiProjectID(resp map[string]any) string {
	if id, ok := resp["cloudaicompanionProject"].(string); ok {
		return strings.TrimSpace(id)
	}
	if projectMap, ok := resp["cloudaicompanionProject"].(map[string]any); ok {
		if id, okID := projectMap["id"].(string); okID {
			return strings.TrimSpace(id)
		}
	}
	return ""
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

// KimiHook implements Kimi OAuth device-code flow specific behavior.
// Reference: https://github.com/router-for-me/CLIProxyAPI internal/auth/kimi/kimi.go
//
// X-Msh-Device-Id is NOT set here — it's per-flow state injected by the
// caller via oauth.WithExtraHeader, so refresh and inference can reuse it.
type KimiHook struct{}

func (h *KimiHook) BeforeAuth(params map[string]string) error {
	return nil
}

func (h *KimiHook) BeforeToken(body map[string]string, header http.Header) error {
	header.Set("X-Msh-Platform", "kimi_cli")
	header.Set("X-Msh-Version", "1.10.6")
	header.Set("X-Msh-Device-Name", KimiDeviceName())
	header.Set("X-Msh-Device-Model", KimiDeviceModel())
	header.Set("X-Msh-Os-Version", KimiOsVersion())
	return nil
}

func (h *KimiHook) AfterToken(ctx context.Context, accessToken string, httpClient *http.Client) (map[string]any, error) {
	// Kimi has no public userinfo endpoint; CLIProxyAPI hardcodes the display label.
	return nil, nil
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

// KimiDeviceName returns the hostname for Kimi headers (auth and inference).
func KimiDeviceName() string {
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}
	return "unknown"
}

// KimiDeviceModel returns the device model in the "<OS> <GOARCH>" format
// CLIProxyAPI uses for Kimi headers (e.g. "macOS arm64").
func KimiDeviceModel() string {
	goos := runtime.GOOS
	switch goos {
	case "darwin":
		goos = "macOS"
	case "linux":
		goos = "Linux"
	case "windows":
		goos = "Windows"
	}
	return goos + " " + runtime.GOARCH
}

// KimiOsVersion returns a representative OS version string for the X-Msh-Os-Version header.
// Values mirror typical platform.version() output from kimi-cli:
//   - Linux   → Ubuntu 22.04 LTS default kernel
//   - macOS   → macOS Sonoma 14.6.1
//   - Windows → Windows 11 23H2
func KimiOsVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return "14.6.1"
	case "windows":
		return "10.0.22631"
	default: // linux and others
		return "6.8.0"
	}
}
