package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"path/filepath"

	"github.com/tingly-dev/tingly-box/internal"
)

// Claude Code settings.json application: build/apply managed settings,
// install hook scripts, and merge notify/IM hooks and onboarding state.

// ClaudeSettingsPayload contains the payload for applying Claude settings
type ClaudeSettingsPayload struct {
	Env map[string]string `json:"env"`
}

// BuildClaudeSettings renders a complete Claude settings document from the
// supplied base without touching disk. Generated runtime artifacts use this
// to publish one complete snapshot instead of exposing intermediate writes.
func BuildClaudeSettings(base []byte, env map[string]string, opts ...ApplyOption) ([]byte, error) {
	return buildClaudeSettings(base, env, resolveApplyOptions(opts...))
}

func buildClaudeSettings(base []byte, env map[string]string, applyOpts *applyOptions) ([]byte, error) {
	existingConfig := make(map[string]interface{})
	if len(bytes.TrimSpace(base)) > 0 {
		if err := json.Unmarshal(base, &existingConfig); err != nil {
			return nil, fmt.Errorf("failed to parse existing JSON: %w", err)
		}
	}
	if existingConfig == nil {
		existingConfig = make(map[string]interface{})
	}

	// Replace the entire env key with the generated environment.
	envInterface := make(map[string]interface{}, len(env))
	for k, v := range env {
		envInterface[k] = v
	}
	existingConfig["env"] = envInterface

	if applyOpts.defaultMode != "" {
		// Claude Code's settings-reference documents this key as nested under
		// "permissions" (see .design/claude-code-config.md). The legacy
		// top-level key is also written so the setting still takes effect on
		// older CLI installs that predate the move; readClaudeCodeSettings
		// prefers the nested value when both are present.
		existingConfig["defaultMode"] = applyOpts.defaultMode
		permissions, _ := existingConfig["permissions"].(map[string]interface{})
		if permissions == nil {
			permissions = map[string]interface{}{}
		}
		permissions["defaultMode"] = applyOpts.defaultMode
		existingConfig["permissions"] = permissions
	}
	if applyOpts.showThinkingSummaries != nil {
		existingConfig["showThinkingSummaries"] = *applyOpts.showThinkingSummaries
	}
	maps.Copy(existingConfig, applyOpts.extras)

	output, err := json.MarshalIndent(existingConfig, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal JSON: %w", err)
	}
	return output, nil
}

// ApplyClaudeSettingsToPath applies Claude settings env vars to a specific target file.
// If the file exists, it merges the env section into the existing config (with backup).
// If not, it creates a new file with only the env section.
func ApplyClaudeSettingsToPath(targetPath string, env map[string]string, opts ...ApplyOption) (*ApplyResult, error) {
	result := &ApplyResult{
		Success: false,
		Message: "",
	}

	applyOpts := resolveApplyOptions(opts...)

	// Ensure directory exists
	if err := ensureDir(targetPath); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	// Check if file exists
	_, err := os.Stat(targetPath)
	fileExists := err == nil

	base := []byte("{}")
	if fileExists {
		data, err := os.ReadFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to read existing file: %v", err)
			return result, nil
		}
		base = data
	}

	output, err := buildClaudeSettings(base, env, applyOpts)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to build settings JSON: %v", err)
		return result, nil
	}

	if fileExists {
		// Only create backup if enabled
		if applyOpts.backup {
			backupPath, err := backupFileWithRetention(targetPath, applyOpts.retention)
			if err != nil {
				result.Message = fmt.Sprintf("Failed to create backup: %v", err)
				return result, nil
			}
			result.BackupPath = backupPath
		}
		result.Updated = true
	} else {
		result.Created = true
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
func ApplyClaudeSettingsFromEnv(env map[string]string, opts ...ApplyOption) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	targetPath := filepath.Join(homeDir, ".claude", "settings.json")
	return ApplyClaudeSettingsToPath(targetPath, env, opts...)
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

	created, err = writeManagedFileIfChanged(scriptPath, content, 0755)
	if err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}
	return scriptPath, created, nil
}

// InstallNotifyScript installs the tingly-notify.sh script (push-only) to ~/.claude/
// Returns the path to the installed script and whether it was newly created
func InstallNotifyScript() (scriptPath string, created bool, err error) {
	return installScript("tingly-notify.sh", "script/tingly-notify.sh")
}

// InstallIMHookScript installs the tingly-im-hook.sh script (interactive approval) to ~/.claude/
// Returns the path to the installed script and whether it was newly created
func InstallIMHookScript() (scriptPath string, created bool, err error) {
	return installScript("tingly-im-hook.sh", "script/tingly-im-hook.sh")
}

// installScript is a helper that installs a script from embedded assets to ~/.claude/
func installScript(targetName, assetPath string) (scriptPath string, created bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to get home directory: %w", err)
	}

	scriptPath = filepath.Join(homeDir, ".claude", targetName)

	// Read script from embedded assets
	content, err := internal.ScriptAssets.ReadFile(assetPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to read script from assets: %w", err)
	}

	// Ensure directory exists
	if err := ensureDir(scriptPath); err != nil {
		return "", false, fmt.Errorf("failed to create directory: %w", err)
	}

	created, err = writeManagedFileIfChanged(scriptPath, content, 0755)
	if err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}
	return scriptPath, created, nil
}

// NotifyHookEntries defines the Claude Code hooks to install for PUSH-ONLY notifications.
// This includes Stop events and completion-type notifications.
// For interactive approval hooks (PreToolUse, permission notifications), use ImHookEntries instead.
func NotifyHookEntries() map[string]interface{} {
	scriptCmd := "~/.claude/tingly-notify.sh"
	return map[string]interface{}{
		"Stop": []map[string]interface{}{
			{"matcher": "", "hooks": []map[string]interface{}{
				{"type": "command", "command": scriptCmd},
			}},
		},
		"Notification": []map[string]interface{}{
			{"matcher": "completion", "hooks": []map[string]interface{}{
				{"type": "command", "command": scriptCmd},
			}},
		},
	}
}

// ImHookEntries defines the Claude Code hooks to install for INTERACTIVE approval via IM.
// This includes PreToolUse (all tool calls) and permission-type notifications.
func ImHookEntries() map[string]interface{} {
	scriptCmd := "~/.claude/tingly-im-hook.sh"
	return map[string]interface{}{
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

// ApplyImHooks installs the IM hook script (interactive approval) and merges IM hooks into settings.json.
// This is independent of the agent apply flow — it can be called standalone.
// Existing hooks with different matchers are preserved.
func ApplyImHooks() (*ApplyResult, error) {
	_, _, err := InstallIMHookScript()
	if err != nil {
		return nil, fmt.Errorf("failed to install IM hook script: %w", err)
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
	newHooks := ImHookEntries()
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
		return nil, fmt.Errorf("failed to write file: %w", err)
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
	maps.Copy(existingConfig, payload)

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
