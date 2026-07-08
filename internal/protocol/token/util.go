package token

import "strings"

func EstimateTokensString(s string) int64 {
	if len(s) == 0 {
		return 0
	}
	return int64((len(s) + 3) / 4)
}

func StringPtr(s string) *string {
	return &s
}

// SplitIntoChunks splits content into word-based chunks for streaming.
// Words and chunks are sliced out of the input by byte offset, so no
// per-rune or per-word string concatenation happens.
func SplitIntoChunks(content string) []string {
	words := []string{}
	wordStart := -1
	for i, ch := range content {
		if ch == ' ' || ch == '\n' || ch == '\t' {
			if wordStart >= 0 {
				words = append(words, content[wordStart:i])
				wordStart = -1
			}
			words = append(words, content[i:i+1])
		} else if wordStart < 0 {
			wordStart = i
		}
	}
	if wordStart >= 0 {
		words = append(words, content[wordStart:])
	}
	// Add some grouping to make chunks more realistic
	chunks := []string{}
	var sb strings.Builder
	for i, word := range words {
		sb.WriteString(word)
		if (i+1)%3 == 0 || i == len(words)-1 {
			chunks = append(chunks, sb.String())
			sb.Reset()
		}
	}
	if len(chunks) == 0 {
		chunks = append(chunks, content)
	}
	return chunks
}
