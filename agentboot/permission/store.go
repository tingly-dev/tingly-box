package permission

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot"
)

// Store handles permission history persistence
type Store struct {
	db *sql.DB
}

// NewStore creates a new permission store
func NewStore(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("db is nil")
	}

	store := &Store{db: db}

	// Initialize schema
	if err := store.InitSchema(); err != nil {
		return nil, fmt.Errorf("failed to init schema: %w", err)
	}

	return store, nil
}

// InitSchema creates the permission history table
func (s *Store) InitSchema() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS permission_history (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id TEXT NOT NULL,
			chat_id TEXT,
			agent_type TEXT,
			tool_name TEXT NOT NULL,
			input TEXT,
			approved INTEGER NOT NULL,
			reason TEXT,
			mode TEXT,
			timestamp INTEGER NOT NULL,
			response_time_ms INTEGER
		);

		CREATE INDEX IF NOT EXISTS idx_perm_session ON permission_history(session_id);
		CREATE INDEX IF NOT EXISTS idx_perm_chat ON permission_history(chat_id);
		CREATE INDEX IF NOT EXISTS idx_perm_tool ON permission_history(tool_name);
		CREATE INDEX IF NOT EXISTS idx_perm_timestamp ON permission_history(timestamp);
	`)
	return err
}

// DecisionRecord represents a stored permission decision
type DecisionRecord struct {
	ID             int64     `json:"id"`
	SessionID      string    `json:"session_id"`
	ChatID         string    `json:"chat_id"`
	AgentType      string    `json:"agent_type"`
	ToolName       string    `json:"tool_name"`
	Input          string    `json:"input"`
	Approved       bool      `json:"approved"`
	Reason         string    `json:"reason"`
	Mode           string    `json:"mode"`
	Timestamp      time.Time `json:"timestamp"`
	ResponseTimeMs int       `json:"response_time_ms"`
}

// RecordDecision stores a permission decision
func (s *Store) RecordDecision(req agentboot.PermissionRequest, resp agentboot.PermissionResponse, responseTime time.Duration) error {
	inputJSON, _ := json.Marshal(req.Input)

	_, err := s.db.Exec(`
		INSERT INTO permission_history
		(session_id, chat_id, agent_type, tool_name, input, approved, reason, mode, timestamp, response_time_ms)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, req.SessionID, req.SessionID, string(req.AgentType), req.ToolName, string(inputJSON),
		boolToInt(resp.Approved), resp.Reason, "", time.Now().Unix(), int(responseTime.Milliseconds()))

	return err
}

// GetDecisionHistory retrieves decision history for a session
func (s *Store) GetDecisionHistory(sessionID string, limit int) ([]DecisionRecord, error) {
	if limit == 0 {
		limit = 100
	}

	rows, err := s.db.Query(`
		SELECT id, session_id, chat_id, agent_type, tool_name, input, approved, reason, mode, timestamp, response_time_ms
		FROM permission_history
		WHERE session_id = ?
		ORDER BY timestamp DESC
		LIMIT ?
	`, sessionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []DecisionRecord
	for rows.Next() {
		var r DecisionRecord
		var inputSQL sql.NullString
		var reasonSQL sql.NullString
		var modeSQL sql.NullString
		var timestamp int64

		err := rows.Scan(&r.ID, &r.SessionID, &r.ChatID, &r.AgentType, &r.ToolName, &inputSQL,
			&r.Approved, &reasonSQL, &modeSQL, &timestamp, &r.ResponseTimeMs)
		if err != nil {
			return nil, err
		}

		if inputSQL.Valid {
			r.Input = inputSQL.String
		}
		if reasonSQL.Valid {
			r.Reason = reasonSQL.String
		}
		if modeSQL.Valid {
			r.Mode = modeSQL.String
		}

		r.Timestamp = time.Unix(timestamp, 0)
		records = append(records, r)
	}

	return records, nil
}

// GetStats retrieves permission statistics
type Stats struct {
	Total            int                    `json:"total"`
	Approved         int                    `json:"approved"`
	Denied           int                    `json:"denied"`
	AutoApproved     int                    `json:"auto_approved"`
	ManualApproved   int                    `json:"manual_approved"`
	AvgResponseTime  int                    `json:"avg_response_time_ms"`
	TopTools         []ToolUsageStat        `json:"top_tools"`
}

type ToolUsageStat struct {
	ToolName string `json:"tool_name"`
	Count    int    `json:"count"`
	Approved int    `json:"approved"`
	Denied   int    `json:"denied"`
}

func (s *Store) GetStats(since time.Time, sessionID string) (*Stats, error) {
	var stats Stats

	// Get total counts
	query := `
		SELECT
			COUNT(*) as total,
			SUM(CASE WHEN approved = 1 THEN 1 ELSE 0 END) as approved,
			SUM(CASE WHEN approved = 0 THEN 1 ELSE 0 END) as denied,
			AVG(response_time_ms) as avg_response_time
		FROM permission_history
		WHERE timestamp >= ?
	`
	args := []interface{}{since.Unix()}

	if sessionID != "" {
		query += " AND session_id = ?"
		args = append(args, sessionID)
	}

	err := s.db.QueryRow(query, args...).Scan(
		&stats.Total, &stats.Approved, &stats.Denied, &stats.AvgResponseTime,
	)
	if err != nil {
		return nil, err
	}

	// Get top tools
	toolQuery := `
		SELECT tool_name,
			   COUNT(*) as count,
			   SUM(CASE WHEN approved = 1 THEN 1 ELSE 0 END) as approved,
			   SUM(CASE WHEN approved = 0 THEN 1 ELSE 0 END) as denied
		FROM permission_history
		WHERE timestamp >= ?
	`
	args2 := []interface{}{since.Unix()}

	if sessionID != "" {
		toolQuery += " AND session_id = ?"
		args2 = append(args2, sessionID)
	}

	toolQuery += " GROUP BY tool_name ORDER BY count DESC LIMIT 10"

	rows, err := s.db.Query(toolQuery, args2...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var stat ToolUsageStat
		if err := rows.Scan(&stat.ToolName, &stat.Count, &stat.Approved, &stat.Denied); err != nil {
			return nil, err
		}
		stats.TopTools = append(stats.TopTools, stat)
	}

	return &stats, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// PurgeOlderThan removes records older than the specified time
func (s *Store) PurgeOlderThan(cutoff time.Time) (int64, error) {
	result, err := s.db.Exec(`
		DELETE FROM permission_history WHERE timestamp < ?
	`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ClearAll removes all permission history
func (s *Store) ClearAll() error {
	_, err := s.db.Exec(`DELETE FROM permission_history`)
	return err
}
