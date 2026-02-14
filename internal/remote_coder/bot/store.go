package bot

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Settings struct {
	Token         string   `json:"token"`
	Platform      string   `json:"platform"`
	ProxyURL      string   `json:"proxy_url"`
	ChatIDLock    string   `json:"chat_id"`
	BashAllowlist []string `json:"bash_allowlist"`
}

type Store struct {
	db *sql.DB
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
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func initSchema(db *sql.DB) error {
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
	`)
	if err != nil {
		return err
	}

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

	return nil
}

func (s *Store) GetSettings() (Settings, error) {
	settings := Settings{}
	if s == nil || s.db == nil {
		return settings, nil
	}

	row := s.db.QueryRow(`SELECT telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist FROM remote_coder_bot_settings WHERE id = 1`)
	var token sql.NullString
	var platform sql.NullString
	var proxyURL sql.NullString
	var chatIDLock sql.NullString
	var bashAllowlist sql.NullString
	if err := row.Scan(&token, &platform, &proxyURL, &chatIDLock, &bashAllowlist); err != nil {
		if err != sql.ErrNoRows {
			return settings, err
		}
	} else if token.Valid {
		settings.Token = token.String
	}
	if platform.Valid {
		settings.Platform = platform.String
	}
	if proxyURL.Valid {
		settings.ProxyURL = proxyURL.String
	}
	if chatIDLock.Valid {
		settings.ChatIDLock = chatIDLock.String
	}
	if bashAllowlist.Valid && bashAllowlist.String != "" {
		_ = json.Unmarshal([]byte(bashAllowlist.String), &settings.BashAllowlist)
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

	_, err = tx.Exec(`
		INSERT INTO remote_coder_bot_settings (id, telegram_token, platform, proxy_url, chat_id_lock, bash_allowlist, updated_at)
		VALUES (1, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			telegram_token = excluded.telegram_token,
			platform = excluded.platform,
			proxy_url = excluded.proxy_url,
			chat_id_lock = excluded.chat_id_lock,
			bash_allowlist = excluded.bash_allowlist,
			updated_at = excluded.updated_at
	`, settings.Token, settings.Platform, settings.ProxyURL, settings.ChatIDLock, allowlistJSON, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
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
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || s == nil || s.db == nil {
		return "", false, nil
	}
	row := s.db.QueryRow(`SELECT session_id FROM remote_coder_bot_sessions WHERE chat_id = ?`, chatID)
	var sessionID string
	if err := row.Scan(&sessionID); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return sessionID, true, nil
}

func (s *Store) SetSessionForChat(chatID, sessionID string) error {
	chatID = strings.TrimSpace(chatID)
	sessionID = strings.TrimSpace(sessionID)
	if chatID == "" || sessionID == "" || s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO remote_coder_bot_sessions (chat_id, session_id, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			session_id = excluded.session_id,
			updated_at = excluded.updated_at
	`, chatID, sessionID, time.Now().UTC().Format(time.RFC3339))
	return err
}

func (s *Store) GetBashCwd(chatID string) (string, bool, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" || s == nil || s.db == nil {
		return "", false, nil
	}
	row := s.db.QueryRow(`SELECT cwd FROM remote_coder_bot_bash_cwd WHERE chat_id = ?`, chatID)
	var cwd string
	if err := row.Scan(&cwd); err != nil {
		if err == sql.ErrNoRows {
			return "", false, nil
		}
		return "", false, err
	}
	return cwd, true, nil
}

func (s *Store) SetBashCwd(chatID, cwd string) error {
	chatID = strings.TrimSpace(chatID)
	cwd = strings.TrimSpace(cwd)
	if chatID == "" || cwd == "" || s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`
		INSERT INTO remote_coder_bot_bash_cwd (chat_id, cwd, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(chat_id) DO UPDATE SET
			cwd = excluded.cwd,
			updated_at = excluded.updated_at
	`, chatID, cwd, time.Now().UTC().Format(time.RFC3339))
	return err
}
