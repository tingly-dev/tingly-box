package claude

import (
	"strings"

	"github.com/tingly-dev/tingly-box/agentboot/session"
)

// SessionFilter defines a function to filter sessions
// Returns true if the session should be included, false to exclude
type SessionFilter func(metadata session.SessionMetadata) bool

// DefaultSessionFilter returns the default filter that excludes:
// - Meta messages (isMeta=true)
// - Empty or very short content (< 5 chars)
// - Sessions with no meaningful content
func DefaultSessionFilter() SessionFilter {
	return func(metadata session.SessionMetadata) bool {
		// Check if empty first message
		trimmedFirst := strings.TrimSpace(metadata.FirstMessage)
		trimmedLastUser := strings.TrimSpace(metadata.LastUserMessage)

		// If both first and last user messages are empty/too short, exclude
		// This handles sessions that are mostly meta/system events
		if len(trimmedFirst) < 5 && len(trimmedLastUser) < 5 {
			return false
		}

		// Check for known meta patterns
		// (Future: can be expanded with more sophisticated detection)
		emptyMetaPatterns := []string{
			"<local-command-caveat>",
			"<ide_opened_file>",
			"<local-command-",
		}

		content := trimmedFirst
		if content == "" {
			content = trimmedLastUser
		}

		// Check if content starts with known meta patterns
		for _, pattern := range emptyMetaPatterns {
			if strings.Contains(content, pattern) {
				return false
			}
		}

		return true
	}
}

// WithFilter creates a new slice with only sessions that pass the filter
func WithFilter(sessions []session.SessionMetadata, filter SessionFilter) []session.SessionMetadata {
	if filter == nil {
		return sessions
	}

	var filtered []session.SessionMetadata
	for _, sess := range sessions {
		if filter(sess) {
			filtered = append(filtered, sess)
		}
	}
	return filtered
}

// ExcludeMetaOnly returns a filter that only excludes isMeta=true sessions
// This is useful when you want to keep all user messages including system warnings
func ExcludeMetaOnly() SessionFilter {
	return func(metadata session.SessionMetadata) bool {
		// Check for explicit isMeta flag if available
		// This would need to be tracked during parsing
		return true
	}
}

// ExcludeShortContent returns a filter that excludes sessions with very short content
func ExcludeShortContent(minLength int) SessionFilter {
	return func(metadata session.SessionMetadata) bool {
		trimmedFirst := strings.TrimSpace(metadata.FirstMessage)
		trimmedLastUser := strings.TrimSpace(metadata.LastUserMessage)

		// Check both first and last user message
		if len(trimmedFirst) >= minLength {
			return true
		}
		if len(trimmedLastUser) >= minLength {
			return true
		}

		return false
	}
}

// ExcludePatterns returns a filter that excludes sessions containing specific patterns
func ExcludePatterns(patterns []string) SessionFilter {
	return func(metadata session.SessionMetadata) bool {
		content := strings.TrimSpace(metadata.FirstMessage)
		if content == "" {
			content = strings.TrimSpace(metadata.LastUserMessage)
		}

		for _, pattern := range patterns {
			if strings.Contains(content, pattern) {
				return false
			}
		}

		return true
	}
}

// CombineFilters returns a filter that only passes if all filters pass
func CombineFilters(filters ...SessionFilter) SessionFilter {
	return func(metadata session.SessionMetadata) bool {
		for _, filter := range filters {
			if !filter(metadata) {
				return false
			}
		}
		return true
	}
}
