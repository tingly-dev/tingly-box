package session

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/sirupsen/logrus"
)

// MessageStore persists session messages to SQLite
type MessageStore struct {
	db *sql.DB
}

// NewMessageStore creates a SQLite-backed message store
func NewMessageStore(dbPath string) (*MessageStore, error) {
	if dbPath == "" {
		return nil, fmt.Errorf("db path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dbPath), 0700); err != nil {
		return nil, fmt.Errorf("failed to create db dir: %w", err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := initMessageSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &MessageStore{db: db}, nil
}

func initMessageSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS remote_cc_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			role TEXT NOT NULL,
			content TEXT NOT NULL,
			summary TEXT,
			timestamp TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_remote_cc_messages_session_time
		ON remote_cc_messages(session_id, timestamp);
	`)
	return err
}

// InsertMessage writes a message to storage
func (s *MessageStore) InsertMessage(sessionID string, msg Message) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(
		`INSERT INTO remote_cc_messages(session_id, role, content, summary, timestamp) VALUES (?, ?, ?, ?, ?)`,
		sessionID, msg.Role, msg.Content, msg.Summary, msg.Timestamp.Format(time.RFC3339),
	)
	return err
}

// GetMessages returns messages for a session in chronological order
func (s *MessageStore) GetMessages(sessionID string) ([]Message, error) {
	if s == nil || s.db == nil {
		return []Message{}, nil
	}
	rows, err := s.db.Query(
		`SELECT role, content, summary, timestamp FROM remote_cc_messages WHERE session_id = ? ORDER BY timestamp ASC, id ASC`,
		sessionID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Message
	for rows.Next() {
		var role, content, summary, ts string
		if err := rows.Scan(&role, &content, &summary, &ts); err != nil {
			return nil, err
		}
		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			t = time.Now()
		}
		out = append(out, Message{
			Role:      role,
			Content:   content,
			Summary:   summary,
			Timestamp: t,
		})
	}
	return out, nil
}

// DeleteMessagesForSession removes all messages for a session
func (s *MessageStore) DeleteMessagesForSession(sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM remote_cc_messages WHERE session_id = ?`, sessionID)
	return err
}

// ClearAllMessages removes all messages
func (s *MessageStore) ClearAllMessages() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM remote_cc_messages`)
	return err
}

// PurgeOlderThan deletes messages older than a cutoff
func (s *MessageStore) PurgeOlderThan(cutoff time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	res, err := s.db.Exec(`DELETE FROM remote_cc_messages WHERE timestamp < ?`, cutoff.Format(time.RFC3339))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		logrus.Infof("Purged %d remote-cc messages older than %s", n, cutoff.Format(time.RFC3339))
	}
	return nil
}

// Close closes the underlying DB
func (s *MessageStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}
