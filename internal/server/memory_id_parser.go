package server

import (
	"fmt"
	"regexp"

	"github.com/google/uuid"
)

// ClaudeCodeUserID represents the parsed components of a claude_code user_id
// Format: user_{hash}_account__session_{uuid}
// Example: user_5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907_account__session_81ca9881-6299-46c2-ae66-7bb28357034f
type ClaudeCodeUserID struct {
	UserID    string // The hash part: 5e35a7eade54f54369d7937e3c0530db22e875f470179b5e9cb01e682630c907
	SessionID string // The UUID part: 81ca9881-6299-46c2-ae66-7bb28357034f
	RawValue  string // Original value from header
}

// claudeCodeUserIDPattern matches: user_{hash}_account__session_{uuid}
var claudeCodeUserIDPattern = regexp.MustCompile(
	`^user_([a-f0-9]{64})_account__session_([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})$`,
)

// ParseClaudeCodeUserID parses a claude_code user_id string
// Returns parsed info or error if format doesn't match
func ParseClaudeCodeUserID(userID string) (*ClaudeCodeUserID, error) {
	matches := claudeCodeUserIDPattern.FindStringSubmatch(userID)
	if matches == nil {
		return nil, fmt.Errorf("invalid claude_code user_id format: %s", userID)
	}

	return &ClaudeCodeUserID{
		UserID:    matches[1], // 64-char hash
		SessionID: matches[2], // UUID
		RawValue:  userID,
	}, nil
}

// TryParseClaudeCodeUserID attempts to parse, returns nil if format doesn't match
func TryParseClaudeCodeUserID(userID string) *ClaudeCodeUserID {
	parsed, err := ParseClaudeCodeUserID(userID)
	if err != nil {
		return nil
	}
	return parsed
}

// GenerateSessionID generates a new UUID-based session ID
func GenerateSessionID() string {
	return uuid.New().String()
}

// GenerateUserID generates a new UUID-based user ID
func GenerateUserID() string {
	return uuid.New().String()
}

// ResolveSessionID returns the session ID from claude_code user_id if available,
// otherwise generates a new UUID
func ResolveSessionID(anthropicUserID string) string {
	// Try to parse as claude_code format
	if parsed := TryParseClaudeCodeUserID(anthropicUserID); parsed != nil {
		return parsed.SessionID
	}
	// Generate new UUID if pattern doesn't match
	return GenerateSessionID()
}

// ResolveUserID returns the user ID from claude_code user_id if available,
// otherwise returns the original value (or generates UUID if empty)
func ResolveUserID(anthropicUserID string) string {
	if anthropicUserID == "" {
		return GenerateUserID()
	}
	// Try to parse as claude_code format
	if parsed := TryParseClaudeCodeUserID(anthropicUserID); parsed != nil {
		return parsed.UserID
	}
	// Return original if it doesn't match claude_code pattern
	// (might be a different user_id format from another source)
	return anthropicUserID
}
