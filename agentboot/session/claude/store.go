package claude

import (
	"context"
	"encoding/json"
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

		projectDir := filepath.Join(s.projectsDir, entry.Name())

		// Try to get original path from sessions-index.json first
		originalPath := s.getOriginalPathFromIndex(projectDir)
		if originalPath != "" {
			projects = append(projects, originalPath)
			continue
		}

		// Fallback: try to find path from session files or existing directories
		projectPath := s.findProjectPathFallback(projectDir)
		if projectPath != "" {
			projects = append(projects, projectPath)
		}
	}

	return projects, nil
}

// findProjectPathFallback tries to find the project path using various fallback strategies
func (s *Store) findProjectPathFallback(projectDir string) string {
	// Strategy 1: Read cwd from first session file
	sessionFiles, err := filepath.Glob(filepath.Join(projectDir, "*.jsonl"))
	if err == nil && len(sessionFiles) > 0 {
		if cwd := s.getCwdFromSessionFile(sessionFiles[0]); cwd != "" {
			return cwd
		}
	}

	// Strategy 2: Decode the path and try to find the closest existing directory
	decoded := s.decodeProjectPath(filepath.Join(projectDir, "dummy.jsonl"))
	if decoded != "" {
		// Try exact match first
		if info, err := os.Stat(decoded); err == nil && info.IsDir() {
			return decoded
		}

		// Strategy 3: Try to find the closest existing parent directory
		if closest := s.findClosestExistingPath(decoded); closest != "" {
			return closest
		}
	}

	// Last resort: return the decoded path even if it doesn't exist
	// This is better than returning nothing
	return decoded
}

// getCwdFromSessionFile extracts the cwd field from a session file
func (s *Store) getCwdFromSessionFile(sessionPath string) string {
	content, err := os.ReadFile(sessionPath)
	if err != nil {
		return ""
	}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		var event map[string]interface{}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Look for cwd field in user events
		if eventType, ok := event["type"].(string); ok && eventType == "user" {
			if cwd, ok := event["cwd"].(string); ok && cwd != "" {
				return cwd
			}
		}
	}

	return ""
}

// findClosestExistingPath tries to find the closest existing directory
// by checking parents and children of the decoded path
func (s *Store) findClosestExistingPath(decodedPath string) string {
	// First, try to find a valid parent directory
	// Split the path into components and try progressively longer prefixes
	parts := strings.Split(strings.Trim(decodedPath, "/"), "/")

	// Try from the full path down to root
	// We need at least 4 components for a meaningful path (e.g., /Users/yz/Project/something)
	minComponents := 4
	for i := len(parts); i >= minComponents; i-- {
		candidate := "/" + filepath.Join(parts[:i]...)
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			// Found an existing directory
			// Prefer deeper paths (more specific)
			return candidate
		}
	}

	// If no suitable parent found, return empty string
	// to avoid returning too generic paths like /Users or /Users/yz
	return ""
}

// walkForMatch recursively walks the directory tree to find a matching path
func (s *Store) walkForMatch(baseDir string, targetParts []string, depth int) string {
	if depth > 3 || depth >= len(targetParts) {
		return ""
	}

	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return ""
	}

	targetPart := targetParts[depth]
	// Normalize target part for comparison (replace - with space for fuzzy matching)
	normalizedTarget := strings.ReplaceAll(targetPart, "-", " ")
	normalizedTarget = strings.ToLower(normalizedTarget)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		name := entry.Name()
		normalized := strings.ReplaceAll(name, "-", " ")
		normalized = strings.ToLower(normalized)

		// Check for exact match or fuzzy match
		if name == targetPart || normalized == normalizedTarget ||
			strings.Contains(normalized, normalizedTarget) ||
			strings.Contains(normalizedTarget, normalized) {

			// Check if we've matched enough parts
			if depth == len(targetParts)-1 {
				return filepath.Join(baseDir, name)
			}

			// Try to go deeper
			fullPath := filepath.Join(baseDir, name)
			if deeper := s.walkForMatch(fullPath, targetParts, depth+1); deeper != "" {
				return deeper
			}
		}
	}

	return ""
}

// getOriginalPathFromIndex reads the originalPath from sessions-index.json if available
func (s *Store) getOriginalPathFromIndex(projectDir string) string {
	indexPath := filepath.Join(projectDir, "sessions-index.json")
	content, err := os.ReadFile(indexPath)
	if err != nil {
		return ""
	}

	var index struct {
		OriginalPath string `json:"originalPath"`
	}
	if err := json.Unmarshal(content, &index); err != nil {
		return ""
	}

	return index.OriginalPath
}

// GetProjectStats returns statistics about sessions in a project
type ProjectStats struct {
	TotalSessions     int       `json:"total_sessions"`
	ActiveSessions    int       `json:"active_sessions"`
	CompletedSessions int       `json:"completed_sessions"`
	ErrorSessions     int       `json:"error_sessions"`
	TotalCostUSD      float64   `json:"total_cost_usd"`
	TotalTokens       int64     `json:"total_tokens"`
	OldestSession     time.Time `json:"oldest_session"`
	NewestSession     time.Time `json:"newest_session"`
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
