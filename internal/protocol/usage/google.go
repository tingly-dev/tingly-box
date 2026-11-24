package usage

import (
	protocol "github.com/tingly-dev/tingly-box/ai"
	"google.golang.org/genai"
)

// FromGoogle normalizes Gemini usage. Cached content is a subset of the prompt;
// thoughts are additional generated tokens, separate from candidate tokens.
func FromGoogle(u *genai.GenerateContentResponseUsageMetadata) *protocol.TokenUsage {
	if u == nil {
		return protocol.ZeroTokenUsage()
	}
	return protocol.NewTokenUsageFull(int(u.PromptTokenCount-u.CachedContentTokenCount), int(u.CandidatesTokenCount+u.ThoughtsTokenCount), int(u.CachedContentTokenCount), 0, int(u.ThoughtsTokenCount))
}
