package bot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// Settings represents bot configuration with platform-specific auth
type Settings struct {
	UUID          string            `json:"uuid,omitempty"`
	Name          string            `json:"name,omitempty"`           // User-defined name for the bot
	Token         string            `json:"token,omitempty"`          // Legacy: for backward compatibility
	Platform      string            `json:"platform"`                 // Platform identifier
	AuthType      string            `json:"auth_type"`                // Auth type: token, oauth, qr
	Auth          map[string]string `json:"auth"`                     // Dynamic auth fields based on platform
	ProxyURL      string            `json:"proxy_url,omitempty"`      // Optional proxy URL
	ChatIDLock    string            `json:"chat_id,omitempty"`        // Optional chat ID lock
	BashAllowlist []string          `json:"bash_allowlist,omitempty"` // Optional bash command allowlist
	Enabled       bool              `json:"enabled"`                  // Whether this bot is enabled
	CreatedAt     string            `json:"created_at,omitempty"`
	UpdatedAt     string            `json:"updated_at,omitempty"`
}

type Store struct {
	db        *sql.DB
	chatStore *ChatStore
}

func NewStore(dbPath string) (*Store, error) {
	if strings.TrimSpace(dbPath) == "" {
		return nil, fmt.Errorf("db path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	chatStore, err := NewChatStore(db)
	if err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db, chatStore: chatStore}, nil
}

// ChatStore returns the chat store
func (s *Store) ChatStore() *ChatStore {
	return s.chatStore
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB returns the underlying database connection
func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func initSchema(db *sql.DB) error {
	// Create bot credentials table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS remote_coder_bot_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			telegram_token TEXT,
			platform TEXT,
			proxy_url TEXT,
			chat_id_lock TEXT,
			bash_allowlist TEXT,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS remote_coder_bot_settings_v2 (
			uuid TEXT PRIMARY KEY,
			name TEXT,
			platform TEXT NOT NULL,
			auth_type TEXT,
			auth_config TEXT,
			proxy_url TEXT,
			chat_id_lock TEXT,
			bash_allowlist TEXT,
			enabled INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		-- New unified chat table
		CREATE TABLE IF NOT EXISTS remote_coder_chats (
			chat_id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			project_path TEXT,
			owner_id TEXT,
			session_id TEXT,
			is_whitelisted INTEGER DEFAULT 0,
			whitelisted_by TEXT,
			bash_cwd TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		-- Legacy tables (kept for migration)
		CREATE TABLE IF NOT EXISTS remote_coder_bot_sessions (
			chat_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS remote_coder_bot_bash_cwd (
			chat_id TEXT PRIMARY KEY,
			cwd TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS remote_coder_projects (
			id TEXT PRIMARY KEY,
			path TEXT NOT NULL,
			name TEXT,
			owner_id TEXT NOT NULL,
			platform TEXT NOT NULL,
			bot_uuid TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(path, bot_uuid)
		);

		CREATE TABLE IF NOT EXISTS remote_coder_group_bindings (
			id TEXT PRIMARY KEY,
			group_id TEXT NOT NULL,
			platform TEXT NOT NULL,
			project_id TEXT NOT NULL,
			bot_uuid TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(group_id, platform)
		);

		CREATE TABLE IF NOT EXISTS remote_coder_group_whitelist (
			group_id TEXT PRIMARY KEY,
			platform TEXT NOT NULL,
			added_by TEXT NOT NULL,
			created_at TEXT NOT NULL
		);
	`)
	if err != nil {
		return err
	}

	// Add missing columns to legacy tables
	if err := ensureColumn(db, "remote_coder_bot_settings", "platform", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "proxy_url", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "chat_id_lock", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "bash_allowlist", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "auth_type", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "auth_config", "TEXT"); err != nil {
		return err
	}
	if err := ensureColumn(db, "remote_coder_bot_settings", "name", "TEXT"); err != nil {
		return err
	}

	// Create indexes for new chats table
	indexes := []string{
		`CREATE INDEX IF NOT EXISTS idx_chats_platform ON remote_coder_chats(platform)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_owner ON remote_coder_chats(owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_chats_session ON remote_coder_chats(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_path ON remote_coder_projects(path)`,
		`CREATE INDEX IF NOT EXISTS idx_projects_owner ON remote_coder_projects(owner_id)`,
		`CREATE INDEX IF NOT EXISTS idx_group_bindings_group ON remote_coder_group_bindings(group_id, platform)`,
	}
	for _, idx := range indexes {
		if _, err := db.Exec(idx); err != nil {
			return err
		}
	}

	// Migrate data from v1 to v2 if needed
	if err := migrateV1ToV2(db); err != nil {
		return err
	}

	// Migrate data to new unified chats table
	if err := migrateToChatsTable(db); err != nil {
		return err
	}

	return nil
}

// migrateToChatsTable migrates data from legacy tables to the new unified chats table
func migrateToChatsTable(db *sql.DB) error {
	// Check if migration already done by checking if chats table has data
	var chatsCount int
	row := db.QueryRow(`SELECT COUNT(*) FROM remote_coder_chats`)
	if err := row.Scan(&chatsCount); err != nil {
		return nil // Table doesn't exist yet, will be created
	}
	if chatsCount > 0 {
		return nil // Already migrated
	}

	now := time.Now().UTC().Format(time.RFC3339)

	// 1. Migrate sessions (chat_id -> session_id)
	rows, err := db.Query(`SELECT chat_id, session_id FROM remote_coder_bot_sessions`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var chatID, sessionID string
			if err := rows.Scan(&chatID, &sessionID); err != nil {
				continue
			}
			_, _ = db.Exec(`
				INSERT INTO remote_coder_chats (chat_id, platform, session_id, created_at, updated_at)
				VALUES (?, 'telegram', ?, ?, ?)
				ON CONFLICT(chat_id) DO UPDATE SET session_id = excluded.session_id
			`, chatID, sessionID, now, now)
		}
	}

	// 2. Migrate bash cwd
	rows, err = db.Query(`SELECT chat_id, cwd FROM remote_coder_bot_bash_cwd`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var chatID, cwd string
			if err := rows.Scan(&chatID, &cwd); err != nil {
				continue
			}
			_, _ = db.Exec(`
				INSERT INTO remote_coder_chats (chat_id, platform, bash_cwd, created_at, updated_at)
				VALUES (?, 'telegram', ?, ?, ?)
				ON CONFLICT(chat_id) DO UPDATE SET bash_cwd = excluded.bash_cwd
			`, chatID, cwd, now, now)
		}
	}

	// 3. Migrate group whitelist
	rows, err = db.Query(`SELECT group_id, platform, added_by FROM remote_coder_group_whitelist`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var groupID, platform, addedBy string
			if err := rows.Scan(&groupID, &platform, &addedBy); err != nil {
				continue
			}
			_, _ = db.Exec(`
				INSERT INTO remote_coder_chats (chat_id, platform, is_whitelisted, whitelisted_by, created_at, updated_at)
				VALUES (?, ?, 1, ?, ?, ?)
				ON CONFLICT(chat_id) DO UPDATE SET is_whitelisted = 1, whitelisted_by = excluded.whitelisted_by
			`, groupID, platform, addedBy, now, now)
		}
	}

	// 4. Migrate project bindings (group bindings)
	rows, err = db.Query(`
		SELECT b.group_id, b.platform, p.path, p.owner_id
		FROM remote_coder_group_bindings b
		JOIN remote_coder_projects p ON b.project_id = p.id
	`)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var groupID, platform, projectPath, ownerID string
			if err := rows.Scan(&groupID, &platform, &projectPath, &ownerID); err != nil {
				continue
			}
			_, _ = db.Exec(`
				INSERT INTO remote_coder_chats (chat_id, platform, project_path, owner_id, created_at, updated_at)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(chat_id) DO UPDATE SET project_path = excluded.project_path, owner_id = excluded.owner_id
			`, groupID, platform, projectPath, ownerID, now, now)
		}
	}

	return nil
}

// migrateV1ToV2 migrates data from the old single-row table to the new multi-row table
func migrateV1ToV2(db *sql.DB) error {
	// Check if v2 table has any data
	var v2Count int
	row := db.QueryRow(`SELECT COUNT(*) FROM remote_coder_bot_settings_v2`)
	if err := row.Scan(&v2Count); err != nil {
		return err
	}
	if v2Count > 0 {
		return nil // Already migrated
	}

	// Check if v1 table has data
	var v1Count int
	row = db.QueryRow(`SELECT COUNT(*) FROM remote_coder_bot_settings`)
	if err := row.Scan(&v1Count); err != nil {
		return nil // v1 table doesn't exist or is empty
	}
	if v1Count == 0 {
		return nil // Nothing to migrate
	}

	// Get data from v1
	row = db.QueryRow(`SELECT telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist, auth_type, auth_config, name, updated_at FROM remote_coder_bot_settings WHERE id = 1`)
	var token, platform, proxyURL, chatIDLock, bashAllowlist, authType, authConfig, name, updatedAt sql.NullString
	if err := row.Scan(&token, &platform, &proxyURL, &chatIDLock, &bashAllowlist, &authType, &authConfig, &name, &updatedAt); err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}

	// Only migrate if there's actual data
	if !token.Valid && !platform.Valid && !authConfig.Valid {
		return nil
	}

	// Generate UUID for migrated record
	newUUID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)
	if updatedAt.Valid && updatedAt.String != "" {
		now = updatedAt.String
	}

	// Insert into v2
	_, err := db.Exec(`
		INSERT INTO remote_coder_bot_settings_v2 (uuid, name, platform, auth_type, auth_config, proxy_url, chat_id_lock, bash_allowlist, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?)
	`, newUUID, name.String, platform.String, authType.String, authConfig.String, proxyURL.String, chatIDLock.String, bashAllowlist.String, now, now)

	return err
}

func (s *Store) GetSettings() (Settings, error) {
	settings := Settings{
		Auth: make(map[string]string),
	}
	if s == nil || s.db == nil {
		return settings, nil
	}

	row := s.db.QueryRow(`SELECT telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist, auth_type, auth_config, name FROM remote_coder_bot_settings WHERE id = 1`)
	var token sql.NullString
	var platform sql.NullString
	var proxyURL sql.NullString
	var chatIDLock sql.NullString
	var bashAllowlist sql.NullString
	var authType sql.NullString
	var authConfig sql.NullString
	var name sql.NullString
	if err := row.Scan(&token, &platform, &proxyURL, &chatIDLock, &bashAllowlist, &authType, &authConfig, &name); err != nil {
		if err != sql.ErrNoRows {
			return settings, err
		}
	} else {
		// Handle name
		if name.Valid {
			settings.Name = name.String
		}

		// Handle platform
		if platform.Valid {
			settings.Platform = platform.String
		}

		// Handle legacy token field - migrate to auth map if auth_config is empty
		if token.Valid {
			settings.Token = token.String
			// For backward compatibility: if auth_config is empty, populate auth map from token
			if !authConfig.Valid || authConfig.String == "" {
				settings.AuthType = "token"
				settings.Auth["token"] = token.String
			}
		}

		// Handle proxy URL
		if proxyURL.Valid {
			settings.ProxyURL = proxyURL.String
		}

		// Handle chat ID lock
		if chatIDLock.Valid {
			settings.ChatIDLock = chatIDLock.String
		}

		// Handle bash allowlist
		if bashAllowlist.Valid && bashAllowlist.String != "" {
			_ = json.Unmarshal([]byte(bashAllowlist.String), &settings.BashAllowlist)
		}

		// Handle new auth fields
		if authType.Valid {
			settings.AuthType = authType.String
		}
		if authConfig.Valid && authConfig.String != "" {
			_ = json.Unmarshal([]byte(authConfig.String), &settings.Auth)
		}
	}

	return settings, nil
}

func (s *Store) SaveSettings(settings Settings) error {
	if s == nil || s.db == nil {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	allowlistJSON := ""
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			allowlistJSON = string(b)
		}
	}

	authConfigJSON := ""
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			authConfigJSON = string(b)
		}
	}

	// For backward compatibility: also store token in legacy field if using token auth
	legacyToken := settings.Token
	if settings.AuthType == "token" && legacyToken == "" {
		legacyToken = settings.Auth["token"]
	}

	_, err = tx.Exec(`
		INSERT INTO remote_coder_bot_settings (id, telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist, auth_type, auth_config, name, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			telegram_token = excluded.telegram_token,
			platform = excluded.platform,
			proxy_url = excluded.proxy_url,
			chat_id_lock = excluded.chat_id_lock,
			bash_allowlist = excluded.bash_allowlist,
			auth_type = excluded.auth_type,
			auth_config = excluded.auth_config,
			name = excluded.name,
			updated_at = excluded.updated_at
	`, legacyToken, settings.Platform, settings.ProxyURL, settings.ChatIDLock, allowlistJSON, settings.AuthType, authConfigJSON, settings.Name, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

// ============== V2 CRUD Methods ==============

// ListSettings returns all bot configurations from v2 table
func (s *Store) ListSettings() ([]Settings, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`SELECT uuid, name, platform, auth_type, auth_config, proxy_url, chat_id_lock, bash_allowlist, enabled, created_at, updated_at FROM remote_coder_bot_settings_v2 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Settings
	for rows.Next() {
		setting, err := scanSettings(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

// ListEnabledSettings returns all enabled bot configurations
func (s *Store) ListEnabledSettings() ([]Settings, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`SELECT uuid, name, platform, auth_type, auth_config, proxy_url, chat_id_lock, bash_allowlist, enabled, created_at, updated_at FROM remote_coder_bot_settings_v2 WHERE enabled = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var settings []Settings
	for rows.Next() {
		setting, err := scanSettings(rows)
		if err != nil {
			return nil, err
		}
		settings = append(settings, setting)
	}
	return settings, rows.Err()
}

// GetSettingsByUUID returns a single bot configuration by UUID
func (s *Store) GetSettingsByUUID(uuid string) (Settings, error) {
	if s == nil || s.db == nil {
		return Settings{Auth: make(map[string]string)}, nil
	}

	row := s.db.QueryRow(`SELECT uuid, name, platform, auth_type, auth_config, proxy_url, chat_id_lock, bash_allowlist, enabled, created_at, updated_at FROM remote_coder_bot_settings_v2 WHERE uuid = ?`, uuid)
	setting, err := scanSettingsRow(row)
	if err == sql.ErrNoRows {
		return Settings{Auth: make(map[string]string)}, nil
	}
	return setting, err
}

// CreateSettings creates a new bot configuration
func (s *Store) CreateSettings(settings Settings) (Settings, error) {
	if s == nil || s.db == nil {
		return Settings{Auth: make(map[string]string)}, nil
	}

	if settings.UUID == "" {
		settings.UUID = uuid.New().String()
	}

	now := time.Now().UTC().Format(time.RFC3339)
	settings.CreatedAt = now
	settings.UpdatedAt = now

	allowlistJSON := ""
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			allowlistJSON = string(b)
		}
	}

	authConfigJSON := ""
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			authConfigJSON = string(b)
		}
	}

	enabled := 0
	if settings.Enabled {
		enabled = 1
	}

	_, err := s.db.Exec(`
		INSERT INTO remote_coder_bot_settings_v2 (uuid, name, platform, auth_type, auth_config, proxy_url, chat_id_lock, bash_allowlist, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, settings.UUID, settings.Name, settings.Platform, settings.AuthType, authConfigJSON, settings.ProxyURL, settings.ChatIDLock, allowlistJSON, enabled, settings.CreatedAt, settings.UpdatedAt)
	if err != nil {
		return Settings{Auth: make(map[string]string)}, err
	}

	// Also save to legacy table for backward compatibility
	if settings.AuthType == "token" && len(settings.Auth) > 0 {
		legacyToken := settings.Auth["token"]
		_, _ = s.db.Exec(`
			INSERT INTO remote_coder_bot_settings (id, telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist, auth_type, auth_config, name, updated_at)
			VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET
				telegram_token = excluded.telegram_token,
				platform = excluded.platform,
				proxy_url = excluded.proxy_url,
				chat_id_lock = excluded.chat_id_lock,
				bash_allowlist = excluded.bash_allowlist,
				auth_type = excluded.auth_type,
				auth_config = excluded.auth_config,
				name = excluded.name,
				updated_at = excluded.updated_at
		`, legacyToken, settings.Platform, settings.ProxyURL, settings.ChatIDLock, allowlistJSON, settings.AuthType, authConfigJSON, settings.Name, settings.UpdatedAt)
	}

	return settings, nil
}

// UpdateSettings updates an existing bot configuration
func (s *Store) UpdateSettings(uuid string, settings Settings) error {
	if s == nil || s.db == nil {
		return nil
	}

	now := time.Now().UTC().Format(time.RFC3339)
	settings.UpdatedAt = now

	allowlistJSON := ""
	if len(settings.BashAllowlist) > 0 {
		if b, err := json.Marshal(settings.BashAllowlist); err == nil {
			allowlistJSON = string(b)
		}
	}

	authConfigJSON := ""
	if len(settings.Auth) > 0 {
		if b, err := json.Marshal(settings.Auth); err == nil {
			authConfigJSON = string(b)
		}
	}

	enabled := 0
	if settings.Enabled {
		enabled = 1
	}

	result, err := s.db.Exec(`
		UPDATE remote_coder_bot_settings_v2 SET
			name = ?, platform = ?, auth_type = ?, auth_config = ?, proxy_url = ?, chat_id_lock = ?, bash_allowlist = ?, enabled = ?, updated_at = ?
		WHERE uuid = ?
	`, settings.Name, settings.Platform, settings.AuthType, authConfigJSON, settings.ProxyURL, settings.ChatIDLock, allowlistJSON, enabled, settings.UpdatedAt, uuid)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("bot settings with uuid %s not found", uuid)
	}

	return nil
}

// DeleteSettings deletes a bot configuration
func (s *Store) DeleteSettings(uuid string) error {
	if s == nil || s.db == nil {
		return nil
	}

	result, err := s.db.Exec(`DELETE FROM remote_coder_bot_settings_v2 WHERE uuid = ?`, uuid)
	if err != nil {
		return err
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("bot settings with uuid %s not found", uuid)
	}

	return nil
}

// ToggleSettings toggles the enabled status of a bot configuration
func (s *Store) ToggleSettings(uuid string) (bool, error) {
	if s == nil || s.db == nil {
		return false, nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Get current status
	row := tx.QueryRow(`SELECT enabled FROM remote_coder_bot_settings_v2 WHERE uuid = ?`, uuid)
	var currentEnabled int
	if err := row.Scan(&currentEnabled); err != nil {
		if err == sql.ErrNoRows {
			return false, fmt.Errorf("bot settings with uuid %s not found", uuid)
		}
		return false, err
	}

	newEnabled := 0
	if currentEnabled == 0 {
		newEnabled = 1
	}

	now := time.Now().UTC().Format(time.RFC3339)
	_, err = tx.Exec(`UPDATE remote_coder_bot_settings_v2 SET enabled = ?, updated_at = ? WHERE uuid = ?`, newEnabled, now, uuid)
	if err != nil {
		return false, err
	}

	if err = tx.Commit(); err != nil {
		return false, err
	}

	return newEnabled == 1, nil
}

// scanSettings is a helper to scan a row into Settings
func scanSettings(rows *sql.Rows) (Settings, error) {
	var setting Settings
	var uuid, name, platform, authType, authConfig, proxyURL, chatIDLock, bashAllowlist, createdAt, updatedAt sql.NullString
	var enabled int

	if err := rows.Scan(&uuid, &name, &platform, &authType, &authConfig, &proxyURL, &chatIDLock, &bashAllowlist, &enabled, &createdAt, &updatedAt); err != nil {
		return setting, err
	}

	setting.Auth = make(map[string]string)
	setting.UUID = uuid.String
	setting.Name = name.String
	setting.Platform = platform.String
	setting.AuthType = authType.String
	setting.ProxyURL = proxyURL.String
	setting.ChatIDLock = chatIDLock.String
	setting.Enabled = enabled == 1
	setting.CreatedAt = createdAt.String
	setting.UpdatedAt = updatedAt.String

	if authConfig.Valid && authConfig.String != "" {
		_ = json.Unmarshal([]byte(authConfig.String), &setting.Auth)
	}

	if bashAllowlist.Valid && bashAllowlist.String != "" {
		_ = json.Unmarshal([]byte(bashAllowlist.String), &setting.BashAllowlist)
	}

	return setting, nil
}

// scanSettingsRow is a helper to scan a single row into Settings
func scanSettingsRow(row *sql.Row) (Settings, error) {
	var setting Settings
	var uuid, name, platform, authType, authConfig, proxyURL, chatIDLock, bashAllowlist, createdAt, updatedAt sql.NullString
	var enabled int

	if err := row.Scan(&uuid, &name, &platform, &authType, &authConfig, &proxyURL, &chatIDLock, &bashAllowlist, &enabled, &createdAt, &updatedAt); err != nil {
		return setting, err
	}

	setting.Auth = make(map[string]string)
	setting.UUID = uuid.String
	setting.Name = name.String
	setting.Platform = platform.String
	setting.AuthType = authType.String
	setting.ProxyURL = proxyURL.String
	setting.ChatIDLock = chatIDLock.String
	setting.Enabled = enabled == 1
	setting.CreatedAt = createdAt.String
	setting.UpdatedAt = updatedAt.String

	if authConfig.Valid && authConfig.String != "" {
		_ = json.Unmarshal([]byte(authConfig.String), &setting.Auth)
	}

	if bashAllowlist.Valid && bashAllowlist.String != "" {
		_ = json.Unmarshal([]byte(bashAllowlist.String), &setting.BashAllowlist)
	}

	return setting, nil
}

func ensureColumn(db *sql.DB, tableName, columnName, columnType string) error {
	rows, err := db.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, tableName))
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name string
		var colType string
		var notNull int
		var defaultValue sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if strings.EqualFold(name, columnName) {
			return nil
		}
	}

	_, err = db.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s %s`, tableName, columnName, columnType))
	return err
}

func (s *Store) GetSessionForChat(chatID string) (string, bool, error) {
	if s == nil || s.chatStore == nil {
		return "", false, nil
	}
	return s.chatStore.GetSession(chatID)
}

func (s *Store) SetSessionForChat(chatID, sessionID string) error {
	if s == nil || s.chatStore == nil {
		return nil
	}
	return s.chatStore.SetSession(chatID, sessionID)
}

func (s *Store) GetBashCwd(chatID string) (string, bool, error) {
	if s == nil || s.chatStore == nil {
		return "", false, nil
	}
	return s.chatStore.GetBashCwd(chatID)
}

func (s *Store) SetBashCwd(chatID, cwd string) error {
	if s == nil || s.chatStore == nil {
		return nil
	}
	return s.chatStore.SetBashCwd(chatID, cwd)
}

// ============== Group Whitelist Methods ==============

func (s *Store) AddGroupToWhitelist(groupID, platform, addedBy string) error {
	if s == nil || s.chatStore == nil {
		return nil
	}
	return s.chatStore.AddToWhitelist(groupID, platform, addedBy)
}

func (s *Store) RemoveGroupFromWhitelist(groupID string) error {
	if s == nil || s.chatStore == nil {
		return nil
	}
	return s.chatStore.RemoveFromWhitelist(groupID)
}

func (s *Store) IsGroupWhitelisted(groupID string) bool {
	if s == nil || s.chatStore == nil {
		return false
	}
	return s.chatStore.IsWhitelisted(groupID)
}

// ListWhitelistedGroups returns all whitelisted groups
func (s *Store) ListWhitelistedGroups() ([]struct {
	GroupID   string
	Platform  string
	AddedBy   string
	CreatedAt string
}, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	// Use chatStore to get whitelisted chats
	chats, err := s.db.Query(`
		SELECT chat_id, platform, whitelisted_by, created_at
		FROM remote_coder_chats WHERE is_whitelisted = 1
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer chats.Close()

	var results []struct {
		GroupID   string
		Platform  string
		AddedBy   string
		CreatedAt string
	}
	for chats.Next() {
		var r struct {
			GroupID   string
			Platform  string
			AddedBy   string
			CreatedAt string
		}
		if err := chats.Scan(&r.GroupID, &r.Platform, &r.AddedBy, &r.CreatedAt); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, nil
}
