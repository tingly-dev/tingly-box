package session

import (
	"database/sql"
	"encoding/json"
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
		CREATE TABLE IF NOT EXISTS remote_cc_sessions (
			id TEXT PRIMARY KEY,
			status TEXT NOT NULL,
			request TEXT,
			response TEXT,
			error TEXT,
			created_at TEXT NOT NULL,
			last_activity TEXT NOT NULL,
			expires_at TEXT NOT NULL,
			context TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_remote_cc_sessions_last_activity
		ON remote_cc_sessions(last_activity);

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

// UpsertSession writes session metadata to storage
func (s *MessageStore) UpsertSession(sess *Session) error {
	if s == nil || s.db == nil || sess == nil {
		return nil
	}
	contextJSON := ""
	if sess.Context != nil {
		if b, err := json.Marshal(sess.Context); err == nil {
			contextJSON = string(b)
		}
	}
	_, err := s.db.Exec(
		`INSERT INTO remote_cc_sessions(id, status, request, response, error, created_at, last_activity, expires_at, context)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET
		   status=excluded.status,
		   request=excluded.request,
		   response=excluded.response,
		   error=excluded.error,
		   created_at=excluded.created_at,
		   last_activity=excluded.last_activity,
		   expires_at=excluded.expires_at,
		   context=excluded.context`,
		sess.ID,
		string(sess.Status),
		sess.Request,
		sess.Response,
		sess.Error,
		sess.CreatedAt.Format(time.RFC3339),
		sess.LastActivity.Format(time.RFC3339),
		sess.ExpiresAt.Format(time.RFC3339),
		contextJSON,
	)
	return err
}

// DeleteSession removes a session from storage
func (s *MessageStore) DeleteSession(sessionID string) error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM remote_cc_sessions WHERE id = ?`, sessionID)
	return err
}

// ClearAllSessions removes all sessions from storage
func (s *MessageStore) ClearAllSessions() error {
	if s == nil || s.db == nil {
		return nil
	}
	_, err := s.db.Exec(`DELETE FROM remote_cc_sessions`)
	return err
}

// LoadSessions loads all sessions from storage
func (s *MessageStore) LoadSessions() ([]*Session, error) {
	if s == nil || s.db == nil {
		return []*Session{}, nil
	}
	rows, err := s.db.Query(
		`SELECT id, status, request, response, error, created_at, last_activity, expires_at, context FROM remote_cc_sessions`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Session
	for rows.Next() {
		var id, status, request, response, errMsg, createdAt, lastActivity, expiresAt, contextJSON string
		if err := rows.Scan(&id, &status, &request, &response, &errMsg, &createdAt, &lastActivity, &expiresAt, &contextJSON); err != nil {
			return nil, err
		}
		created, err := time.Parse(time.RFC3339, createdAt)
		if err != nil {
			created = time.Now()
		}
		last, err := time.Parse(time.RFC3339, lastActivity)
		if err != nil {
			last = created
		}
		expires, err := time.Parse(time.RFC3339, expiresAt)
		if err != nil {
			expires = last.Add(30 * time.Minute)
		}
		ctx := make(map[string]interface{})
		if contextJSON != "" {
			_ = json.Unmarshal([]byte(contextJSON), &ctx)
		}
		out = append(out, &Session{
			ID:           id,
			Status:       Status(status),
			Request:      request,
			Response:     response,
			Error:        errMsg,
			CreatedAt:    created,
			LastActivity: last,
			ExpiresAt:    expires,
			Context:      ctx,
		})
	}
	return out, nil
}

// GetSession retrieves a single session from storage
func (s *MessageStore) GetSession(sessionID string) (*Session, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	row := s.db.QueryRow(
		`SELECT id, status, request, response, error, created_at, last_activity, expires_at, context
		 FROM remote_cc_sessions WHERE id = ?`,
		sessionID,
	)

	var id, status, request, response, errMsg, createdAt, lastActivity, expiresAt, contextJSON string
	if err := row.Scan(&id, &status, &request, &response, &errMsg, &createdAt, &lastActivity, &expiresAt, &contextJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}

	created, err := time.Parse(time.RFC3339, createdAt)
	if err != nil {
		created = time.Now()
	}
	last, err := time.Parse(time.RFC3339, lastActivity)
	if err != nil {
		last = created
	}
	expires, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		expires = last.Add(30 * time.Minute)
	}
	ctx := make(map[string]interface{})
	if contextJSON != "" {
		_ = json.Unmarshal([]byte(contextJSON), &ctx)
	}

	return &Session{
		ID:           id,
		Status:       Status(status),
		Request:      request,
		Response:     response,
		Error:        errMsg,
		CreatedAt:    created,
		LastActivity: last,
		ExpiresAt:    expires,
		Context:      ctx,
	}, nil
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
		logrus.Infof("Purged %d remote-coder messages older than %s", n, cutoff.Format(time.RFC3339))
	}
	return nil
}

// PurgeSessionsOlderThan deletes sessions older than a cutoff
func (s *MessageStore) PurgeSessionsOlderThan(cutoff time.Time) error {
	if s == nil || s.db == nil {
		return nil
	}
	res, err := s.db.Exec(`DELETE FROM remote_cc_sessions WHERE last_activity < ?`, cutoff.Format(time.RFC3339))
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		logrus.Infof("Purged %d remote-coder sessions older than %s", n, cutoff.Format(time.RFC3339))
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
