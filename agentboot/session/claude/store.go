package claude

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/agentboot/session"
)

// Store implements session.Store for Claude Code
type Store struct {
	projectsDir string // Default: ~/.claude/projects
}

// NewStore creates a new Claude session store
func NewStore(projectsDir string) (*Store, error) {
	if projectsDir == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		projectsDir = filepath.Join(homeDir, ".claude", "projects")
	}

	return &Store{
		projectsDir: projectsDir,
	}, nil
}

// ListSessions returns all sessions for a project, ordered by start time (newest first)
func (s *Store) ListSessions(ctx context.Context, projectPath string) ([]session.SessionMetadata, error) {
	projectDir := s.resolveProjectPath(projectPath)
	if projectDir == "" {
		return nil, session.ErrInvalidPath{Path: projectPath}
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []session.SessionMetadata{}, nil
		}
		return nil, err
	}

	var sessions []session.SessionMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .jsonl files
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionPath := filepath.Join(projectDir, entry.Name())
		metadata, err := s.parseSessionFile(sessionPath)
		if err != nil {
			// Log error but continue processing other files
			continue
		}

		sessions = append(sessions, *metadata)
	}

	// Sort by start time, newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// GetSession retrieves a specific session's metadata
func (s *Store) GetSession(ctx context.Context, sessionID string) (*session.SessionMetadata, error) {
	sessionPath := s.findSessionPath(sessionID)
	if sessionPath == "" {
		return nil, session.ErrSessionNotFound{SessionID: sessionID}
	}

	return s.parseSessionFile(sessionPath)
}

// GetRecentSessions returns the N most recent sessions
func (s *Store) GetRecentSessions(ctx context.Context, projectPath string, limit int) ([]session.SessionMetadata, error) {
	sessions, err := s.ListSessions(ctx, projectPath)
	if err != nil {
		return nil, err
	}

	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// ListSessionsFiltered returns sessions that pass the filter, ordered by start time (newest first)
func (s *Store) ListSessionsFiltered(ctx context.Context, projectPath string, filter SessionFilter) ([]session.SessionMetadata, error) {
	projectDir := s.resolveProjectPath(projectPath)
	if projectDir == "" {
		return nil, session.ErrInvalidPath{Path: projectPath}
	}

	entries, err := os.ReadDir(projectDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []session.SessionMetadata{}, nil
		}
		return nil, err
	}

	var sessions []session.SessionMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// Only process .jsonl files
		if !strings.HasSuffix(entry.Name(), ".jsonl") {
			continue
		}

		sessionPath := filepath.Join(projectDir, entry.Name())
		metadata, err := s.parseSessionFile(sessionPath)
		if err != nil {
			// Log error but continue processing other files
			continue
		}

		// Apply filter if provided
		if filter != nil && !filter(*metadata) {
			continue
		}

		sessions = append(sessions, *metadata)
	}

	// Sort by start time, newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].StartTime.After(sessions[j].StartTime)
	})

	return sessions, nil
}

// GetRecentSessionsFiltered returns the N most recent sessions that pass the filter
func (s *Store) GetRecentSessionsFiltered(ctx context.Context, projectPath string, limit int, filter SessionFilter) ([]session.SessionMetadata, error) {
	sessions, err := s.ListSessionsFiltered(ctx, projectPath, filter)
	if err != nil {
		return nil, err
	}

	if len(sessions) > limit {
		sessions = sessions[:limit]
	}

	return sessions, nil
}

// GetSessionEvents retrieves events from a session
func (s *Store) GetSessionEvents(ctx context.Context, sessionID string, offset, limit int) ([]session.SessionEvent, error) {
	sessionPath := s.findSessionPath(sessionID)
	if sessionPath == "" {
		return nil, session.ErrSessionNotFound{SessionID: sessionID}
	}

	return s.parseSessionEvents(sessionPath, offset, limit)
}

// GetSessionSummary returns a summary (first N and last M events)
func (s *Store) GetSessionSummary(ctx context.Context, sessionID string, firstN, lastM int) (*session.SessionSummary, error) {
	sessionPath := s.findSessionPath(sessionID)
	if sessionPath == "" {
		return nil, session.ErrSessionNotFound{SessionID: sessionID}
	}

	metadata, err := s.parseSessionFile(sessionPath)
	if err != nil {
		return nil, err
	}

	head, err := s.parseSessionEvents(sessionPath, 0, firstN)
	if err != nil {
		return nil, err
	}

	tail, err := s.parseSessionEventsFromEnd(sessionPath, lastM)
	if err != nil {
		return nil, err
	}

	return &session.SessionSummary{
		Metadata: *metadata,
		Head:     head,
		Tail:     tail,
	}, nil
}

// findSessionPath searches for a session file across all projects
func (s *Store) findSessionPath(sessionID string) string {
	// Try direct access to project directories
	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectDir := filepath.Join(s.projectsDir, entry.Name())
		sessionPath := filepath.Join(projectDir, sessionID+".jsonl")

		if _, err := os.Stat(sessionPath); err == nil {
			return sessionPath
		}
	}

	return ""
}

// GetProjectSessionsDir returns the directory where sessions are stored for a project
func (s *Store) GetProjectSessionsDir(projectPath string) string {
	return s.resolveProjectPath(projectPath)
}

// GetProjectsDir returns the base projects directory
func (s *Store) GetProjectsDir() string {
	return s.projectsDir
}

// ListProjects returns all project directories that have sessions
func (s *Store) ListProjects(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(s.projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}

	var projects []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		projectPath := filepath.Join(s.projectsDir, entry.Name())
		files, err := os.ReadDir(projectPath)
		if err != nil {
			continue
		}

		// Check if there are any .jsonl files
		for _, file := range files {
			if !file.IsDir() && strings.HasSuffix(file.Name(), ".jsonl") {
				// Decode project name for display
				decoded := s.decodeProjectPath(filepath.Join(projectPath, file.Name()))
				projects = append(projects, decoded)
				break
			}
		}
	}

	return projects, nil
}

// GetProjectStats returns statistics about sessions in a project
type ProjectStats struct {
	TotalSessions    int       `json:"total_sessions"`
	ActiveSessions   int       `json:"active_sessions"`
	CompletedSessions int      `json:"completed_sessions"`
	ErrorSessions    int       `json:"error_sessions"`
	TotalCostUSD     float64   `json:"total_cost_usd"`
	TotalTokens      int64     `json:"total_tokens"`
	OldestSession    time.Time `json:"oldest_session"`
	NewestSession    time.Time `json:"newest_session"`
}

// GetProjectStats returns statistics for a project
func (s *Store) GetProjectStats(ctx context.Context, projectPath string) (*ProjectStats, error) {
	sessions, err := s.ListSessions(ctx, projectPath)
	if err != nil {
		return nil, err
	}

	stats := &ProjectStats{
		TotalSessions: len(sessions),
	}

	for _, sess := range sessions {
		switch sess.Status {
		case session.SessionStatusActive:
			stats.ActiveSessions++
		case session.SessionStatusComplete:
			stats.CompletedSessions++
		case session.SessionStatusError:
			stats.ErrorSessions++
		}

		stats.TotalCostUSD += sess.TotalCostUSD
		stats.TotalTokens += int64(sess.InputTokens + sess.OutputTokens)

		if sess.StartTime.Before(stats.OldestSession) || stats.OldestSession.IsZero() {
			stats.OldestSession = sess.StartTime
		}
		if sess.StartTime.After(stats.NewestSession) {
			stats.NewestSession = sess.StartTime
		}
	}

	return stats, nil
}
