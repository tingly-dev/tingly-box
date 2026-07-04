package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	tomlpkg "github.com/pelletier/go-toml/v2"
	"github.com/tingly-dev/tingly-box/internal"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// defaultBackupRetention is the default number of backup files to keep per
// original config file. Older backups beyond this count are removed by
// rotateBackups after each new backup is created.
const defaultBackupRetention = 3

// codexGatewayProviderName is the tingly-box provider key written into
// config.toml's [model_providers] table by mergeCodexConfig and removed by
// ClearCodexGatewayConfig.
const codexGatewayProviderName = "tingly-box"

// codexGatewayTopLevelKeys are the top-level config.toml keys owned by
// tingly-box. Both mergeCodexConfig (writer) and ClearCodexGatewayConfig
// (eraser) reference this list so the two functions stay in sync.
var codexGatewayTopLevelKeys = []string{"model", "model_provider", "model_catalog_json"}

// backupTimestampLayout matches the timestamp format embedded in backup
// filenames produced by generateBackupPath.
const backupTimestampLayout = "20060102-150405"

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

// backupFile creates a backup of the existing file and rotates older backups
// matching the same originalPath, keeping at most defaultBackupRetention copies.
// Rotation failures are logged but do not fail the backup itself, since the
// fresh backup has already been written successfully.
func backupFile(path string) (string, error) {
	return backupFileWithRetention(path, defaultBackupRetention)
}

// backupFileWithRetention is like backupFile but allows overriding the
// retention count. retention <= 0 falls back to defaultBackupRetention.
func backupFileWithRetention(path string, retention int) (string, error) {
	src, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("failed to open original file: %w", err)
	}
	defer src.Close()

	backupPath := generateBackupPath(path)

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

	if retention <= 0 {
		retention = defaultBackupRetention
	}
	// Best-effort rotation: a failure here must not invalidate the
	// freshly-written backup that the caller now depends on.
	_ = rotateBackups(path, retention)

	return backupPath, nil
}

// BackupInfo describes a single backup file for an original config path.
type BackupInfo struct {
	Path      string    `json:"path"`
	Timestamp time.Time `json:"timestamp"`
}

// ListBackups returns all backup files for originalPath in <dir>/backup/,
// ordered newest-first. Files that do not match the
// "<base>.bak-<timestamp><ext>" pattern are ignored.
func ListBackups(originalPath string) ([]BackupInfo, error) {
	dir := filepath.Dir(originalPath)
	base := filepath.Base(originalPath)
	ext := filepath.Ext(originalPath)
	backupDir := filepath.Join(dir, "backup")
	prefix := base + ".bak-"

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read backup directory: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, ext) {
			continue
		}
		stamp := strings.TrimSuffix(strings.TrimPrefix(name, prefix), ext)
		ts, err := time.ParseInLocation(backupTimestampLayout, stamp, time.Local)
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Path:      filepath.Join(backupDir, name),
			Timestamp: ts,
		})
	}

	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Timestamp.After(backups[j].Timestamp)
	})
	return backups, nil
}

