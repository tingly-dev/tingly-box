package bot

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Settings struct {
	Token     string   `json:"token"`
	Allowlist []string `json:"allowlist"`
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
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS remote_coder_bot_allowlist (
			chat_id TEXT PRIMARY KEY,
			added_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS remote_coder_bot_sessions (
			chat_id TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
	`)
	return err
}

func (s *Store) GetSettings() (Settings, error) {
	settings := Settings{}
	if s == nil || s.db == nil {
		return settings, nil
	}

	row := s.db.QueryRow(`SELECT telegram_token FROM remote_coder_bot_settings WHERE id = 1`)
	var token sql.NullString
	if err := row.Scan(&token); err != nil {
		if err != sql.ErrNoRows {
			return settings, err
		}
	} else if token.Valid {
		settings.Token = token.String
	}

	allowlist, err := s.getAllowlist()
	if err != nil {
		return settings, err
	}
	settings.Allowlist = allowlist
	return settings, nil
}

func (s *Store) SaveSettings(settings Settings) error {
	if s == nil || s.db == nil {
		return nil
	}

	cleanAllowlist := normalizeAllowlist(settings.Allowlist)
	settings.Allowlist = cleanAllowlist

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	_, err = tx.Exec(`
		INSERT INTO remote_coder_bot_settings (id, telegram_token, updated_at)
		VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			telegram_token = excluded.telegram_token,
			updated_at = excluded.updated_at
	`, settings.Token, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}

	if _, err = tx.Exec(`DELETE FROM remote_coder_bot_allowlist`); err != nil {
		return err
	}

	if len(settings.Allowlist) > 0 {
		stmt, errPrepare := tx.Prepare(`INSERT INTO remote_coder_bot_allowlist (chat_id, added_at) VALUES (?, ?)`)
		if errPrepare != nil {
			return errPrepare
		}
		defer stmt.Close()
		for _, chatID := range settings.Allowlist {
			if _, err = stmt.Exec(chatID, time.Now().UTC().Format(time.RFC3339)); err != nil {
				return err
			}
		}
	}

	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

func (s *Store) getAllowlist() ([]string, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}

	rows, err := s.db.Query(`SELECT chat_id FROM remote_coder_bot_allowlist`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []string
	for rows.Next() {
		var chatID string
		if err := rows.Scan(&chatID); err != nil {
			return nil, err
		}
		if strings.TrimSpace(chatID) == "" {
			continue
		}
		out = append(out, chatID)
	}
	sort.Strings(out)
	return out, nil
}

func (s *Store) IsAllowed(chatID string) (bool, error) {
	chatID = strings.TrimSpace(chatID)
	if chatID == "" {
		return false, nil
	}
	if s == nil || s.db == nil {
		return false, nil
	}
	row := s.db.QueryRow(`SELECT 1 FROM remote_coder_bot_allowlist WHERE chat_id = ?`, chatID)
	var exists int
	if err := row.Scan(&exists); err != nil {
		if err == sql.ErrNoRows {
			return false, nil
		}
		return false, err
	}
	return true, nil
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

func normalizeAllowlist(allowlist []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, entry := range allowlist {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if _, exists := seen[entry]; exists {
			continue
		}
		seen[entry] = struct{}{}
		out = append(out, entry)
	}
	return out
}
