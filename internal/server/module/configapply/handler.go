package configapply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/agent"
	"github.com/tingly-dev/tingly-box/internal/server/config"
	"github.com/tingly-dev/tingly-box/internal/server/middleware"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// Handler handles config apply HTTP requests
type Handler struct {
	config *config.Config
	host   string
}

// NewHandler creates a new configapply handler
func NewHandler(cfg *config.Config, host string) *Handler {
	return &Handler{
		config: cfg,
		host:   host,
	}
}

// HTTPTransportConfigUpdate represents the update request for HTTP transport settings
type HTTPTransportConfigUpdate struct {
	RespectEnvProxy *bool   `json:"respect_env_proxy"` // nil = no change
	GlobalProxyURL  *string `json:"global_proxy_url"`  // nil = no change; "" = clear
}

// GetConfig returns the current system configuration
// Only returns settings that are safe to expose to the UI
func (h *Handler) GetConfig(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	response := gin.H{
		"success": true,
		"data": gin.H{
			"http_transport": gin.H{
				"respect_env_proxy": cfg.HTTPTransport.RespectEnvProxy,
				"global_proxy_url":  cfg.HTTPTransport.GlobalProxyURL,
			},
		},
	}
	c.JSON(http.StatusOK, response)
}

// UpdateConfig updates the system configuration
// Only allows updating specific safe fields
func (h *Handler) UpdateConfig(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Global config not available",
		})
		return
	}

	var req struct {
		HTTPTransport HTTPTransportConfigUpdate `json:"http_transport"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request body: " + err.Error(),
		})
		return
	}

	// Update respect_env_proxy if provided
	if req.HTTPTransport.RespectEnvProxy != nil {
		cfg.HTTPTransport.RespectEnvProxy = req.HTTPTransport.RespectEnvProxy
	}

	// Update global_proxy_url if provided (pointer allows distinguishing "not sent" from "clear")
	if req.HTTPTransport.GlobalProxyURL != nil {
		cfg.HTTPTransport.GlobalProxyURL = *req.HTTPTransport.GlobalProxyURL
	}

	// Save the configuration
	if err := cfg.Save(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to save configuration: " + err.Error(),
		})
		return
	}

	// Apply the new transport configuration
	cfg.ApplyHTTPTransportConfig()

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"http_transport": gin.H{
				"respect_env_proxy": cfg.HTTPTransport.RespectEnvProxy,
				"global_proxy_url":  cfg.HTTPTransport.GlobalProxyURL,
			},
		},
	})
}

// ApplyClaudeConfig generates and applies Claude Code configuration from system state
func (h *Handler) ApplyClaudeConfig(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	var req ApplyClaudeConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "Invalid request body: " + err.Error(),
		})
		return
	}
	if req.Preferences == nil {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "preferences field is required",
		})
		return
	}

	// Get base URL from the user's request (respects reverse proxy headers)
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := middleware.BaseURLFromRequest(c, port)
	// Use the model token from config (tingly-box- prefixed JWT)
	apiKey := h.config.GetModelToken()

	// Materialize prefs to the env map written into settings.json.
	env, prefsErr := req.Preferences.ToEnv(baseURL, apiKey)
	if prefsErr != nil {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "Invalid preferences: " + prefsErr.Error(),
		})
		return
	}

	// Always inject TINGLY_API_URL so the statusline script (if installed)
	// targets the correct tingly-box port.  The env section is replaced on
	// every apply, so this must be set unconditionally to survive re-applies.
	env["TINGLY_API_URL"] = baseURL

	// Install status line script if requested (before applying settings)
	var err error

	var statusLineInstalled bool
	var statusLinePath string

	var opts []config.ApplyOption
	if req.InstallStatusLine {
		var scriptCreated bool
		statusLinePath, scriptCreated, err = config.InstallStatusLineScript()
		if err != nil {
			c.JSON(http.StatusInternalServerError, config.ApplyResult{
				Success: false,
				Message: "Failed to install status line script: " + err.Error(),
			})
			return
		}
		statusLineInstalled = true
		_ = scriptCreated // Used for tracking but not needed for response
		statusLine := map[string]any{"type": "command", "command": "~/.claude/tingly-statusline.sh"}
		opts = append(opts, config.WithExtra("statusLine", statusLine))
	}

	// Apply settings.json (now including statusLine config if requested)
	settingsResult, err := config.ApplyClaudeSettingsFromEnv(env, opts...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}

	if !settingsResult.Success {
		c.JSON(http.StatusInternalServerError, settingsResult)
		return
	}

	// Apply .claude.json
	onboardingPayload := map[string]interface{}{
		"hasCompletedOnboarding": true,
	}
	onboardingResult, err := config.ApplyClaudeOnboarding(onboardingPayload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}

	// Combine results
	combinedResult := config.ApplyResult{
		Success: settingsResult.Success && onboardingResult.Success,
		Message: "",
	}

	// Track backup paths
	backupPaths := []string{}
	if settingsResult.BackupPath != "" {
		backupPaths = append(backupPaths, settingsResult.BackupPath)
	}
	if onboardingResult.BackupPath != "" {
		backupPaths = append(backupPaths, onboardingResult.BackupPath)
	}

	// Track created/updated
	createdFiles := []string{}
	updatedFiles := []string{}
	if settingsResult.Created {
		createdFiles = append(createdFiles, "~/.claude/settings.json")
	} else {
		updatedFiles = append(updatedFiles, "~/.claude/settings.json")
	}
	if onboardingResult.Created {
		createdFiles = append(createdFiles, "~/.claude.json")
	} else {
		updatedFiles = append(updatedFiles, "~/.claude.json")
	}

	// Add status line script to created/updated files
	if statusLineInstalled {
		createdFiles = append(createdFiles, statusLinePath)
	}

	// Build response
	response := ApplyConfigResponse{
		Success:          combinedResult.Success,
		SettingsResult:   *settingsResult,
		OnboardingResult: *onboardingResult,
		CreatedFiles:     createdFiles,
		UpdatedFiles:     updatedFiles,
		BackupPaths:      backupPaths,
	}

	c.JSON(http.StatusOK, response)
}

// ApplyOpenCodeConfig generates and applies OpenCode configuration from system state
func (h *Handler) ApplyOpenCodeConfigFromState(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// Get base URL from the user's request (respects reverse proxy headers)
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := middleware.BaseURLFromRequest(c, port)
	configBaseURL := baseURL + "/tingly/opencode"

	// Use the model token from config (tingly-box- prefixed JWT)
	apiKey := h.config.GetModelToken()

	// Models are collected from active rules when available; an empty map
	// is fine — BuildOpenCodeConfig fills in a sensible default.
	models := make(map[string]interface{})

	// Generate OpenCode config with collected models
	payload := agent.BuildOpenCodeConfig(configBaseURL, apiKey, models)

	result, err := config.ApplyOpenCodeConfig(payload)
	if err != nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}

	// Build response
	response := ApplyOpenCodeConfigResponse{
		ApplyResult: *result,
	}

	c.JSON(http.StatusOK, response)
}

// RestoreClaudeConfig rolls back Claude Code config files to their most
// recent backup. Mirrors the CLI 'agent restore claude-code' flow.
func (h *Handler) RestoreClaudeConfig(c *gin.Context) {
	h.restoreAgent(c, agent.AgentTypeClaudeCode)
}

// RestoreOpenCodeConfig rolls back OpenCode config files to their most
// recent backup. Mirrors the CLI 'agent restore opencode' flow.
func (h *Handler) RestoreOpenCodeConfig(c *gin.Context) {
	h.restoreAgent(c, agent.AgentTypeOpenCode)
}

// restoreAgent runs the shared restore flow for the given agent type and
// writes the appropriate JSON response.
func (h *Handler) restoreAgent(c *gin.Context, agentType agent.AgentType) {
	if h.config == nil {
		c.JSON(http.StatusInternalServerError, RestoreConfigResponse{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	host := h.host
	if host == "" {
		host = "localhost"
	}
	apply := agent.NewAgentApply(h.config, host)

	result, err := apply.RestoreAgent(&agent.RestoreAgentRequest{
		AgentType: agentType,
		Force:     true,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, RestoreConfigResponse{
			Success:   false,
			AgentType: string(agentType),
			Message:   "Restore failed: " + err.Error(),
		})
		return
	}

	resp := RestoreConfigResponse{
		Success:           result.Success,
		AgentType:         string(result.AgentType),
		RestoredFiles:     result.RestoredFiles,
		PreRestoreBackups: result.PreRestoreBackups,
		Failures:          result.Failures,
		Message:           result.Message,
	}
	if !result.Success {
		// Still return 200 so the structured payload reaches the client; the
		// "success" field and Failures already convey the error state.
		c.JSON(http.StatusOK, resp)
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetOpenCodeConfigPreview generates OpenCode configuration preview from system state
// This endpoint returns the config JSON for display purposes without applying it
func (h *Handler) GetOpenCodeConfigPreview(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// Get base URL from the user's request (respects reverse proxy headers)
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := middleware.BaseURLFromRequest(c, port)
	configBaseURL := baseURL + "/tingly/opencode"

	// Use the model token from config (tingly-box- prefixed JWT)
	apiKey := h.config.GetModelToken()

	// Models are collected from active rules when available; an empty map
	// is fine — BuildOpenCodeConfig fills in a sensible default.
	models := make(map[string]interface{})

	// Generate OpenCode config JSON
	configPayload := agent.BuildOpenCodeConfig(configBaseURL, apiKey, models)

	configJSON, err := json.MarshalIndent(configPayload, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "Failed to generate config JSON: " + err.Error(),
		})
		return
	}

	// Marshal models to JSON for the script
	modelsJSON, err := json.Marshal(models)
	if err != nil {
		c.JSON(http.StatusInternalServerError, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "Failed to marshal models: " + err.Error(),
		})
		return
	}

	// Generate Windows script
	scriptWindows := generateOpenCodeScript(configBaseURL, apiKey, string(modelsJSON), "windows")

	// Generate Unix script
	scriptUnix := generateOpenCodeScript(configBaseURL, apiKey, string(modelsJSON), "unix")

	c.JSON(http.StatusOK, OpenCodeConfigPreviewResponse{
		Success:    true,
		ConfigJSON: string(configJSON),
		ScriptWin:  scriptWindows,
		ScriptUnix: scriptUnix,
	})
}

// generateOpenCodeScript generates a setup script for OpenCode configuration
// modelsJSON is a JSON string of the models map
func generateOpenCodeScript(configBaseURL, apiKey, modelsJSON, platform string) string {
	nodeCode := fmt.Sprintf(`const fs = require("fs");
const path = require("path");
const os = require("os");

const homeDir = os.homedir();
const configDir = path.join(homeDir, ".config", "opencode");
const configPath = path.join(configDir, "opencode.json");

// Create config directory if it doesn't exist
if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
}

