package codeximport

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/pelletier/go-toml/v2"
)

const (
	defaultCodexDir       = ".codex"
	defaultSourceProvider = "openai"
	defaultTargetProvider = "tingly-box"

	importStateActiveKey         = "codex_import_active"
	importStateSourceProviderKey = "codex_import_source_provider"
	importStateTargetProviderKey = "codex_import_target_provider"
	importStateCodexHomeKey      = "codex_import_codex_home"
	importStateSqliteHomeKey     = "codex_import_sqlite_home"
	importStateStateDBPathKey    = "codex_import_state_db_path"
	importStateAutoUndoOnStopKey = "codex_import_auto_undo_on_stop"
)

func ImportStateActiveKey() string         { return importStateActiveKey }
func ImportStateSourceProviderKey() string { return importStateSourceProviderKey }
func ImportStateTargetProviderKey() string { return importStateTargetProviderKey }
func ImportStateCodexHomeKey() string      { return importStateCodexHomeKey }
func ImportStateSqliteHomeKey() string     { return importStateSqliteHomeKey }
func ImportStateStateDBPathKey() string    { return importStateStateDBPathKey }
func ImportStateAutoUndoOnStopKey() string { return importStateAutoUndoOnStopKey }

type Importer struct {
	now func() time.Time
}

func NewImporter() *Importer {
	return &Importer{
		now: time.Now,
	}
}

type codexConfig struct {
	ModelProvider string `toml:"model_provider"`
	SqliteHome    string `toml:"sqlite_home"`
}

type resolvedPaths struct {
	codexHome      string
	sqliteHome     string
	stateDBPath    string
	sourceProvider string
	targetProvider string
}

func (i *Importer) ImportOpenAISessions(req ImportOpenAISessionsRequest) (*ImportOpenAISessionsResponse, error) {
	paths, err := i.resolvePaths(req)
	if err != nil {
		return nil, err
	}
	if paths.sourceProvider == paths.targetProvider {
		return nil, fmt.Errorf("source provider and target provider are the same: %s", paths.sourceProvider)
	}

	includeArchived := true
	if req.IncludeArchived != nil {
		includeArchived = *req.IncludeArchived
	}
	createBackup := false
	if req.CreateBackup != nil {
		createBackup = *req.CreateBackup
	}

	result := &ImportOpenAISessionsResponse{
		Success:        true,
		CodexHome:      paths.codexHome,
		SqliteHome:     paths.sqliteHome,
		StateDBPath:    paths.stateDBPath,
		SourceProvider: paths.sourceProvider,
		TargetProvider: paths.targetProvider,
		DryRun:         req.DryRun,
	}

	sessionDirs := []struct {
		root       string
		isArchived bool
	}{
		{root: filepath.Join(paths.codexHome, "sessions")},
	}
	if includeArchived {
		sessionDirs = append(sessionDirs, struct {
			root       string
			isArchived bool
		}{root: filepath.Join(paths.codexHome, "archived_sessions"), isArchived: true})
	}

	for _, dir := range sessionDirs {
		stats, err := i.rewriteSessionDir(dir.root, paths.sourceProvider, paths.targetProvider, req.DryRun, createBackup)
		if err != nil {
			return nil, err
		}
		result.ScannedFiles += stats.scannedFiles
		result.UpdatedFiles = append(result.UpdatedFiles, stats.updatedFiles...)
		result.BackupPaths = append(result.BackupPaths, stats.backupPaths...)
		result.SkippedLockedFiles = append(result.SkippedLockedFiles, stats.skippedLockedFiles...)
		result.SkippedInvalidJSONL = append(result.SkippedInvalidJSONL, stats.skippedInvalidJSONL...)
		result.SkippedUnchangedFiles += stats.skippedUnchangedFiles
		if dir.isArchived {
			result.UpdatedArchivedFiles += stats.updatedFilesCount
		} else {
			result.UpdatedSessionFiles += stats.updatedFilesCount
		}
	}

	rowsUpdated, backupPath, err := i.updateSQLiteThreads(paths.stateDBPath, paths.sourceProvider, paths.targetProvider, req.DryRun, createBackup)
	if err != nil {
		return nil, err
	}
	result.UpdatedThreadRows = rowsUpdated
	if backupPath != "" {
		result.BackupPaths = append(result.BackupPaths, backupPath)
	}

	if req.DryRun {
		result.Message = "Dry run completed"
	} else {
		result.Message = "Codex sessions imported successfully"
	}

	return result, nil
}