// rotateBackups deletes older backups for originalPath, keeping at most the
// `keep` most recent ones. keep <= 0 falls back to defaultBackupRetention.
func rotateBackups(originalPath string, keep int) error {
	if keep <= 0 {
		keep = defaultBackupRetention
	}
	backups, err := ListBackups(originalPath)
	if err != nil {
		return err
	}
	if len(backups) <= keep {
		return nil
	}
	var firstErr error
	for _, b := range backups[keep:] {
		if err := os.Remove(b.Path); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// RestoreResult describes the outcome of restoring a single config file.
type RestoreResult struct {
	Success          bool   `json:"success"`
	OriginalPath     string `json:"originalPath"`
	RestoredFrom     string `json:"restoredFrom,omitempty"`
	PreRestoreBackup string `json:"preRestoreBackup,omitempty"`
	Message          string `json:"message"`
}

// RestoreLatestBackup restores originalPath from its most recent backup.
// If originalPath currently exists, a "pre-restore" backup of the live file
// is created first (and is itself subject to rotation) so the restore is
// reversible.
func RestoreLatestBackup(originalPath string) (*RestoreResult, error) {
	result := &RestoreResult{OriginalPath: originalPath}

	backups, err := ListBackups(originalPath)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to list backups: %v", err)
		return result, err
	}
	if len(backups) == 0 {
		result.Message = fmt.Sprintf("No backup found for %s", originalPath)
		return result, fmt.Errorf("no backup found for %s", originalPath)
	}

	latest := backups[0]
	result.RestoredFrom = latest.Path

	if _, err := os.Stat(originalPath); err == nil {
		preBackup, err := backupFile(originalPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create pre-restore backup: %v", err)
			return result, err
		}
		result.PreRestoreBackup = preBackup
	} else if !os.IsNotExist(err) {
		result.Message = fmt.Sprintf("Failed to stat original file: %v", err)
		return result, err
	}

	if err := ensureDir(originalPath); err != nil {
		result.Message = fmt.Sprintf("Failed to ensure target directory: %v", err)
		return result, err
	}

	if err := copyFile(latest.Path, originalPath); err != nil {
		result.Message = fmt.Sprintf("Failed to restore from backup: %v", err)
		return result, err
	}

	result.Success = true
	if result.PreRestoreBackup != "" {
		result.Message = fmt.Sprintf("Restored %s from %s (pre-restore backup: %s)",
			originalPath, latest.Path, result.PreRestoreBackup)
	} else {
		result.Message = fmt.Sprintf("Restored %s from %s", originalPath, latest.Path)
	}
	return result, nil
}

// copyFile copies src to dst, truncating dst if it exists.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

// ensureDir ensures the directory for the given path exists
func ensureDir(path string) error {
	dir := filepath.Dir(path)
	return os.MkdirAll(dir, 0755)
}

// ApplyOption is a functional option for ApplyClaudeSettingsToPath
type ApplyOption func(*applyOptions)

type applyOptions struct {
	backup      bool
	retention   int
	extras      map[string]any
	defaultMode string
}

// WithBackup enables or disables backup when applying settings.
// Default is true (create backup).
func WithBackup(enable bool) ApplyOption {
	return func(opts *applyOptions) {
		opts.backup = enable
	}
}

// WithBackupRetention overrides the default number of backups to keep
// after rotation. n <= 0 means use the package default.
func WithBackupRetention(n int) ApplyOption {
	return func(opts *applyOptions) {
		opts.retention = n
	}
}

// WithDefaultMode sets the Claude Code defaultMode value in settings.json.
func WithDefaultMode(mode string) ApplyOption {
	return func(opts *applyOptions) {
		opts.defaultMode = mode
	}
}

// WithExtra sets a single extra key-value pair to merge into the settings.
func WithExtra(key string, value any) ApplyOption {
	return func(opts *applyOptions) {
		if opts.extras == nil {
			opts.extras = make(map[string]any)
		}
		opts.extras[key] = value
	}
}

// ApplyClaudeSettingsToPath applies Claude settings env vars to a specific target file.
// If the file exists, it merges the env section into the existing config (with backup).
// If not, it creates a new file with only the env section.
func ApplyClaudeSettingsToPath(targetPath string, env map[string]string, opts ...ApplyOption) (*ApplyResult, error) {
	result := &ApplyResult{
		Success: false,
		Message: "",
	}

	// Parse options
	applyOpts := &applyOptions{
		backup: true, // default: enable backup
	}
	for _, opt := range opts {
		opt(applyOpts)
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
		existingConfig = make(map[string]interface{})
		result.Created = true
	}

	// Merge env section - replace the entire env key with new env
	envInterface := make(map[string]interface{})
	for k, v := range env {
		envInterface[k] = v
	}

	existingConfig["env"] = envInterface
	if applyOpts.defaultMode != "" {
		existingConfig["defaultMode"] = applyOpts.defaultMode
	}
	// Apply extras from options
	for k, v := range applyOpts.extras {
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

	// Check if file exists
	_, err = os.Stat(scriptPath)
	fileExists := err == nil

	// Write the script
	if err := os.WriteFile(scriptPath, content, 0755); err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}

	return scriptPath, !fileExists, nil
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

	// Check if file exists
	_, err = os.Stat(scriptPath)
	fileExists := err == nil

	// Write the script
	if err := os.WriteFile(scriptPath, content, 0755); err != nil {
		return "", false, fmt.Errorf("failed to write script: %w", err)
	}

	return scriptPath, !fileExists, nil
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

// codexModelCatalogFile is the basename of the tingly-managed Codex model
// catalog file written next to config.toml. config.toml's `model_catalog_json`
// is pointed at the absolute path of this file so `/model` can enumerate
// tingly-served models.
const codexModelCatalogFile = "tingly-model-catalog.json"

const codexModelCatalogSchema = "https://raw.githubusercontent.com/tingly-dev/tingly-box/main/internal/server/config/codex-model-catalog.schema.json"

// CodexPrefs is the typed, user-tunable surface of Codex's config.toml.
// JSON tags map 1:1 to the config.toml keys, so the frontend round-trips the
// same field names. Values are kept as strings so empty = omit (let Codex use
// its own default), avoiding the "0/false means unset" ambiguity.
//
// The struct itself is the whitelist: only these keys can ever be set from a
// request, so prefs can never clobber tingly-managed fields (model,
// model_provider, model_catalog_json, model_providers.*) or inject arbitrary
// TOML. Scope is deliberately limited to model/reasoning knobs (not
// approval_policy / sandbox_mode safety toggles).
type CodexPrefs struct {
	ModelReasoningEffort            string `json:"model_reasoning_effort,omitempty"`
	ModelReasoningSummary           string `json:"model_reasoning_summary,omitempty"`
	ModelVerbosity                  string `json:"model_verbosity,omitempty"`
	ModelSupportsReasoningSummaries string `json:"model_supports_reasoning_summaries,omitempty"`
}

// codexEnumValues lists the valid values for each enum-typed CodexPrefs field.
// Values outside the set are dropped during conversion (forward-compatible,
// injection-safe).
var codexEnumValues = map[string][]string{
	"model_reasoning_effort":  {"none", "minimal", "low", "medium", "high", "xhigh"},
	"model_reasoning_summary": {"auto", "concise", "detailed", "none"},
	"model_verbosity":         {"low", "medium", "high"},
}

// DefaultCodexPrefs returns the defaults for the CLI path and no-prefs fallback.
// All fields are empty so tingly-box stays out of the way for third-party
// providers that may not support OpenAI reasoning-summary extensions.
// Users who need reasoning summaries can enable them via the Quick Config form.
func DefaultCodexPrefs() *CodexPrefs {
	return &CodexPrefs{}
}

// toConfig converts prefs into a map of native TOML values ready to merge into
// config.toml. Empty values and invalid enum members are dropped; the bool
// field maps "true" -> true (anything else omitted).
func (p *CodexPrefs) toConfig() map[string]interface{} {
	out := map[string]interface{}{}
	if p == nil {
		return out
	}
	addEnum := func(key, val string) {
		val = strings.TrimSpace(val)
		if val == "" {
			return
		}
		for _, allowed := range codexEnumValues[key] {
			if val == allowed {
				out[key] = val
				return
			}
		}
	}
	addEnum("model_reasoning_effort", p.ModelReasoningEffort)
	addEnum("model_reasoning_summary", p.ModelReasoningSummary)
	addEnum("model_verbosity", p.ModelVerbosity)
	if strings.TrimSpace(p.ModelSupportsReasoningSummaries) == "true" {
		out["model_supports_reasoning_summaries"] = true
	}
	return out
}

// ApplyCodexConfig merges tingly-box Codex settings into ~/.codex/config.toml
// and writes ~/.codex/tingly-model-catalog.json with one entry per supplied
// model so Codex's `/model` picker can see them.
//
// This is the backward-compatible version that uses default context windows.
// For context window support, use ApplyCodexConfigWithContextWindows.
func ApplyCodexConfig(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool) (*ApplyResult, error) {
	return ApplyCodexConfigWithContextWindows(baseURL, models, prefs, writeCatalog, nil)
}

// ApplyCodexConfigWithContextWindows merges tingly-box Codex settings into ~/.codex/config.toml
// and writes ~/.codex/tingly-model-catalog.json with one entry per supplied
// model so Codex's `/model` picker can see them.
//
// The contextWindows parameter overrides the catalog's default context window
// for specific models (e.g., 1M for models with the context_1m flag); nil uses
// defaults.
//
// MERGE semantics: only fields tingly-box manages are overwritten. Everything
// else the user put in config.toml — other top-level keys, other entries under
// `[model_providers.*]`, and unrelated `[profiles.*]` blocks — is left alone.
//
// Managed fields:
//   - top-level `model` (set to models[0] when models is non-empty)
//   - top-level `model_provider = "tingly-box"`
//   - top-level `model_catalog_json` (set to the absolute path of the
//     catalog file when models is non-empty; cleared otherwise so we don't
//     point at a missing file)
//   - `[model_providers.tingly-box]` (always re-pinned to the supplied base URL)
//   - `[profiles.<sanitized(model)>]` for each model — overwritten unconditionally
//     under that key; `agent restore codex` recovers the previous file if needed
//   - the whitelisted user prefs (see codexPrefSpec, e.g.
//     `model_reasoning_effort`, `model_reasoning_summary`,
//     `model_supports_reasoning_summaries`, `model_verbosity`) at the top level
//     and inside each managed profile
//
// Note: Codex's `model_catalog_json` REPLACES the bundled catalog (it does not
// merge), and is read on startup only — switching via `/model` doesn't reload
// it. Users wanting native OpenAI entries in `/model` should keep the bundled
// catalog (i.e. not run apply) or merge by hand.
//
// Orphan tingly profiles from earlier applies are NOT garbage-collected; if
// the user has trimmed their rules they can remove stale profiles by hand.
//
// The previous config.toml and catalog (if any) are backed up before being
// rewritten.
func ApplyCodexConfigWithContextWindows(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool, contextWindows map[string]int) (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	if homeDir == "" {
		// os.UserHomeDir can succeed and return "" in odd container setups
		// where neither $HOME nor /etc/passwd resolves the current user.
		// We refuse to proceed because filepath.Join would emit "/.codex/..."
		// which Codex rejects as a non-absolute catalog path.
		return nil, fmt.Errorf("home directory resolved to empty path")
	}

	configDir := filepath.Join(homeDir, ".codex")
	targetPath := filepath.Join(configDir, "config.toml")
	catalogPath := filepath.Join(configDir, codexModelCatalogFile)
	result := &ApplyResult{}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	existing := map[string]interface{}{}
	if data, err := os.ReadFile(targetPath); err == nil {
		if err := tomlpkg.Unmarshal(data, &existing); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing TOML: %v", err)
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
		result.Created = true
	}

	catalogPathForConfig := ""
	if len(models) > 0 && writeCatalog {
		catalogPathForConfig = catalogPath
	}
	mergeCodexConfig(existing, baseURL, models, catalogPathForConfig, prefs)

	out, err := tomlpkg.Marshal(existing)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal TOML: %v", err)
		return result, nil
	}
	if err := os.WriteFile(targetPath, out, 0644); err != nil {
		result.Message = fmt.Sprintf("Failed to write file: %v", err)
		return result, nil
	}

	if len(models) > 0 && writeCatalog {
		if _, err := os.Stat(catalogPath); err == nil {
			if _, err := backupFile(catalogPath); err != nil {
				result.Message = fmt.Sprintf("Failed to back up catalog: %v", err)
				return result, nil
			}
		}
		catalogBytes, err := RenderCodexModelCatalog(models, contextWindows)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to render model catalog: %v", err)
			return result, nil
		}
		if err := os.WriteFile(catalogPath, catalogBytes, 0644); err != nil {
			result.Message = fmt.Sprintf("Failed to write catalog: %v", err)
			return result, nil
		}
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

// RenderCodexConfigTOML returns the TOML that would be written to a fresh
// ~/.codex/config.toml — i.e. the merge applied to an empty starting point.
// Used by the preview endpoint so the UI can show exactly what's pending.
func RenderCodexConfigTOML(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool) ([]byte, error) {
	catalogPathForConfig := ""
	if len(models) > 0 && writeCatalog {
		// Guard against environments where UserHomeDir returns "" with no
		// error (rare, but it makes filepath.Join emit "/.codex/..." which
		// Codex then fails to parse as AbsolutePathBuf). Better to omit the
		// field entirely than to write a broken path.
		if homeDir, err := os.UserHomeDir(); err == nil && homeDir != "" {
			catalogPathForConfig = filepath.Join(homeDir, ".codex", codexModelCatalogFile)
		}
	}
	cfg := map[string]interface{}{}
	mergeCodexConfig(cfg, baseURL, models, catalogPathForConfig, prefs)
	return tomlpkg.Marshal(cfg)
}

// mergeCodexConfig mutates cfg in place, applying tingly-managed fields while
// preserving everything else. See ApplyCodexConfig for the contract.
//
// catalogPath is the absolute path to write into `model_catalog_json`. Pass
// "" to leave that key untouched (e.g. when no models are configured — we
// don't want to point Codex at a file we never wrote).
func mergeCodexConfig(cfg map[string]interface{}, baseURL string, models []string, catalogPath string, prefs *CodexPrefs) {
	// User-tunable, whitelist-validated keys. Applied at the top level (global
	// default) and stamped into each generated profile so profiles are
	// self-contained. Converted first so it can never carry a managed key.
	coerced := prefs.toConfig()
	for k, v := range coerced {
		cfg[k] = v
	}

	// Managed fields — written after prefs so they always win, guaranteeing
	// prefs cannot clobber them (defense in depth on top of the whitelist).
	if len(models) > 0 {
		cfg["model"] = models[0]
	}
	cfg["model_provider"] = codexGatewayProviderName
	if catalogPath != "" {
		cfg["model_catalog_json"] = catalogPath
	}

	providers, _ := cfg["model_providers"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
	}
	providers[codexGatewayProviderName] = map[string]interface{}{
		"name":                  "OpenAI using Tingly Box",
		"base_url":              baseURL,
		"preferred_auth_method": "apikey",
		"wire_api":              "responses",
	}
	cfg["model_providers"] = providers

	profiles, _ := cfg["profiles"].(map[string]interface{})
	if profiles == nil {
		profiles = map[string]interface{}{}
	}
	for _, model := range models {
		profile := map[string]interface{}{
			"model":          model,
			"model_provider": codexGatewayProviderName,
		}
		for k, v := range coerced {
			profile[k] = v
		}
		profiles[sanitizeCodexProfileKey(model)] = profile
	}
	if len(profiles) > 0 {
		cfg["profiles"] = profiles
	}
}

const (
	// The gateway only knows model slugs from routing rules, not provider-native
	// capabilities. Keep tingly-managed catalog entries conservative and
	// internally consistent until richer per-model metadata is available.
	codexDefaultContextWindow            = 200000
	codexDefaultMaxContextWindow         = 200000
	codexEffectiveContextWindowPercent   = 92
	codexDefaultAutoCompactTokenLimitPct = 85

	// 1M context window for models that support it (Sonnet 4.6+, Opus 4.6+)
	codex1MContextWindow = 1000000
)

// renderCodexModelCatalog produces the JSON payload for
// ~/.codex/tingly-model-catalog.json. Each model becomes one ModelInfo entry
// with the required fields populated using conservative defaults that match
// the OpenAI Responses API surface (text-in/text-out, reasoning summaries on,
// no verbosity knob). Codex 0.124+ deserializes this into
// `protocol::openai_models::ModelsResponse`; field names and value types must
// stay in sync with that struct.
//
// The contextWindows parameter allows overriding the default context window
// for specific models (e.g., 1M context window models). If nil, uses default.
func RenderCodexModelCatalog(models []string, contextWindows map[string]int) ([]byte, error) {
	// supported_reasoning_levels is Vec<ReasoningEffortPreset>, not a bare
	// string list — each element is an {effort, description} object. Values
	// mirror Codex's bundled catalog for GPT-5 so /model shows the familiar
	// presets.
	reasoningPresets := []map[string]interface{}{
		{"effort": "minimal", "description": "Minimal reasoning for the fastest responses"},
		{"effort": "low", "description": "Fast responses with lighter reasoning"},
		{"effort": "medium", "description": "Balances speed and reasoning depth for everyday tasks"},
		{"effort": "high", "description": "Greater reasoning depth for complex problems"},
	}
	entries := make([]map[string]interface{}, 0, len(models))
	for _, model := range models {
		// Per-model override (e.g. 1M context); indexing a nil map is safe.
		contextWindow := codexDefaultContextWindow
		maxContextWindow := codexDefaultMaxContextWindow
		if cw, ok := contextWindows[model]; ok {
			contextWindow = cw
			maxContextWindow = cw
		}

		entries = append(entries, map[string]interface{}{
			"slug":                             model,
			"display_name":                     model,
			"description":                      "Tingly Box managed model",
			"supported_reasoning_levels":       reasoningPresets,
			"default_reasoning_level":          "medium",
			"shell_type":                       "shell_command",
			"visibility":                       "list",
			"supported_in_api":                 true,
			"priority":                         0,
			"base_instructions":                "",
			"supports_reasoning_summaries":     false,
			"default_reasoning_summary":        "none",
			"support_verbosity":                false,
			"truncation_policy":                map[string]interface{}{"mode": "tokens", "limit": 10000},
			"supports_parallel_tool_calls":     true,
			"context_window":                   contextWindow,
			"max_context_window":               maxContextWindow,
			"auto_compact_token_limit":         codexAutoCompactTokenLimit(contextWindow),
			"effective_context_window_percent": codexEffectiveContextWindowPercent,
			"experimental_supported_tools":     []string{},
			"input_modalities":                 []string{"text", "image"},
			"apply_patch_tool_type":            "freeform",
		})
	}
	payload := map[string]interface{}{
		"$schema": codexModelCatalogSchema,
		"models":  entries,
	}
	return json.MarshalIndent(payload, "", "  ")
}

func codexAutoCompactTokenLimit(contextWindow int) int {
	return contextWindow * codexDefaultAutoCompactTokenLimitPct / 100
}

// BuildContextWindowsFromRules maps each active Codex rule carrying the
// context_1m flag to the 1M context window. Keys are the rules' request
// models verbatim — exactly the names collectCodexRuleModels feeds into the
// catalog — so the override always lands on its catalog entry.
func BuildContextWindowsFromRules(cfg *Config) map[string]int {
	contextWindows := make(map[string]int)
	for _, rule := range cfg.GetRequestConfigs() {
		if rule.GetScenario() != typ.ScenarioCodex || !rule.Active || !rule.Flags.Context1M {
			continue
		}
		if model := strings.TrimSpace(rule.RequestModel); model != "" {
			contextWindows[model] = codex1MContextWindow
		}
	}
	return contextWindows
}

var codexProfileKeyInvalid = regexp.MustCompile(`[^A-Za-z0-9_-]`)

// sanitizeCodexProfileKey keeps alphanumerics, `_`, `-`; turns anything else
// into `-`; trims edge dashes. Empty result falls back to "tingly".
func sanitizeCodexProfileKey(name string) string {
	out := strings.Trim(codexProfileKeyInvalid.ReplaceAllString(name, "-"), "-")
	if out == "" {
		return "tingly"
	}
	return out
}

// CodexAuthMode selects how `~/.codex/auth.json` is populated.
type CodexAuthMode string

const (
	// CodexAuthAPIKey writes only `OPENAI_API_KEY` — used when codex CLI
	// should talk to tingly-box as a gateway.
	CodexAuthAPIKey CodexAuthMode = "apikey"
	// CodexAuthChatGPT exports a native ChatGPT-login auth.json so codex CLI
	// can talk to OpenAI directly using OAuth tokens previously obtained by
	// tingly-box. tingly-box does NOT refresh these tokens afterwards —
	// codex CLI owns their lifecycle from that point on.
	CodexAuthChatGPT CodexAuthMode = "chatgpt"
)

// ClearCodexGatewayConfig removes tingly-managed top-level keys from
// ~/.codex/config.toml so that when a user switches to native ChatGPT OAuth
// mode the codex CLI falls back to its own defaults rather than trying to
// route requests through the (now-unused) tingly-box gateway.
//
// Only the tingly-managed top-level fields are removed; everything else
// (other provider entries, profiles, user prefs) is left intact. The previous
// config.toml is backed up before modification.
func ClearCodexGatewayConfig() (*ApplyResult, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetPath := filepath.Join(homeDir, ".codex", "config.toml")
	result := &ApplyResult{}

	data, err := os.ReadFile(targetPath)
	if err != nil {
		// Nothing to clear — treat as success.
		result.Success = true
		result.Message = "no config.toml found, nothing to clear"
		return result, nil
	}

	// Fast path: if the file mentions neither the tingly provider name nor any
	// managed top-level key, skip the unmarshal/marshal round-trip entirely.
	// Re-marshaling user TOML loses comments and reorders keys, so avoiding it
	// in the common no-op case is also a correctness win.
	if !bytes.Contains(data, []byte(codexGatewayProviderName)) {
		result.Success = true
		result.Message = "config.toml has no tingly gateway keys, nothing to clear"
		return result, nil
	}

	cfg := map[string]interface{}{}
	if err := tomlpkg.Unmarshal(data, &cfg); err != nil {
		result.Message = fmt.Sprintf("Failed to parse config.toml: %v", err)
		return result, nil
	}

	changed := false
	for _, k := range codexGatewayTopLevelKeys {
		if _, ok := cfg[k]; ok {
			delete(cfg, k)
			changed = true
		}
	}
	// Also remove the tingly-box provider stanza if present.
	if providers, ok := cfg["model_providers"].(map[string]interface{}); ok {
		if _, ok := providers[codexGatewayProviderName]; ok {
			delete(providers, codexGatewayProviderName)
			if len(providers) == 0 {
				delete(cfg, "model_providers")
			}
			changed = true
		}
	}

	if !changed {
		result.Success = true
		result.Message = "config.toml has no tingly gateway keys, nothing to clear"
		return result, nil
	}

	backupPath, err := backupFile(targetPath)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to create backup: %v", err)
		return result, nil
	}
	result.BackupPath = backupPath

	out, err := tomlpkg.Marshal(cfg)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal TOML: %v", err)
		return result, nil
	}
	if err := os.WriteFile(targetPath, out, 0644); err != nil {
		result.Message = fmt.Sprintf("Failed to write config.toml: %v", err)
		return result, nil
	}

	result.Success = true
	result.Updated = true
	result.Message = fmt.Sprintf("Cleared tingly gateway keys from %s (backup: %s)", targetPath, backupPath)
	return result, nil
}

