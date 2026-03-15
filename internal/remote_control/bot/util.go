package bot

import (
	"strings"
)

// normalizeAllowlistToMap converts a string slice to a map for O(1) lookups
func normalizeAllowlistToMap(values []string) map[string]struct{} {
	result := make(map[string]struct{})
	for _, v := range values {
		normalized := strings.ToLower(strings.TrimSpace(v))
		if normalized != "" {
			result[normalized] = struct{}{}
		}
	}
	return result
}

// chunkText splits text into chunks of the specified limit
func chunkText(text string, limit int) []string {
	if limit <= 0 || len(text) <= limit {
		return []string{text}
	}

	var chunks []string
	remaining := text
	for len(remaining) > 0 {
		if len(remaining) <= limit {
			chunks = append(chunks, remaining)
			break
		}
		chunks = append(chunks, remaining[:limit])
		remaining = remaining[limit:]
	}
	return chunks
}
