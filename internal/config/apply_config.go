package config

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// defaultBackupRetention is the default number of backup files to keep per
// original config file. Older backups beyond this count are removed by
// rotateBackups after each new backup is created.
const defaultBackupRetention = 3

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
	backup                bool
	retention             int
	extras                map[string]any
	defaultMode           string
	showThinkingSummaries *bool
	coAuthoredBy          *bool
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

// WithShowThinkingSummaries sets the Claude Code top-level showThinkingSummaries
// value in settings.json.
func WithShowThinkingSummaries(show bool) ApplyOption {
	return func(opts *applyOptions) {
		opts.showThinkingSummaries = &show
	}
}

// WithCoAuthoredBy sets whether Claude Code credits itself in commits and PRs.
// One choice writes two keys: the legacy top-level includeCoAuthoredBy and the
// newer attribution object that replaces it (see buildClaudeSettings).
func WithCoAuthoredBy(include bool) ApplyOption {
	return func(opts *applyOptions) {
		opts.coAuthoredBy = &include
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

func resolveApplyOptions(opts ...ApplyOption) *applyOptions {
	applyOpts := &applyOptions{
		backup: true, // default: enable backup
	}
	for _, opt := range opts {
		opt(applyOpts)
	}
	return applyOpts
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