type rewriteStats struct {
	scannedFiles          int
	updatedFilesCount     int
	skippedUnchangedFiles int
	updatedFiles          []string
	backupPaths           []string
	skippedLockedFiles    []string
	skippedInvalidJSONL   []string
}

func (i *Importer) rewriteSessionDir(root string, sourceProvider string, targetProvider string, dryRun bool, createBackup bool) (*rewriteStats, error) {
	stats := &rewriteStats{}
	if _, err := os.Stat(root); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return stats, nil
		}
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".jsonl" {
			return nil
		}

		stats.scannedFiles++
		updated, backupPath, err := i.rewriteSessionFile(path, sourceProvider, targetProvider, dryRun, createBackup)
		if err != nil {
			var lockedErr *lockedFileError
			if errors.As(err, &lockedErr) {
				stats.skippedLockedFiles = append(stats.skippedLockedFiles, path)
				return nil
			}
			if errors.Is(err, errInvalidJSONL) {
				stats.skippedInvalidJSONL = append(stats.skippedInvalidJSONL, path)
				return nil
			}
			return err
		}
		if !updated {
			stats.skippedUnchangedFiles++
			return nil
		}

		stats.updatedFilesCount++
		stats.updatedFiles = append(stats.updatedFiles, path)
		if backupPath != "" {
			stats.backupPaths = append(stats.backupPaths, backupPath)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan %s: %w", root, err)
	}

	return stats, nil
}

var errInvalidJSONL = errors.New("invalid jsonl session meta")

func (i *Importer) rewriteSessionFile(path string, sourceProvider string, targetProvider string, dryRun bool, createBackup bool) (bool, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return false, "", wrapLockedFileError(path, "read", err)
	}

	firstLine, remainder := splitFirstLine(data)
	if len(strings.TrimSpace(string(firstLine))) == 0 {
		return false, "", errInvalidJSONL
	}

	updatedFirstLine, changed, err := rewriteSessionProvider(firstLine, sourceProvider, targetProvider)
	if err != nil {
		return false, "", errInvalidJSONL
	}
	if !changed {
		return false, "", nil
	}
	if dryRun {
		return true, "", nil
	}

	backupPath := ""
	if createBackup {
		var err error
		backupPath, err = i.backupFile(path)
		if err != nil {
			return false, "", err
		}
	}

	info, err := os.Stat(path)
	if err != nil {
		return false, "", fmt.Errorf("stat %s: %w", path, err)
	}

	newContent := append(updatedFirstLine, remainder...)
	if err := os.WriteFile(path, newContent, info.Mode().Perm()); err != nil {
		return false, "", wrapLockedFileError(path, "write", err)
	}

	return true, backupPath, nil
}

func splitFirstLine(data []byte) ([]byte, []byte) {
	idx := bytesIndexByte(data, '\n')
	if idx < 0 {
		return data, nil
	}
	return data[:idx], data[idx:]
}

func bytesIndexByte(data []byte, b byte) int {
	for idx, current := range data {
		if current == b {
			return idx
		}
	}
	return -1
}

