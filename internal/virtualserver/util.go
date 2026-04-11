package virtualserver

func estimateTokens(messages []Message) int {
	total := 0
	for _, msg := range messages {
		total += estimateTokensString(msg.Content) + 5
	}
	return total
}

func estimateTokensString(s string) int {
	if len(s) == 0 {
		return 0
	}
	return (len(s) + 3) / 4
}

func stringPtr(s string) *string {
	return &s
}

func splitIntoChunks(content string) []string {
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
