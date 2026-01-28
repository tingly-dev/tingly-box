package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// ApplyResult contains the result of applying a configuration
type ApplyResult struct {
	Success     bool   `json:"success"`
	BackupPath  string `json:"backupPath,omitempty"`
	Message     string `json:"message"`
	Created     bool   `json:"created,omitempty"`
	Updated     bool   `json:"updated,omitempty"`
}

// ClaudeSettingsPayload contains the payload for applying Claude settings
type ClaudeSettingsPayload struct {
	Env map[string]string `json:"env"`
}

// OpenCodeProviderConfig contains the provider configuration for OpenCode
type OpenCodeProviderConfig struct {
	Name    string                 `json:"name"`
	NPM     string                 `json:"npm"`
	Options map[string]interface{} `json:"options"`
	Models  map[string]interface{} `json:"models"`
}

// OpenCodeConfigPayload contains the payload for applying OpenCode config
type OpenCodeConfigPayload struct {
	Provider map[string]OpenCodeProviderConfig `json:"provider"`
}

// generateBackupPath generates a backup file path with timestamp
func generateBackupPath(originalPath string) string {
	now := time.Now()
	timestamp := now.Format("20060102-150405")
	ext := filepath.Ext(originalPath)
	base := originalPath[:len(originalPath)-len(ext)]
	return fmt.Sprintf("%s.bak-%s%s", base, timestamp, ext)
}

// backupFile creates a backup of the existing file
func backupFile(path string) (string, error) {
	// Read the original file
	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open original file: %w", err)
	}
	defer src.Close()

	backupPath := generateBackupPath(path)
	dst, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to create backup file: %w", err)
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return "", fmt.Errorf("failed to copy to backup: %w", err)
	}

	return backupPath, nil
}

// ensureDir ensures the directory for the given path exists
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

// ApplyClaudeSettingsFromEnv applies Claude settings configuration with env vars
// This is the safe version - env map is controlled by backend
func ApplyClaudeSettingsFromEnv(env map[string]string) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	targetPath := filepath.Join(homeDir, ".claude", "settings.json")
	result := &ApplyResult{
		Success: false,
		Message: "",
	}

	// Ensure directory exists
	if err := ensureDir(targetPath); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	// Check if file exists
	_, err = os.Stat(targetPath)
	fileExists := err == nil

	var existingConfig map[string]interface{}
	if fileExists {
		// Read existing file
		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to read existing file: %v", err)
			return result, nil
		}

		// Parse existing config
		if err := json.Unmarshal(data, &existingConfig); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing JSON: %v", err)
			return result, nil
		}

		// Create backup
		backupPath, err := backupFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create backup: %v", err)
			return result, nil
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		existingConfig = make(map[string]interface{})
		result.Created = true
	}

	// Merge env section - replace the entire env key with new env
	envInterface := make(map[string]interface{})
	for k, v := range env {
		envInterface[k] = v
	}
	existingConfig["env"] = envInterface

	// Write the merged config
	output, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal JSON: %v", err)
		return result, nil
	}

	if err := os.WriteFile(targetPath, output, 0644); err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	result.Success = true
	if result.Created {
		result.Message = fmt.Sprintf("Created %s", targetPath)
	} else if result.BackupPath != "" {
		result.Message = fmt.Sprintf("Updated %s (backup: %s)", targetPath, result.BackupPath)
	} else {
		result.Message = fmt.Sprintf("Updated %s", targetPath)
	}

	return result, nil
}

// ApplyClaudeOnboarding applies Claude onboarding configuration
// It merges top-level keys, preserving existing keys not in payload
func ApplyClaudeOnboarding(payload map[string]interface{}) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	targetPath := filepath.Join(homeDir, ".claude.json")
	result := &ApplyResult{
		Success: false,
		Message: "",
	}

	// Ensure directory exists (though .claude.json is usually in home)
	if err := ensureDir(targetPath); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	// Check if file exists
	_, err = os.Stat(targetPath)
	fileExists := err == nil

	var existingConfig map[string]interface{}
	if fileExists {
		// Read existing file
		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to read existing file: %v", err)
			return result, nil
		}

		// Parse existing config
		if err := json.Unmarshal(data, &existingConfig); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing JSON: %v", err)
			return result, nil
		}

		// Create backup
		backupPath, err := backupFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create backup: %v", err)
			return result, nil
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		existingConfig = make(map[string]interface{})
		result.Created = true
	}

	// Merge top-level keys from payload
	for k, v := range payload {
		existingConfig[k] = v
	}

	// Write the merged config
	output, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal JSON: %v", err)
		return result, nil
	}

	if err := os.WriteFile(targetPath, output, 0644); err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	result.Success = true
	if result.Created {
		result.Message = fmt.Sprintf("Created %s", targetPath)
	} else if result.BackupPath != "" {
		result.Message = fmt.Sprintf("Updated %s (backup: %s)", targetPath, result.BackupPath)
	} else {
		result.Message = fmt.Sprintf("Updated %s", targetPath)
	}

	return result, nil
}

// ApplyOpenCodeConfig applies OpenCode configuration
// It merges the provider map while preserving other providers and settings
func ApplyOpenCodeConfig(payload map[string]interface{}) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "opencode")
	targetPath := filepath.Join(configDir, "opencode.json")
	result := &ApplyResult{
		Success: false,
		Message: "",
	}

	// Ensure directory exists
	if err := os.MkdirAll(configDir, 0755); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	// Check if file exists
	_, err = os.Stat(targetPath)
	fileExists := err == nil

	var existingConfig map[string]interface{}
	if fileExists {
		// Read existing file
		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to read existing file: %v", err)
			return result, nil
		}

		// Parse existing config
		if err := json.Unmarshal(data, &existingConfig); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing JSON: %v", err)
			return result, nil
		}

		// Create backup
		backupPath, err := backupFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create backup: %v", err)
			return result, nil
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		existingConfig = make(map[string]interface{})
		result.Created = true
	}

	// Ensure $schema default
	if _, ok := existingConfig["$schema"]; !ok {
		existingConfig["$schema"] = "https://opencode.ai/config.json"
	}

	// Get existing providers or create empty map
	existingProviders := make(map[string]interface{})
	if providers, ok := existingConfig["provider"].(map[string]interface{}); ok {
		existingProviders = providers
	}

	// Merge new providers from payload
	if newProviders, ok := payload["provider"].(map[string]interface{}); ok {
		for k, v := range newProviders {
			existingProviders[k] = v
		}
	}

	existingConfig["provider"] = existingProviders

	// Write the merged config
	output, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal JSON: %v", err)
		return result, nil
	}

	if err := os.WriteFile(targetPath, output, 0644); err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	result.Success = true
	if result.Created {
		result.Message = fmt.Sprintf("Created %s", targetPath)
	} else if result.BackupPath != "" {
		result.Message = fmt.Sprintf("Updated %s (backup: %s)", targetPath, result.BackupPath)
	} else {
		result.Message = fmt.Sprintf("Updated %s", targetPath)
	}

	return result, nil
}