func rewriteSessionProvider(firstLine []byte, sourceProvider string, targetProvider string) ([]byte, bool, error) {
	var payload any
	if err := json.Unmarshal(firstLine, &payload); err != nil {
		return nil, false, err
	}
	changed := rewriteProviderFields(payload, sourceProvider, targetProvider)
	if !changed {
		return firstLine, false, nil
	}
	updated, err := json.Marshal(payload)
	if err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func rewriteProviderFields(payload any, sourceProvider string, targetProvider string) bool {
	switch typed := payload.(type) {
	case map[string]any:
		changed := false
		for key, value := range typed {
			switch key {
			case "model_provider", "model_provider_id":
				if provider, ok := value.(string); ok && provider == sourceProvider {
					typed[key] = targetProvider
					changed = true
				}
			default:
				if rewriteProviderFields(value, sourceProvider, targetProvider) {
					changed = true
				}
			}
		}
		return changed
	case []any:
		changed := false
		for _, item := range typed {
			if rewriteProviderFields(item, sourceProvider, targetProvider) {
				changed = true
			}
		}
		return changed
	default:
		return false
	}
}

func (i *Importer) updateSQLiteThreads(dbPath string, sourceProvider string, targetProvider string, dryRun bool, createBackup bool) (int64, string, error) {
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL")
	if err != nil {
		return 0, "", fmt.Errorf("open sqlite %s: %w", dbPath, err)
	}
	defer db.Close()

	var count int64
	if err := db.QueryRow(`SELECT COUNT(*) FROM threads WHERE model_provider = ?`, sourceProvider).Scan(&count); err != nil {
		return 0, "", fmt.Errorf("count threads for provider %s: %w", sourceProvider, err)
	}
	if count == 0 {
		return 0, "", nil
	}
	if dryRun {
		return count, "", nil
	}

	backupPath := ""
	if createBackup {
		var err error
		backupPath, err = i.backupSQLiteDatabase(db, dbPath)
		if err != nil {
			return 0, "", err
		}
	}

	tx, err := db.Begin()
	if err != nil {
		return 0, "", fmt.Errorf("begin sqlite transaction: %w", err)
	}

	res, err := tx.Exec(`UPDATE threads SET model_provider = ? WHERE model_provider = ?`, targetProvider, sourceProvider)
	if err != nil {
		_ = tx.Rollback()
		if isLikelySQLiteLockedError(err) {
			return 0, "", fmt.Errorf("sqlite database is locked at %s; close Codex, Codex App, or app-server and retry: %w", dbPath, err)
		}
		return 0, "", fmt.Errorf("update sqlite threads: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, "", fmt.Errorf("commit sqlite transaction: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return 0, "", fmt.Errorf("read sqlite affected rows: %w", err)
	}
	return rows, backupPath, nil
}

func (i *Importer) backupSQLiteDatabase(db *sql.DB, dbPath string) (string, error) {
	backupPath := dbPath + ".backup"
	stmt := fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteString(backupPath))
	if _, err := db.Exec(stmt); err != nil {
		if isLikelySQLiteLockedError(err) {
			return "", fmt.Errorf("sqlite database is locked at %s; close Codex, Codex App, or app-server and retry: %w", dbPath, err)
		}
		return "", fmt.Errorf("backup sqlite database %s: %w", dbPath, err)
	}
	return backupPath, nil
}

func escapeSQLiteString(input string) string {
	return strings.ReplaceAll(input, "'", "''")
}

func (i *Importer) backupFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", wrapLockedFileError(path, "read", err)
	}

	backupPath := generateBackupPath(path, i.now())
	if err := os.MkdirAll(filepath.Dir(backupPath), 0o755); err != nil {
		return "", fmt.Errorf("create backup dir for %s: %w", path, err)
	}
	if err := os.WriteFile(backupPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write backup for %s: %w", path, err)
	}
	return backupPath, nil
}

func generateBackupPath(path string, now time.Time) string {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	ext := filepath.Ext(base)
	name := strings.TrimSuffix(base, ext)
	_ = now
	return filepath.Join(dir, "backup", fmt.Sprintf("%s.bak%s", name, ext))
}

func (i *Importer) resolvePaths(req ImportOpenAISessionsRequest) (*resolvedPaths, error) {
	codexHome, err := resolveCodexHome(req.CodexHome)
	if err != nil {
		return nil, err
	}

	cfg, _ := loadCodexConfig(filepath.Join(codexHome, "config.toml"))

	sourceProvider := strings.TrimSpace(req.SourceProvider)
	if sourceProvider == "" {
		sourceProvider = defaultSourceProvider
	}

	targetProvider := strings.TrimSpace(req.TargetProvider)
	if targetProvider == "" {
		targetProvider = strings.TrimSpace(cfg.ModelProvider)
	}
	if targetProvider == "" {
		targetProvider = defaultTargetProvider
	}

	sqliteHome, err := resolveSqliteHome(req.SqliteHome, codexHome, cfg.SqliteHome)
	if err != nil {
		return nil, err
	}

	stateDBPath, err := resolveStateDBPath(req.StateDBPath, sqliteHome)
	if err != nil {
		return nil, err
	}

	return &resolvedPaths{
		codexHome:      codexHome,
		sqliteHome:     sqliteHome,
		stateDBPath:    stateDBPath,
		sourceProvider: sourceProvider,
		targetProvider: targetProvider,
	}, nil
}

