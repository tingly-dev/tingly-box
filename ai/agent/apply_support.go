package agent

// This file holds a self-contained copy of the config-file-writing primitives
// originally defined in internal/server/config/apply_config.go. It exists so
// ai/agent (an independently-versioned go module) does not need to import the
// parent repo's internal/server/config package — see
// .sdlc/docs/ai-module-decoupling-refactor-20260803.spec.md for the full
// rationale. internal/server/config keeps its own copies for
// internal/server/module/configapply, which still depends on them directly;
// the two copies are intentionally duplicated, not shared.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	tomlpkg "github.com/pelletier/go-toml/v2"
	yamlpkg "gopkg.in/yaml.v3"
)

// defaultBackupRetention is the default number of backup files to keep per
// original config file. Older backups beyond this count are removed by
// rotateBackups after each new backup is created.
const defaultBackupRetention = 3

// codexGatewayProviderName is the tingly-box provider key written into
// config.toml's [model_providers] table by mergeCodexConfig.
const codexGatewayProviderName = "tingly-box"

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

// WithExtra sets a single extra key-value pair to merge into the settings.
func WithExtra(key string, value any) ApplyOption {
	return func(opts *applyOptions) {
		if opts.extras == nil {
			opts.extras = make(map[string]any)
		}
		opts.extras[key] = value
	}
}

