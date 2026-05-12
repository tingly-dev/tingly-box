package agent

import (
	"fmt"
	"strings"

	serverconfig "github.com/tingly-dev/tingly-box/internal/server/config"
)

// OpenCodeConfig implements AgentConfig for OpenCode
type OpenCodeConfig struct{}

// OpenCodeParams contains parameters for applying OpenCode configuration
type OpenCodeParams struct {
	// ConfigBaseURL is the base URL for OpenCode API endpoint
	ConfigBaseURL string
	// APIKey is the authentication token
	APIKey string
	// Models is a map of model-name -> model config
	// If nil or empty, uses default single-model layout
	Models map[string]interface{}
}

// Apply applies OpenCode configuration
func (o *OpenCodeConfig) Apply(paramsInterface interface{}) (*ApplyAgentResult, error) {
	params, ok := paramsInterface.(*OpenCodeParams)
	if !ok {
		return nil, fmt.Errorf("invalid params type, expected *OpenCodeParams")
	}

	// Generate config payload
	payload := GenerateOpenCodePayload(params.ConfigBaseURL, params.APIKey, params.Models)

	// Apply config
	applyResult, err := serverconfig.ApplyOpenCodeConfig(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to apply OpenCode config: %w", err)
	}

	result := &ApplyAgentResult{
		AgentType: AgentTypeOpenCode,
		Success:   applyResult.Success,
	}

	if applyResult.Success {
		result.ConfigFiles = []string{"~/.config/opencode/opencode.json"}
		if applyResult.Created {
			result.ConfigFiles[0] += " (created)"
		} else {
			result.ConfigFiles[0] += " (updated)"
		}
	}

	if applyResult.BackupPath != "" {
		result.BackupPaths = []string{applyResult.BackupPath}
	}

	return result, nil
}

// Restore restores OpenCode configuration from backup
func (o *OpenCodeConfig) Restore() (*RestoreAgentResult, error) {
	return RestoreAgent(AgentTypeOpenCode)
}

// ApplyOpenCode applies OpenCode configuration.
// This function does NOT handle routing rules - that's done by the caller.
// Deprecated: Use OpenCodeConfig.Apply() instead
func ApplyOpenCode(params *OpenCodeParams) (*ApplyAgentResult, error) {
	config := &OpenCodeConfig{}
	return config.Apply(params)
}

// GenerateOpenCodePayload generates the OpenCode configuration payload.
// models is a map of model-name → model config object (map[string]interface{}).
// Pass nil or empty map to use the default single-model layout.
func GenerateOpenCodePayload(configBaseURL, apiKey string, models map[string]interface{}) map[string]interface{} {
	if len(models) == 0 {
		ruleName := "tingly-opencode"
		models = map[string]interface{}{
			ruleName: map[string]interface{}{"name": ruleName},
		}
	}
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
	return map[string]interface{}{
		"$schema":  "https://opencode.ai/config.json",
		"provider": providerConfig,
	}
}

// GenerateOpenCodeScript generates a setup script for OpenCode configuration.
// modelsJSON is a JSON string of the models map.
func GenerateOpenCodeScript(configBaseURL, apiKey, modelsJSON, platform string) string {
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