const models = %s;

const newProvider = {
    "tingly-box": {
        "name": "tingly-box",
        "npm": "@ai-sdk/anthropic",
        "options": {
            "baseURL": "%s",
            "apiKey": "%s"
        },
        "models": models
    }
};

let existingConfig = {};
if (fs.existsSync(configPath)) {
    const content = fs.readFileSync(configPath, "utf-8");
    existingConfig = JSON.parse(content);
}

// Merge providers
const newConfig = {
    ...existingConfig,
    "$schema": existingConfig["$schema"] || "https://opencode.ai/config.json",
    "provider": {
        ...(existingConfig.provider || {}),
        ...newProvider
    }
};

fs.writeFileSync(configPath, JSON.stringify(newConfig, null, 2));
console.log("OpenCode config written to", configPath);`, modelsJSON, configBaseURL, apiKey)

	if platform == "windows" {
		return "# PowerShell - Run in PowerShell\nnode -e @\"\n" + nodeCode + "\n\"@"
	}
	// Unix - escape single quotes
	escapedCode := strings.ReplaceAll(nodeCode, "'", "'\\''")
	return "# Bash - Run in terminal\nnode -e '" + escapedCode + "'"
}

// collectCodexRuleModels returns the request_models of every active rule in
// the Codex scenario, deduplicated and in declaration order.
func collectCodexRuleModels(cfg *config.Config) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, rule := range cfg.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioCodex || !rule.Active {
			continue
		}
		model := strings.TrimSpace(rule.RequestModel)
		if model == "" {
			continue
		}
		if _, dup := seen[model]; dup {
			continue
		}
		seen[model] = struct{}{}
		out = append(out, model)
	}
	return out
}

// ApplyCodexConfigFromState applies the Codex CLI configuration derived from
// the active Codex scenario rules. Mirrors the OpenCode endpoint: it does NOT
// touch routing rules — those are managed via the rules UI.
func (h *Handler) ApplyCodexConfigFromState(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// preferences is optional; an absent/empty body falls back to defaults.
	var req ApplyCodexConfigRequest
	_ = c.ShouldBindJSON(&req)
	prefs := req.Preferences
	if prefs == nil {
		prefs = config.DefaultCodexPrefs()
	}

	models := collectCodexRuleModels(cfg)

	// Build context windows map from rules for models with context_1m flag
	contextWindows := config.BuildContextWindowsFromRules(cfg)

	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	codexBaseURL := middleware.BaseURLFromRequest(c, port) + "/tingly/codex"
	apiKey := h.config.GetModelToken()

	writeCatalog := req.WriteCatalog == nil || *req.WriteCatalog

	authMode := config.CodexAuthMode(req.AuthMode)
	var chatgptTokens *config.CodexChatGPTTokens
	if authMode == config.CodexAuthChatGPT {
		tokens, err := h.loadCodexChatGPTTokens(req.OAuthProviderUUID)
		if err != nil {
			c.JSON(http.StatusBadRequest, ApplyCodexConfigResponse{
				Success: false,
				Message: err.Error(),
			})
			return
		}
		chatgptTokens = tokens
	}

	// ChatGPT mode: clear tingly gateway keys from config.toml so codex CLI
	// uses its own defaults, then leave the rest of config.toml untouched.
	// Gateway mode: full rewrite as before.
	var (
		configResult *config.ApplyResult
		err          error
	)
	if authMode == config.CodexAuthChatGPT {
		configResult, err = config.ClearCodexGatewayConfig()
	} else {
		configResult, err = config.ApplyCodexConfigWithContextWindows(codexBaseURL, models, prefs, writeCatalog, contextWindows)
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyCodexConfigResponse{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}
	authResult, err := config.ApplyCodexAuth(authMode, apiKey, chatgptTokens)
	if err != nil {
		c.JSON(http.StatusInternalServerError, ApplyCodexConfigResponse{
			Success: false,
			Message: "Internal error: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, ApplyCodexConfigResponse{
		Success:        configResult.Success && authResult.Success,
		ConfigResult:   *configResult,
		AuthResult:     *authResult,
		CatalogWritten: writeCatalog && len(models) > 0 && authMode != config.CodexAuthChatGPT,
		Models:         models,
	})
}

// loadCodexChatGPTTokens pulls the OAuth credentials of the Codex provider
// identified by uuid out of config storage and shapes them for the native
// auth.json writer. We rely on the provider record (not oauth.Manager) because
// configapply already has the global config in hand and providers are the
// canonical persistence target for OAuth tokens.
func (h *Handler) loadCodexChatGPTTokens(uuid string) (*config.CodexChatGPTTokens, error) {
	if uuid == "" {
		return nil, fmt.Errorf("oauthProviderUuid is required for chatgpt auth mode")
	}
	provider, err := h.config.GetProviderByUUID(uuid)
	if err != nil {
		return nil, fmt.Errorf("oauth provider not found: %w", err)
	}
	if !provider.IsCodexProvider() || provider.OAuthDetail == nil {
		return nil, fmt.Errorf("provider %q is not a Codex OAuth provider", provider.Name)
	}
	return &config.CodexChatGPTTokens{
		AccessToken:  provider.OAuthDetail.AccessToken,
		RefreshToken: provider.OAuthDetail.RefreshToken,
		IDToken:      provider.OAuthDetail.GetExtraFieldString("id_token"),
		AccountID:    provider.OAuthDetail.GetExtraFieldString("account_id"),
	}, nil
}

// GetCodexConfigPreview returns the TOML/JSON that ApplyCodexConfigFromState
// would write to a fresh file. The real apply still merges into any existing
// ~/.codex/config.toml; this preview just shows the managed slice.
func (h *Handler) GetCodexConfigPreview(c *gin.Context) {
	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, CodexConfigPreviewResponse{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// preferences is optional; an absent/empty body falls back to defaults.
	var req ApplyCodexConfigRequest
	_ = c.ShouldBindJSON(&req)
	prefs := req.Preferences
	if prefs == nil {
		prefs = config.DefaultCodexPrefs()
	}

	models := collectCodexRuleModels(cfg)

	// Build context windows map from rules for models with context_1m flag
	contextWindows := config.BuildContextWindowsFromRules(cfg)

	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	codexBaseURL := middleware.BaseURLFromRequest(c, port) + "/tingly/codex"
	apiKey := h.config.GetModelToken()

	writeCatalog := req.WriteCatalog == nil || *req.WriteCatalog
	tomlBytes, err := config.RenderCodexConfigTOML(codexBaseURL, models, prefs, writeCatalog)
	if err != nil {
		c.JSON(http.StatusInternalServerError, CodexConfigPreviewResponse{
			Success: false,
			Message: "Failed to render config: " + err.Error(),
		})
		return
	}

	authBytes, err := json.MarshalIndent(map[string]string{"OPENAI_API_KEY": apiKey}, "", "  ")
	if err != nil {
		c.JSON(http.StatusInternalServerError, CodexConfigPreviewResponse{
			Success: false,
			Message: "Failed to render auth: " + err.Error(),
		})
		return
	}

	resp := CodexConfigPreviewResponse{
		Success:    true,
		ConfigToml: string(tomlBytes),
		AuthJson:   string(authBytes),
		Models:     models,
	}
	if writeCatalog && len(models) > 0 {
		catalogBytes, err := config.RenderCodexModelCatalog(models, contextWindows)
		if err == nil {
			resp.CatalogJson = string(catalogBytes)
		}
	}
	c.JSON(http.StatusOK, resp)
}

// RestoreCodexConfig rolls back Codex config files to their most recent backup.
func (h *Handler) RestoreCodexConfig(c *gin.Context) {
	h.restoreAgent(c, agent.AgentTypeCodex)
}