func resolveCodexHome(input string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return expandPath(input)
	}
	if env := strings.TrimSpace(os.Getenv("CODEX_HOME")); env != "" {
		return expandPath(env)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, defaultCodexDir), nil
}

func resolveSqliteHome(input string, codexHome string, configSqliteHome string) (string, error) {
	switch {
	case strings.TrimSpace(input) != "":
		return expandPath(input)
	case strings.TrimSpace(configSqliteHome) != "":
		return expandPath(configSqliteHome)
	case strings.TrimSpace(os.Getenv("CODEX_SQLITE_HOME")) != "":
		return expandPath(os.Getenv("CODEX_SQLITE_HOME"))
	default:
		return codexHome, nil
	}
}

func resolveStateDBPath(input string, sqliteHome string) (string, error) {
	if strings.TrimSpace(input) != "" {
		return expandPath(input)
	}

	matches, err := filepath.Glob(filepath.Join(sqliteHome, "state_*.sqlite"))
	if err != nil {
		return "", fmt.Errorf("discover sqlite db in %s: %w", sqliteHome, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no state_*.sqlite found under %s", sqliteHome)
	}

	sort.Slice(matches, func(i, j int) bool {
		return stateVersion(matches[i]) > stateVersion(matches[j])
	})
	return matches[0], nil
}

func stateVersion(path string) int {
	base := filepath.Base(path)
	trimmed := strings.TrimSuffix(strings.TrimPrefix(base, "state_"), ".sqlite")
	version, err := strconv.Atoi(trimmed)
	if err != nil {
		return -1
	}
	return version
}

func expandPath(path string) (string, error) {
	if path == "" {
		return "", nil
	}
	path = expandWindowsEnvVars(os.ExpandEnv(path))
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home: %w", err)
		}
		if path == "~" {
			return home, nil
		}
		return filepath.Join(home, strings.TrimPrefix(path, "~/")), nil
	}
	return filepath.Clean(path), nil
}

var windowsEnvVarPattern = regexp.MustCompile(`%([^%]+)%`)

func expandWindowsEnvVars(path string) string {
	return windowsEnvVarPattern.ReplaceAllStringFunc(path, func(match string) string {
		key := strings.Trim(match, "%")
		if key == "" {
			return match
		}
		value, ok := os.LookupEnv(key)
		if !ok || value == "" {
			return match
		}
		return value
	})
}

func loadCodexConfig(path string) (*codexConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return &codexConfig{}, err
	}
	var cfg codexConfig
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return &codexConfig{}, err
	}
	return &cfg, nil
}

type lockedFileError struct {
	path  string
	op    string
	cause error
}

func (e *lockedFileError) Error() string {
	return fmt.Sprintf("%s %s: %v", e.op, e.path, e.cause)
}

func (e *lockedFileError) Unwrap() error {
	return e.cause
}

func wrapLockedFileError(path string, op string, err error) error {
	if isLikelyLockedFileError(err) {
		return &lockedFileError{path: path, op: op, cause: err}
	}
	return fmt.Errorf("%s %s: %w", op, path, err)
}

func isLikelyLockedFileError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	message := strings.ToLower(err.Error())
	lockMarkers := []string{
		"used by another process",
		"being used by another process",
		"sharing violation",
		"permission denied",
		"access is denied",
	}
	for _, marker := range lockMarkers {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}

func isLikelySQLiteLockedError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "database is locked") ||
		strings.Contains(message, "database table is locked") ||
		strings.Contains(message, "database is busy")
}