// CodexChatGPTTokens carries the OAuth credentials needed to materialize a
// native ChatGPT-login `auth.json`.
type CodexChatGPTTokens struct {
	IDToken      string
	AccessToken  string
	RefreshToken string
	AccountID    string
}

// ApplyCodexAuth writes `~/.codex/auth.json`. The previous version (if any) is
// backed up before modification; existing top-level keys outside the managed
// set are preserved.
//
// Mode semantics:
//   - CodexAuthAPIKey: sets `OPENAI_API_KEY` to the supplied key (gateway mode).
//   - CodexAuthChatGPT: writes `tokens` / `last_refresh` / `auth_mode: "chatgpt"`
//     and clears `OPENAI_API_KEY`. Tokens come from the caller; tingly-box does
//     not subsequently refresh them.
func ApplyCodexAuth(mode CodexAuthMode, apiKey string, tokens *CodexChatGPTTokens) (*ApplyResult, error) {
	// Validate inputs before touching disk so a malformed request can't leave
	// orphaned backups behind.
	payload := map[string]interface{}{}
	switch mode {
	case CodexAuthChatGPT:
		if tokens == nil || tokens.AccessToken == "" || tokens.RefreshToken == "" {
			return &ApplyResult{Message: "ChatGPT auth requires access_token and refresh_token"}, nil
		}
		payload["auth_mode"] = "chatgpt"
		tokensMap := map[string]interface{}{
			"access_token":  tokens.AccessToken,
			"refresh_token": tokens.RefreshToken,
		}
		if tokens.IDToken != "" {
			tokensMap["id_token"] = tokens.IDToken
		}
		if tokens.AccountID != "" {
			tokensMap["account_id"] = tokens.AccountID
		}
		payload["tokens"] = tokensMap
		payload["last_refresh"] = time.Now().UTC().Format(time.RFC3339)
	case "", CodexAuthAPIKey:
		payload["OPENAI_API_KEY"] = apiKey
	default:
		return &ApplyResult{Message: fmt.Sprintf("Unknown Codex auth mode: %q", mode)}, nil
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".codex")
	targetPath := filepath.Join(configDir, "auth.json")
	result := &ApplyResult{}

	if err := os.MkdirAll(configDir, 0755); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	// Marshal before touching disk so a malformed payload can't leave an
	// orphan backup behind.
	output, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal JSON: %v", err)
		return result, nil
	}

	// Each mode writes a fresh file — no merging with the previous auth.json.
	// Switching apikey→chatgpt must not leave OPENAI_API_KEY behind, and
	// chatgpt→apikey must not leave the tokens block behind. The backup
	// preserves whatever the user had.
	if _, err := os.Stat(targetPath); err == nil {
		backupPath, err := backupFile(targetPath)
		if err != nil {
			result.Message = fmt.Sprintf("Failed to create backup: %v", err)
			return result, nil
		}
		result.BackupPath = backupPath
		result.Updated = true
	} else {
		result.Created = true
	}

	if err := os.WriteFile(targetPath, output, 0600); err != nil {
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
