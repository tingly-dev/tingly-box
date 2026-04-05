package codeximport

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestImportOpenAISessions(t *testing.T) {
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "codex-home")
	sqliteHome := filepath.Join(tmpDir, "sqlite-home")
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(codexHome, "archived_sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sqliteHome, 0o755); err != nil {
		t.Fatal(err)
	}

	config := []byte("model_provider = \"tingly-box\"\nsqlite_home = \"" + sqliteHome + "\"\n")
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(codexHome, "sessions", "thread.jsonl")
	if err := writeSessionFile(sessionFile, map[string]any{
		"kind":              "session_start",
		"model_provider_id": "openai",
		"message":           "hello",
	}); err != nil {
		t.Fatal(err)
	}

	archivedFile := filepath.Join(codexHome, "archived_sessions", "archived.jsonl")
	if err := writeSessionFile(archivedFile, map[string]any{
		"type": "session_meta",
		"meta": map[string]any{
			"model_provider": "openai",
		},
	}); err != nil {
		t.Fatal(err)
	}

	state4 := filepath.Join(sqliteHome, "state_4.sqlite")
	if err := createThreadsDB(state4, []string{"openai"}); err != nil {
		t.Fatal(err)
	}
	state5 := filepath.Join(sqliteHome, "state_5.sqlite")
	if err := createThreadsDB(state5, []string{"openai", "other"}); err != nil {
		t.Fatal(err)
	}

	importer := NewImporter()
	importer.now = func() time.Time {
		return time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	}

	result, err := importer.ImportOpenAISessions(ImportOpenAISessionsRequest{
		CodexHome: codexHome,
	})
	if err != nil {
		t.Fatalf("import failed: %v", err)
	}

	if result.TargetProvider != "tingly-box" {
		t.Fatalf("expected target provider tingly-box, got %s", result.TargetProvider)
	}
	if result.StateDBPath != state5 {
		t.Fatalf("expected highest version sqlite path %s, got %s", state5, result.StateDBPath)
	}
	if result.UpdatedSessionFiles != 1 {
		t.Fatalf("expected 1 updated session file, got %d", result.UpdatedSessionFiles)
	}
	if result.UpdatedArchivedFiles != 1 {
		t.Fatalf("expected 1 updated archived file, got %d", result.UpdatedArchivedFiles)
	}
	if result.UpdatedThreadRows != 1 {
		t.Fatalf("expected 1 updated sqlite row, got %d", result.UpdatedThreadRows)
	}
	if len(result.BackupPaths) != 0 {
		t.Fatalf("expected no backup paths by default, got %d", len(result.BackupPaths))
	}

	assertFirstLineProvider(t, sessionFile, "model_provider_id", "tingly-box")
	assertNestedProvider(t, archivedFile, "tingly-box")
	assertDBProviders(t, state5, []string{"other", "tingly-box"})
}

func TestImportOpenAISessionsDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := []byte("model_provider = \"tingly-box\"\n")
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), config, 0o644); err != nil {
		t.Fatal(err)
	}

	sessionFile := filepath.Join(codexHome, "sessions", "thread.jsonl")
	if err := writeSessionFile(sessionFile, map[string]any{
		"kind":              "session_start",
		"model_provider_id": "openai",
	}); err != nil {
		t.Fatal(err)
	}

	stateDB := filepath.Join(codexHome, "state_5.sqlite")
	if err := createThreadsDB(stateDB, []string{"openai"}); err != nil {
		t.Fatal(err)
	}

	result, err := NewImporter().ImportOpenAISessions(ImportOpenAISessionsRequest{
		CodexHome: codexHome,
		DryRun:    true,
	})
	if err != nil {
		t.Fatalf("dry run failed: %v", err)
	}
	if result.UpdatedSessionFiles != 1 {
		t.Fatalf("expected dry run to report one updated session file, got %d", result.UpdatedSessionFiles)
	}
	if result.UpdatedThreadRows != 1 {
		t.Fatalf("expected dry run to report one sqlite row, got %d", result.UpdatedThreadRows)
	}
	if len(result.BackupPaths) != 0 {
		t.Fatalf("expected no backups in dry run, got %d", len(result.BackupPaths))
	}

	assertFirstLineProvider(t, sessionFile, "model_provider_id", "openai")
	assertDBProviders(t, stateDB, []string{"openai"})
}

func TestImportOpenAISessionsWithBackupEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "codex-home")
	if err := os.MkdirAll(filepath.Join(codexHome, "sessions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("model_provider = \"tingly-box\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(codexHome, "sessions", "thread.jsonl")
	if err := writeSessionFile(sessionFile, map[string]any{
		"kind":              "session_start",
		"model_provider_id": "openai",
	}); err != nil {
		t.Fatal(err)
	}
	stateDB := filepath.Join(codexHome, "state_5.sqlite")
	if err := createThreadsDB(stateDB, []string{"openai"}); err != nil {
		t.Fatal(err)
	}

	enableBackup := true
	importer := NewImporter()
	importer.now = func() time.Time {
		return time.Date(2026, 4, 5, 12, 0, 0, 0, time.UTC)
	}
	result, err := importer.ImportOpenAISessions(ImportOpenAISessionsRequest{
		CodexHome:    codexHome,
		CreateBackup: &enableBackup,
	})
	if err != nil {
		t.Fatalf("import with backup failed: %v", err)
	}
	if len(result.BackupPaths) != 2 {
		t.Fatalf("expected 2 backup paths with backup enabled, got %d", len(result.BackupPaths))
	}
	expectedSessionBackup := filepath.Join(filepath.Dir(sessionFile), "backup", "thread.bak.jsonl")
	expectedDBBackup := stateDB + ".backup"
	if result.BackupPaths[0] != expectedSessionBackup && result.BackupPaths[1] != expectedSessionBackup {
		t.Fatalf("expected session backup path %s, got %v", expectedSessionBackup, result.BackupPaths)
	}
	if result.BackupPaths[0] != expectedDBBackup && result.BackupPaths[1] != expectedDBBackup {
		t.Fatalf("expected db backup path %s, got %v", expectedDBBackup, result.BackupPaths)
	}
}

func TestResolvePathsSupportsWindowsEnvStyleVariables(t *testing.T) {
	tmpDir := t.TempDir()
	codexHome := filepath.Join(tmpDir, "codex-home")
	sqliteHome := filepath.Join(tmpDir, "sqlite-home")
	if err := os.MkdirAll(codexHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sqliteHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(codexHome, "config.toml"), []byte("model_provider = \"tingly-box\"\nsqlite_home = \"%CODEX_SQLITE_TEST_HOME%\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stateDB := filepath.Join(sqliteHome, "state_5.sqlite")
	if err := createThreadsDB(stateDB, []string{"openai"}); err != nil {
		t.Fatal(err)
	}

	t.Setenv("CODEX_TEST_HOME", codexHome)
	t.Setenv("CODEX_SQLITE_TEST_HOME", sqliteHome)

	paths, err := NewImporter().resolvePaths(ImportOpenAISessionsRequest{
		CodexHome:   `%CODEX_TEST_HOME%`,
		StateDBPath: `%CODEX_SQLITE_TEST_HOME%/state_5.sqlite`,
	})
	if err != nil {
		t.Fatalf("resolvePaths failed: %v", err)
	}

	if paths.codexHome != codexHome {
		t.Fatalf("expected codexHome %s, got %s", codexHome, paths.codexHome)
	}
	if paths.sqliteHome != sqliteHome {
		t.Fatalf("expected sqliteHome %s, got %s", sqliteHome, paths.sqliteHome)
	}
	if paths.stateDBPath != stateDB {
		t.Fatalf("expected state db path %s, got %s", stateDB, paths.stateDBPath)
	}
}

func writeSessionFile(path string, firstLine map[string]any) error {
	data, err := json.Marshal(firstLine)
	if err != nil {
		return err
	}
	content := append(data, []byte("\n{\"kind\":\"event\"}\n")...)
	return os.WriteFile(path, content, 0o644)
}

func createThreadsDB(path string, providers []string) error {
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE threads (id TEXT PRIMARY KEY, model_provider TEXT NOT NULL)`); err != nil {
		return err
	}
	for idx, provider := range providers {
		if _, err := db.Exec(`INSERT INTO threads (id, model_provider) VALUES (?, ?)`, strconv.Itoa(idx), provider); err != nil {
			return err
		}
	}
	return nil
}

func assertFirstLineProvider(t *testing.T, path string, key string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	firstLine, _ := splitFirstLine(data)
	var payload map[string]any
	if err := json.Unmarshal(firstLine, &payload); err != nil {
		t.Fatal(err)
	}
	if payload[key] != expected {
		t.Fatalf("expected %s=%s, got %#v", key, expected, payload[key])
	}
}

func assertNestedProvider(t *testing.T, path string, expected string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	firstLine, _ := splitFirstLine(data)
	var payload map[string]any
	if err := json.Unmarshal(firstLine, &payload); err != nil {
		t.Fatal(err)
	}
	meta, ok := payload["meta"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested meta object")
	}
	if meta["model_provider"] != expected {
		t.Fatalf("expected nested model_provider=%s, got %#v", expected, meta["model_provider"])
	}
}

func assertDBProviders(t *testing.T, path string, expected []string) {
	t.Helper()
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT model_provider FROM threads ORDER BY model_provider`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	var providers []string
	for rows.Next() {
		var provider string
		if err := rows.Scan(&provider); err != nil {
			t.Fatal(err)
		}
		providers = append(providers, provider)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}

	sort.Strings(expected)
	if len(providers) != len(expected) {
		t.Fatalf("expected %d providers, got %d (%v)", len(expected), len(providers), providers)
	}
	for idx := range expected {
		if providers[idx] != expected[idx] {
			t.Fatalf("expected providers %v, got %v", expected, providers)
		}
	}
}