func resolveApplyOptions(opts ...ApplyOption) *applyOptions {
	applyOpts := &applyOptions{
		backup: true, // default: enable backup
	}
	for _, opt := range opts {
		opt(applyOpts)
	}
	return applyOpts
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
		// Mirrors internal/server/config/apply_config_claude.go: Claude Code's
		// settings-reference documents this key as nested under "permissions";
		// the legacy top-level key is also written for older CLI installs.
		existingConfig["defaultMode"] = applyOpts.defaultMode
		permissions, _ := existingConfig["permissions"].(map[string]interface{})
		if permissions == nil {
			permissions = map[string]interface{}{}
		}
		permissions["defaultMode"] = applyOpts.defaultMode
		existingConfig["permissions"] = permissions
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

// InstallStatusLineScript writes content (the tingly-statusline.sh script
// bytes) to ~/.claude/tingly-statusline.sh. The caller supplies content —
// this script talks to the running tingly-box server (TINGLY_API_URL,
// the /tingly/:scenario/statusline endpoint) and is therefore owned by the
// parent repo's internal.ScriptAssets, not duplicated here; ai/agent only
// knows how to place bytes it's handed onto disk.
// Returns the path to the installed script and whether it was newly created.
func InstallStatusLineScript(content []byte) (scriptPath string, created bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", false, fmt.Errorf("failed to get home directory: %w", err)
	}

	scriptPath = filepath.Join(homeDir, ".claude", "tingly-statusline.sh")

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

func writeManagedFileIfChanged(path string, content []byte, perm os.FileMode) (bool, error) {
	info, err := os.Lstat(path)
	created := os.IsNotExist(err)
	switch {
	case err == nil && info.Mode().IsRegular():
		current, readErr := os.ReadFile(path)
		if readErr != nil {
			return false, readErr
		}
		if bytes.Equal(current, content) {
			if info.Mode().Perm() != perm.Perm() {
				if chmodErr := os.Chmod(path, perm); chmodErr != nil {
					return false, chmodErr
				}
			}
			return false, nil
		}
	case err == nil && info.IsDir():
		return false, fmt.Errorf("managed file path is a directory: %s", path)
	case err != nil && !os.IsNotExist(err):
		return false, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return false, err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(content); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return false, err
	}
	if err := tmp.Close(); err != nil {
		return false, err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return false, err
	}
	return created, nil
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
		maps.Copy(existingProviders, newProviders)
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
	// Default reasoning effort to "medium" rather than leaving it unset — a
	// concrete, sensible default beats deferring to Codex's built-in default.
	return &CodexPrefs{ModelReasoningEffort: "medium"}
}

// toConfig converts prefs into a map of native TOML values ready to merge into
// config.toml. Empty values and invalid enum members are dropped; the bool
// field maps "true" -> true (anything else omitted).
func (p *CodexPrefs) toConfig() map[string]interface{} {
	out := map[string]interface{}{}
	if p == nil {
		return out
	}
	setEnum := func(key, val string) {
		if v, ok := codexEnumValue(key, val); ok {
			out[key] = v
		}
	}
	setEnum("model_reasoning_effort", p.ModelReasoningEffort)
	setEnum("model_reasoning_summary", p.ModelReasoningSummary)
	setEnum("model_verbosity", p.ModelVerbosity)
	if strings.TrimSpace(p.ModelSupportsReasoningSummaries) == "true" {
		out["model_supports_reasoning_summaries"] = true
	}
	return out
}

// codexEnumValue validates a value against the allowed set for an enum-typed
// CodexPrefs key (see codexEnumValues). It returns the trimmed value and true
// when it is a member, or ""/false otherwise (empty input also yields false).
// Shared by toConfig (write) and CodexPrefsFromConfig (read) so the two stay
// in lockstep — there is one definition of "what is a valid enum value".
func codexEnumValue(key, val string) (string, bool) {
	val = strings.TrimSpace(val)
	if val == "" {
		return "", false
	}
	if slices.Contains(codexEnumValues[key], val) {
		return val, true
	}
	return "", false
}

// CodexPrefsFromConfig is the inverse of (*CodexPrefs).toConfig: it extracts
// the typed, whitelisted CodexPrefs keys from a parsed config.toml top-level
// map. Only the four managed keys are read; enum values are validated (invalid
// values dropped) and the bool field is normalized back to "true"/"" so it
// round-trips through the same JSON shape the frontend edits. Keys outside the
// whitelist are ignored — a hand-edited config.toml can never smuggle extra
// fields into the prefs surface.
func CodexPrefsFromConfig(cfg map[string]interface{}) *CodexPrefs {
	prefs := &CodexPrefs{}
	// Enum-typed keys: validate against codexEnumValues (shared with toConfig)
	// so an out-of-set value is dropped rather than surfaced in the form.
	if v, ok := cfg["model_reasoning_effort"].(string); ok {
		if val, ok := codexEnumValue("model_reasoning_effort", v); ok {
			prefs.ModelReasoningEffort = val
		}
	}
	if v, ok := cfg["model_reasoning_summary"].(string); ok {
		if val, ok := codexEnumValue("model_reasoning_summary", v); ok {
			prefs.ModelReasoningSummary = val
		}
	}
	if v, ok := cfg["model_verbosity"].(string); ok {
		if val, ok := codexEnumValue("model_verbosity", v); ok {
			prefs.ModelVerbosity = val
		}
	}
	// model_supports_reasoning_summaries maps true -> "true"; anything else
	// (false, missing, non-bool) leaves it unset, matching toConfig's stance
	// that only an explicit "true" opts in. Accept both a native TOML bool and
	// the string form (older files / hand-edits).
	if v, ok := cfg["model_supports_reasoning_summaries"]; ok {
		if b, ok := v.(bool); ok && b {
			prefs.ModelSupportsReasoningSummaries = "true"
		} else if s, ok := v.(string); ok && strings.TrimSpace(s) == "true" {
			prefs.ModelSupportsReasoningSummaries = "true"
		}
	}
	return prefs
}

// isTinglyManagedCodexConfig reports whether a parsed config.toml was written
// by tingly-box. The reliable signals are an explicit
// `model_provider = "tingly-box"` or a `[model_providers.tingly-box]` stanza.
// The generic `model`/`model_catalog_json` keys are deliberately NOT signals —
// a stock codex install always has `model`, so flagging on it would mark every
// codex config as tingly-owned.
func isTinglyManagedCodexConfig(cfg map[string]interface{}) bool {
	if provider, ok := cfg["model_provider"].(string); ok && provider == codexGatewayProviderName {
		return true
	}
	if providers, ok := cfg["model_providers"].(map[string]interface{}); ok {
		if _, ok := providers[codexGatewayProviderName]; ok {
			return true
		}
	}
	return false
}

// ReadCodexConfig reads ~/.codex/config.toml and returns the typed prefs, the
// inferred writeCatalog state (true when model_catalog_json is set), and
// whether a tingly-managed config exists. A missing or unparseable file yields
// empty prefs, writeCatalog=false, exists=false.
func ReadCodexConfig() (prefs *CodexPrefs, writeCatalog bool, exists bool, err error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, false, false, fmt.Errorf("failed to get home directory: %w", err)
	}
	targetPath := filepath.Join(homeDir, ".codex", "config.toml")

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		// Missing file is not an error — first-time setup, no applied state.
		return DefaultCodexPrefs(), false, false, nil
	}

	cfg := map[string]interface{}{}
	if err := tomlpkg.Unmarshal(data, &cfg); err != nil {
		// Unparseable file: surface the error but still report non-existence so
		// the form falls back to defaults rather than showing a blank state.
		return DefaultCodexPrefs(), false, false, nil
	}

	_, hasCatalog := cfg["model_catalog_json"]
	return CodexPrefsFromConfig(cfg), hasCatalog, isTinglyManagedCodexConfig(cfg), nil
}

