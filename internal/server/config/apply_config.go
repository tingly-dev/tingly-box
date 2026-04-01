package config

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/tingly-dev/tingly-box/internal"
)

// ApplyResult contains the result of applying a configuration
type ApplyResult struct {
	Success    bool   `json:"success"`
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message"`
	Created    bool   `json:"created,omitempty"`
	Updated    bool   `json:"updated,omitempty"`
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

// generateBackupPath generates a backup file path with timestamp in a backup subdirectory
// Backup is placed in <original-file-directory>/backup/<filename>.bak-<timestamp><ext>
func generateBackupPath(originalPath string) string {
	now := time.Now()
	timestamp := now.Format("20060102-150405")
	ext := filepath.Ext(originalPath)
	base := filepath.Base(originalPath)
	dir := filepath.Dir(originalPath)

	// Place backup in a "backup" subdirectory of the original file's directory
	backupDir := filepath.Join(dir, "backup")
	return filepath.Join(backupDir, fmt.Sprintf("%s.bak-%s%s", base, timestamp, ext))
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

	// Ensure backup directory exists
	if err := os.MkdirAll(filepath.Dir(backupPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

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

type KV struct {
	Key   string
	Value any
}

// ApplyClaudeSettingsToPath applies Claude settings env vars to a specific target file.
// If the file exists, it merges the env section into the existing config (with backup).
// If not, it creates a new file with only the env section.
func ApplyClaudeSettingsToPath(targetPath string, env map[string]string, extras ...KV) (*ApplyResult, error) {
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
	_, err := os.Stat(targetPath)
	fileExists := err == nil

	var existingConfig map[string]interface{}
	if fileExists {
		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to read existing file: %v", err)
			return result, nil
		}

		if err := json.Unmarshal(data, &existingConfig); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing JSON: %v", err)
			return result, nil
		}

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
	for _, extra := range extras {
		existingConfig[extra.Key] = extra.Value
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

// ApplyClaudeSettingsFromEnv applies Claude settings configuration with env vars
// This is the safe version - env map is controlled by backend
func ApplyClaudeSettingsFromEnv(env map[string]string, extras ...KV) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	targetPath := filepath.Join(homeDir, ".claude", "settings.json")
	return ApplyClaudeSettingsToPath(targetPath, env, extras...)
}

// InstallStatusLineScript installs the tingly-statusline.sh script to ~/.claude/
// Returns the path to the installed script and whether it was newly created
func InstallStatusLineScript() (scriptPath string, created bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to get home directory: %w", err)
	}

	scriptPath = filepath.Join(homeDir, ".claude", "tingly-statusline.sh")

	// Read script from embedded assets
	content, err := internal.ScriptAssets.ReadFile("script/tingly-statusline.sh")
	if err != nil {
		return "", false, fmt.Errorf("failed to read status line script from assets: %w", err)
	}

	// Ensure directory exists
	if err := ensureDir(scriptPath); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file exists
	_, err = os.Stat(scriptPath)
	fileExists := err == nil

	// Write the script
	if err := os.WriteFile(scriptPath, content, 0755); err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}

	return scriptPath, !fileExists, nil
}

// InstallNotifyScript installs the claude-notify.sh script to ~/.claude/
// Returns the path to the installed script and whether it was newly created
func InstallNotifyScript() (scriptPath string, created bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to get home directory: %w", err)
	}

	scriptPath = filepath.Join(homeDir, ".claude", "tingly-notify.sh")

	// Read script from embedded assets
	content, err := internal.ScriptAssets.ReadFile("script/claude-notify.sh")
	if err != nil {
		return "", false, fmt.Errorf("failed to read notify script from assets: %w", err)
	}

	// Ensure directory exists
	if err := ensureDir(scriptPath); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	// Check if file exists
	_, err = os.Stat(scriptPath)
	fileExists := err == nil

	// Write the script
	if err := os.WriteFile(scriptPath, content, 0755); err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}

	return scriptPath, !fileExists, nil
}

// NotifyHookEntries defines the Claude Code hooks to install for notifications.
// This can be passed to ApplyNotifyHooks or used directly in settings.json.
func NotifyHookEntries() map[string]interface{} {
	scriptCmd := "~/.claude/tingly-notify.sh"
	return map[string]interface{}{
		"Stop": []map[string]interface{}{
			{"matcher": "", "hooks": []map[string]interface{}{
				{"type": "command", "command": scriptCmd},
			}},
		},
		"Notification": []map[string]interface{}{
			{"matcher": "permission", "hooks": []map[string]interface{}{
				{"type": "command", "command": scriptCmd},
			}},
		},
		"PreToolUse": []map[string]interface{}{
			{"matcher": "AskUserQuestion", "hooks": []map[string]interface{}{
				{"type": "command", "command": scriptCmd},
			}},
		},
	}
}

// ApplyNotifyHooks installs the notify script and merges notification hooks into settings.json.
// This is independent of the agent apply flow — it can be called standalone.
// Existing hooks with different matchers are preserved.
func ApplyNotifyHooks() (*ApplyResult, error) {
	_, _, err := InstallNotifyScript()
	if err != nil {
		return nil, fmt.Errorf("failed to install notify script: %w", err)
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetPath := filepath.Join(homeDir, ".claude", "settings.json")

	result := &ApplyResult{}

	// Read existing or create new
	var existingConfig map[string]interface{}
	data, err := os.ReadFile(targetPath)
	if err != nil {
		existingConfig = make(map[string]interface{})
		result.Created = true
	} else {
		if err := json.Unmarshal(data, &existingConfig); err != nil {
			return nil, fmt.Errorf("failed to parse settings.json: %w", err)
		}
		backupPath, err := backupFile(targetPath)
		if err != nil {
			return nil, err
		}
		result.BackupPath = backupPath
		result.Updated = true
	}

	// Merge hooks: append tingly-box entries, skip if same event+matcher+command already exists
	newHooks := NotifyHookEntries()
	existingHooks, ok := existingConfig["hooks"].(map[string]interface{})
	if !ok {
		existingHooks = make(map[string]interface{})
	}
	for event, newEntries := range newHooks {
		// Preserve existing entries for this event
		var merged []interface{}
		if cur, ok := existingHooks[event]; ok {
			if arr, ok := cur.([]interface{}); ok {
				merged = arr
			}
		}
		// Append new entries that don't already exist (matched by event+matcher+command)
		for _, ne := range newEntries.([]map[string]interface{}) {
			if hasHookEntry(merged, ne) {
				continue // already configured, skip
			}
			merged = append(merged, ne)
		}
		existingHooks[event] = merged
	}
	existingConfig["hooks"] = existingHooks

	// Write
	output, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	if err := os.WriteFile(targetPath, output, 0644); err != nil {
		return nil, fmt.Errorf("failed to write settings.json: %w", err)
	}

	result.Success = true
	if result.Created {
		result.Message = "Created " + targetPath
	} else {
		result.Message = "Updated " + targetPath
	}
	return result, nil
}

// hasHookEntry checks if an entry with the same matcher and command already exists in entries.
func hasHookEntry(entries []interface{}, needle map[string]interface{}) bool {
	needleMatcher, _ := needle["matcher"].(string)
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		matcher, _ := entry["matcher"].(string)
		if matcher != needleMatcher {
			continue
		}
		// Check if any hook in this entry has the same command
		if hooks, ok := entry["hooks"].([]interface{}); ok {
			for _, h := range hooks {
				if hMap, ok := h.(map[string]interface{}); ok {
					if cmd, _ := hMap["command"].(string); cmd == needle["command"] {
						return true
					}
				}
			}
		}
	}
	return false
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
