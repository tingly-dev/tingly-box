package token

func EstimateTokensString(s string) int64 {
	if len(s) == 0 {
		return 0
	}
	return int64((len(s) + 3) / 4)
}

func StringPtr(s string) *string {
	return &s
}

// SplitIntoChunks splits content into word-based chunks for streaming
func SplitIntoChunks(content string) []string {
	words := []string{}
	currentWord := ""
	for _, ch := range content {
		if ch == ' ' || ch == '\n' || ch == '\t' {
			if currentWord != "" {
				words = append(words, currentWord)
				currentWord = ""
			}
			words = append(words, string(ch))
		} else {
			currentWord += string(ch)
		}
	}
	if currentWord != "" {
		words = append(words, currentWord)
	}
	// Add some grouping to make chunks more realistic
	chunks := []string{}
	currentChunk := ""
	for i, word := range words {
		currentChunk += word
		if (i+1)%3 == 0 || i == len(words)-1 {
			chunks = append(chunks, currentChunk)
			currentChunk = ""
		}
	}
	if len(chunks) == 0 {
		chunks = append(chunks, content)
	}
	return chunks
}
