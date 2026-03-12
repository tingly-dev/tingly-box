package configapply

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/server/config"
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

// ApplyClaudeConfig generates and applies Claude Code configuration from system state
func (h *Handler) ApplyClaudeConfig(c *gin.Context) {
	var req struct {
		Mode              string `json:"mode"`              // "unified" or "separate"
		InstallStatusLine bool   `json:"installStatusLine"` // install status line integration
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		req.Mode = "unified" // default to unified
		req.InstallStatusLine = false
	}

	cfg := h.config
	if cfg == nil {
		c.JSON(http.StatusInternalServerError, config.ApplyResult{
			Success: false,
			Message: "Global config not available",
		})
		return
	}

	// Get rules for claude_code scenario
	rules := cfg.GetRequestConfigs()
	var claudeRules []typ.Rule
	for _, rule := range rules {
		if rule.GetScenario() == typ.ScenarioClaudeCode && rule.Active {
			claudeRules = append(claudeRules, rule)
		}
	}

	if len(claudeRules) == 0 {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "No active Claude Code rules found",
		})
		return
	}

	// Get the first active rule's provider
	firstRule := claudeRules[0]
	services := firstRule.GetServices()
	if len(services) == 0 {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "No services configured in Claude Code rule",
		})
		return
	}

	firstService := services[0]
	provider, err := cfg.GetProviderByUUID(firstService.Provider)
	if err != nil || provider == nil {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "Provider not found for Claude Code rule",
		})
		return
	}

	// Get base URL from server config or use default
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d", h.host, port)

	// Generate env vars based on mode
	env := map[string]string{
		"DISABLE_TELEMETRY":                        "1",
		"DISABLE_ERROR_REPORTING":                  "1",
		"CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1",
		"API_TIMEOUT_MS":                           "3000000",
		"ANTHROPIC_BASE_URL":                       baseURL + "/tingly/claude_code",
		"ANTHROPIC_AUTH_TOKEN":                     h.config.GetModelToken(),
	}

	if req.Mode == "separate" {
		env["ANTHROPIC_MODEL"] = "tingly/cc-default"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc-haiku"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc-opus"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc-sonnet"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc-subagent"
	} else {
		// Unified mode - all point to same model
		env["ANTHROPIC_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_HAIKU_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_OPUS_MODEL"] = "tingly/cc"
		env["ANTHROPIC_DEFAULT_SONNET_MODEL"] = "tingly/cc"
		env["CLAUDE_CODE_SUBAGENT_MODEL"] = "tingly/cc"
	}

	// Install status line script if requested (before applying settings)
	var statusLineInstalled bool
	var statusLinePath string

	var extras = []config.KV{}
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
		// Add statusLine config to env
		statusLine := map[string]any{"type": "command", "command": "~/.claude/tingly-statusline.sh"}
		extras = append(extras, config.KV{Key: "statusLine", Value: statusLine})
	}

	// Apply settings.json (now including statusLine config if requested)
	settingsResult, err := config.ApplyClaudeSettingsFromEnv(env, extras...)
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

	// Get rules for opencode scenario
	rules := cfg.GetRequestConfigs()
	var opencodeRules []typ.Rule
	for _, rule := range rules {
		if rule.GetScenario() == typ.ScenarioOpenCode && rule.Active {
			opencodeRules = append(opencodeRules, rule)
		}
	}

	if len(opencodeRules) == 0 {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "No active OpenCode rules found",
		})
		return
	}

	// Get the first active rule's provider
	firstRule := opencodeRules[0]
	services := firstRule.GetServices()
	if len(services) == 0 {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "No services configured in OpenCode rule",
		})
		return
	}

	firstService := services[0]
	provider, err := cfg.GetProviderByUUID(firstService.Provider)
	if err != nil || provider == nil {
		c.JSON(http.StatusBadRequest, config.ApplyResult{
			Success: false,
			Message: "Provider not found for OpenCode rule",
		})
		return
	}

	// Get base URL from server config or use default
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d", h.host, port)
	configBaseURL := baseURL + "/tingly/opencode"

	// Use the model token from config (tingly-box- prefixed JWT)
	apiKey := h.config.GetModelToken()

	// Collect all models from all active OpenCode rules
	models := make(map[string]interface{})
	for _, rule := range opencodeRules {
		requestModel := rule.RequestModel
		if requestModel == "" {
			requestModel = "tingly/cc-default"
		}
		models[requestModel] = map[string]interface{}{
			"name": requestModel,
		}
	}

	// Generate OpenCode config with all models
	providerConfig := map[string]interface{}{
		"tingly-box": map[string]interface{}{
			"name": "tingly-box",
			"npm":  "@ai-sdk/anthropic",
			"options": map[string]interface{}{
				"baseURL": configBaseURL,
				"apiKey":  apiKey,
			},
			"models": models,
		},
	}

	payload := map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}

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

	// Get rules for opencode scenario
	rules := cfg.GetRequestConfigs()
	var opencodeRules []typ.Rule
	for _, rule := range rules {
		if rule.GetScenario() == typ.ScenarioOpenCode && rule.Active {
			opencodeRules = append(opencodeRules, rule)
		}
	}

	if len(opencodeRules) == 0 {
		c.JSON(http.StatusBadRequest, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "No active OpenCode rules found",
		})
		return
	}

	// Get the first active rule's provider
	firstRule := opencodeRules[0]
	services := firstRule.GetServices()
	if len(services) == 0 {
		c.JSON(http.StatusBadRequest, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "No services configured in OpenCode rule",
		})
		return
	}

	firstService := services[0]
	provider, err := cfg.GetProviderByUUID(firstService.Provider)
	if err != nil || provider == nil {
		c.JSON(http.StatusBadRequest, OpenCodeConfigPreviewResponse{
			Success: false,
			Message: "Provider not found for OpenCode rule",
		})
		return
	}

	// Get base URL from server config or use default
	port := h.config.ServerPort
	if port == 0 {
		port = 12580
	}
	baseURL := fmt.Sprintf("http://%s:%d", h.host, port)
	configBaseURL := baseURL + "/tingly/opencode"

	// Use the model token from config (tingly-box- prefixed JWT)
	apiKey := h.config.GetModelToken()

	// Collect all models from all active OpenCode rules
	models := make(map[string]interface{})
	for _, rule := range opencodeRules {
		requestModel := rule.RequestModel
		if requestModel == "" {
			requestModel = "tingly/cc-default"
		}
		models[requestModel] = map[string]interface{}{
			"name": requestModel,
		}
	}

	// Generate OpenCode config JSON
	providerConfig := map[string]interface{}{
		"tingly-box": map[string]interface{}{
			"name": "tingly-box",
			"npm":  "@ai-sdk/anthropic",
			"options": map[string]interface{}{
				"baseURL": configBaseURL,
				"apiKey":  apiKey,
			},
			"models": models,
		},
	}

	configPayload := map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}

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