// ApplyCodexConfig merges tingly-box Codex settings into ~/.codex/config.toml
// and writes ~/.codex/tingly-model-catalog.json with one entry per supplied
// model so Codex's `/model` picker can see them.
//
// This is the backward-compatible version that uses default context windows.
// For context window support, use ApplyCodexConfigWithContextWindows.
func ApplyCodexConfig(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool) (*ApplyResult, error) {
	return ApplyCodexConfigWithContextWindows(baseURL, models, prefs, writeCatalog, nil, "")
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
// bearerToken (hybrid mode) is embedded into the tingly-box provider stanza as
// experimental_bearer_token; pass "" for the classic gateway path where the key
// is written to auth.json instead.
func ApplyCodexConfigWithContextWindows(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool, contextWindows map[string]int, bearerToken string) (*ApplyResult, error) {
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
	mergeCodexConfig(existing, baseURL, models, catalogPathForConfig, prefs, bearerToken)

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
func RenderCodexConfigTOML(baseURL string, models []string, prefs *CodexPrefs, writeCatalog bool, bearerToken string) ([]byte, error) {
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
	mergeCodexConfig(cfg, baseURL, models, catalogPathForConfig, prefs, bearerToken)
	return tomlpkg.Marshal(cfg)
}

// mergeCodexConfig mutates cfg in place, applying tingly-managed fields while
// preserving everything else. See ApplyCodexConfig for the contract.
//
// catalogPath is the absolute path to write into `model_catalog_json`. Pass
// "" to leave that key untouched (e.g. when no models are configured — we
// don't want to point Codex at a file we never wrote).
//
// bearerToken, when non-empty, is written into the tingly-box provider stanza
// as `experimental_bearer_token` (with `requires_openai_auth = true`). This is
// the hybrid-mode path: it keeps the gateway credential provider-scoped inside
// config.toml so ~/.codex/auth.json can retain a native ChatGPT login instead.
// Pass "" for the classic gateway path where the key lives in auth.json's
// OPENAI_API_KEY.
func mergeCodexConfig(cfg map[string]interface{}, baseURL string, models []string, catalogPath string, prefs *CodexPrefs, bearerToken string) {
	// User-tunable, whitelist-validated keys. Applied at the top level (global
	// default) and stamped into each generated profile so profiles are
	// self-contained. Converted first so it can never carry a managed key.
	coerced := prefs.toConfig()
	maps.Copy(cfg, coerced)

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
	providerStanza := map[string]interface{}{
		"name":     "OpenAI using Tingly Box",
		"base_url": baseURL,
		"wire_api": "responses",
	}
	if bearerToken != "" {
		// Hybrid: provider-scoped credential keeps the gateway token out of
		// auth.json so a native ChatGPT login can coexist there.
		// requires_openai_auth=true tells Codex this provider still requires the
		// OpenAI auth path (the credential arrives via the bearer token above).
		providerStanza["experimental_bearer_token"] = bearerToken
		providerStanza["requires_openai_auth"] = true
	} else {
		// Gateway (apikey): the token lives in ~/.codex/auth.json's
		// OPENAI_API_KEY. requires_openai_auth=true is the schema-documented way
		// to tell Codex to source this provider's credential from auth.json.
		providerStanza["requires_openai_auth"] = true
	}
	providers[codexGatewayProviderName] = providerStanza
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
		maps.Copy(profile, coerced)
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
)

// RenderCodexModelCatalog produces the JSON payload for
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
	// CodexAuthHybrid keeps requests flowing through the tingly-box gateway
	// (the gateway token lives in config.toml's provider stanza as
	// experimental_bearer_token) WHILE preserving a native ChatGPT login in
	// auth.json so Codex App still recognizes the official account (remote
	// control, plugins, account display). When ChatGPT tokens are supplied
	// they are materialized into auth.json; when absent, auth.json is left
	// untouched so an existing `codex login` survives.
	CodexAuthHybrid CodexAuthMode = "hybrid"
)

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
//   - CodexAuthHybrid: the gateway credential lives in config.toml (not here),
//     so auth.json only needs to carry the native ChatGPT login. With tokens
//     supplied it behaves like CodexAuthChatGPT; with tokens nil it is a no-op
//     that leaves any existing auth.json (e.g. a prior `codex login`) untouched.
func ApplyCodexAuth(mode CodexAuthMode, apiKey string, tokens *CodexChatGPTTokens) (*ApplyResult, error) {
	// Hybrid with no tokens: preserve whatever login already lives in auth.json.
	if mode == CodexAuthHybrid && tokens == nil {
		return &ApplyResult{Success: true, Message: "Left ~/.codex/auth.json untouched (kept existing ChatGPT login)"}, nil
	}
	// Validate inputs before touching disk so a malformed request can't leave
	// orphaned backups behind.
	payload := map[string]interface{}{}
	switch mode {
	case CodexAuthChatGPT, CodexAuthHybrid:
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

// ---------------------------------------------------------------------------
// DeepSeek Harness (dsh) settings/credentials application
//
// dsh splits its config the same way Codex does: a settings file that
// references credentials indirectly (apiKeyEnv names an env var rather than
// embedding the secret) and a separate write-only credentials file. See
// https://deepseek-harness.github.io/deepseek-harness/en/guide/providers.
// ---------------------------------------------------------------------------

// dshGatewayProviderName is the tingly-box provider key written into
// settings.yaml's `llm-pi-ai.providers` map by mergeDshSettings.
const dshGatewayProviderName = "tingly-box"

// dshAPIKeyEnvName is the env var name settings.yaml's apiKeyEnv points at,
// and the key written into .credentials.yaml. Fixed rather than
// user-configurable: it's an implementation detail of dsh's credential
// indirection, not a user-tunable knob.
const dshAPIKeyEnvName = "TINGLY_BOX_API_KEY"

// dshHomeDir resolves $DSH_HOME, falling back to ~/.dsh when unset (dsh's
// docs do not state a default; ~/.dsh follows the common single-dotdir
// convention used by similar CLIs).
func dshHomeDir() (string, error) {
	if v := strings.TrimSpace(os.Getenv("DSH_HOME")); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("failed to get home directory: %w", err)
	}
	if home == "" {
		return "", fmt.Errorf("home directory resolved to empty path")
	}
	return filepath.Join(home, ".dsh"), nil
}

// DshPrefs is the typed, user-tunable surface of settings.yaml's tingly-box
// provider entry. DefaultInput mirrors the provider-level `defaultInput` key
// ("text" or "text_image" -> [text] / [text, image]); empty omits the key so
// dsh treats the provider as text-only — the conservative default for models
// tingly-box cannot verify support vision (same stance as Codex's third-party
// defaults, see .design/codex-config.md).
type DshPrefs struct {
	DefaultInput string `json:"default_input,omitempty"`
	Protocol     string `json:"protocol,omitempty"`
}

// dshDefaultProtocol is the wire protocol dsh talks to the tingly-box
// provider with when Protocol is unset or invalid. Chosen for backward
// compatibility: it's the value this stanza always wrote before Protocol
// became user-tunable.
const dshDefaultProtocol = "openai-completions"

// dshProtocolValues lists the wire protocols dsh's llm-pi-ai adapter can
// describe with just a key, an endpoint, and headers (its `supportedProtocols()`
// — see packages/llm/llm-pi-ai/src/provider.ts in deepseek-ai/deepseek-harness).
var dshProtocolValues = []string{"openai-completions", "openai-responses", "anthropic-messages"}

// dshProtocolValue validates val against dshProtocolValues, returning the
// trimmed value and true when it is a member, or ""/false otherwise (empty
// input also yields false). Shared by toConfig (write) and DshPrefsFromConfig
// (read) so the two stay in lockstep.
func dshProtocolValue(val string) (string, bool) {
	val = strings.TrimSpace(val)
	if slices.Contains(dshProtocolValues, val) {
		return val, true
	}
	return "", false
}

// DefaultDshPrefs returns the defaults for the CLI path and no-prefs
// fallback: DefaultInput unset (text-only), Protocol defaulting to
// dshDefaultProtocol.
func DefaultDshPrefs() *DshPrefs {
	return &DshPrefs{Protocol: dshDefaultProtocol}
}

// dshDefaultInputList converts the enum value into the YAML modality list,
// or nil for an unset/invalid value (dropped, forward-compatible).
func dshDefaultInputList(val string) []string {
	switch val {
	case "text":
		return []string{"text"}
	case "text_image":
		return []string{"text", "image"}
	default:
		return nil
	}
}

// toConfig converts prefs into the provider-stanza fields it controls. Unlike
// DefaultInput, "api" is always emitted — dsh requires a protocol, so a
// nil/empty/invalid Protocol resolves to dshDefaultProtocol rather than
// omitting the key.
func (p *DshPrefs) toConfig() map[string]interface{} {
	protocol := dshDefaultProtocol
	if p != nil {
		if v, ok := dshProtocolValue(p.Protocol); ok {
			protocol = v
		}
	}
	out := map[string]interface{}{"api": protocol}
	if p == nil {
		return out
	}
	if list := dshDefaultInputList(strings.TrimSpace(p.DefaultInput)); list != nil {
		out["defaultInput"] = list
	}
	return out
}

// DshPrefsFromConfig is the inverse of toConfig: it extracts DefaultInput and
// Protocol from a parsed tingly-box provider stanza.
func DshPrefsFromConfig(providerStanza map[string]interface{}) *DshPrefs {
	prefs := &DshPrefs{}
	if v, ok := providerStanza["api"].(string); ok {
		if val, ok := dshProtocolValue(v); ok {
			prefs.Protocol = val
		}
	}
	list, ok := providerStanza["defaultInput"].([]interface{})
	if !ok || len(list) == 0 {
		return prefs
	}
	hasImage := false
	for _, v := range list {
		if s, ok := v.(string); ok && s == "image" {
			hasImage = true
		}
	}
	if hasImage {
		prefs.DefaultInput = "text_image"
	} else {
		prefs.DefaultInput = "text"
	}
	return prefs
}

// dshProviderStanza drills into cfg["llm-pi-ai"]["providers"]["tingly-box"]
// and returns it if present.
func dshProviderStanza(cfg map[string]interface{}) (map[string]interface{}, bool) {
	root, ok := cfg["llm-pi-ai"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	providers, ok := root["providers"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	stanza, ok := providers[dshGatewayProviderName].(map[string]interface{})
	if !ok {
		return nil, false
	}
	return stanza, true
}

// ReadDshSettings reads $DSH_HOME/settings.yaml and returns the typed prefs
// and whether a tingly-managed provider stanza exists. A missing or
// unparseable file yields empty prefs, exists=false, so the form falls back
// to defaults rather than erroring on first-time setup.
func ReadDshSettings() (prefs *DshPrefs, exists bool, err error) {
	dshHome, err := dshHomeDir()
	if err != nil {
		return nil, false, err
	}
	targetPath := filepath.Join(dshHome, "settings.yaml")

	data, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		return DefaultDshPrefs(), false, nil
	}

	cfg := map[string]interface{}{}
	if err := yamlpkg.Unmarshal(data, &cfg); err != nil {
		return DefaultDshPrefs(), false, nil
	}

	stanza, ok := dshProviderStanza(cfg)
	if !ok {
		return DefaultDshPrefs(), false, nil
	}
	return DshPrefsFromConfig(stanza), true, nil
}

// ApplyDshSettings merges tingly-box's provider entry into
// $DSH_HOME/settings.yaml.
//
// MERGE semantics: only the `llm-pi-ai.providers.tingly-box` stanza is
// overwritten; everything else the user has in settings.yaml — other
// providers, unrelated top-level keys — is left alone. The previous file (if
// any) is backed up before rewrite.
func ApplyDshSettings(baseURL string, models []string, prefs *DshPrefs) (*ApplyResult, error) {
	dshHome, err := dshHomeDir()
	if err != nil {
		return nil, err
	}
	targetPath := filepath.Join(dshHome, "settings.yaml")
	result := &ApplyResult{}

	if err := os.MkdirAll(dshHome, 0755); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	existing := map[string]interface{}{}
	if data, err := os.ReadFile(targetPath); err == nil {
		if err := yamlpkg.Unmarshal(data, &existing); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing settings.yaml: %v", err)
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

	mergeDshSettings(existing, baseURL, models, prefs)

	out, err := yamlpkg.Marshal(existing)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal settings.yaml: %v", err)
		return result, nil
	}
	if err := os.WriteFile(targetPath, out, 0644); err != nil {
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

// RenderDshSettingsYAML returns the YAML that would be written to a fresh
// settings.yaml — i.e. the merge applied to an empty starting point. Used by
// the preview endpoint so the UI can show exactly what's pending.
func RenderDshSettingsYAML(baseURL string, models []string, prefs *DshPrefs) ([]byte, error) {
	cfg := map[string]interface{}{}
	mergeDshSettings(cfg, baseURL, models, prefs)
	return yamlpkg.Marshal(cfg)
}

// mergeDshSettings mutates cfg in place, applying the tingly-box provider
// stanza while preserving everything else. See ApplyDshSettings for the
// contract.
func mergeDshSettings(cfg map[string]interface{}, baseURL string, models []string, prefs *DshPrefs) {
	root, _ := cfg["llm-pi-ai"].(map[string]interface{})
	if root == nil {
		root = map[string]interface{}{}
	}
	providers, _ := root["providers"].(map[string]interface{})
	if providers == nil {
		providers = map[string]interface{}{}
	}

	modelEntries := make([]map[string]interface{}, 0, len(models))
	for _, model := range models {
		modelEntries = append(modelEntries, map[string]interface{}{"id": model})
	}

	stanza := map[string]interface{}{
		"apiKeyEnv": dshAPIKeyEnvName,
		"baseURL":   baseURL,
		"models":    modelEntries,
	}
	// prefs.toConfig() always emits "api" (defaulting when unset/invalid), so
	// the stanza literal above no longer needs a hardcoded default.
	maps.Copy(stanza, prefs.toConfig())

	providers[dshGatewayProviderName] = stanza
	root["providers"] = providers
	cfg["llm-pi-ai"] = root
}

// ApplyDshCredentials writes $DSH_HOME/.credentials.yaml, setting only the
// tingly-box managed env var key; other keys already in the file (other
// providers' credentials) are preserved. The previous file (if any) is
// backed up before modification.
func ApplyDshCredentials(apiKey string) (*ApplyResult, error) {
	dshHome, err := dshHomeDir()
	if err != nil {
		return nil, err
	}
	targetPath := filepath.Join(dshHome, ".credentials.yaml")
	result := &ApplyResult{}

	if err := os.MkdirAll(dshHome, 0755); err != nil {
		result.Message = fmt.Sprintf("Failed to create directory: %v", err)
		return result, nil
	}

	existing := map[string]interface{}{}
	if data, err := os.ReadFile(targetPath); err == nil {
		if err := yamlpkg.Unmarshal(data, &existing); err != nil {
			result.Message = fmt.Sprintf("Failed to parse existing .credentials.yaml: %v", err)
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

	existing[dshAPIKeyEnvName] = apiKey

	out, err := yamlpkg.Marshal(existing)
	if err != nil {
		result.Message = fmt.Sprintf("Failed to marshal .credentials.yaml: %v", err)
		return result, nil
	}
	if err := os.WriteFile(targetPath, out, 0600); err != nil {
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
