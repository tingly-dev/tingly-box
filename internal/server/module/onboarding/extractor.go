package onboarding

import (
	"context"
	"net/url"
	"regexp"
	"strings"
)

// Extractor is the contract handlers depend on. v1 ships a pure rule-based
// implementation; future variants (LLM-assisted, model-based) can satisfy
// the same contract without changing the handler or wire format.
type Extractor interface {
	Extract(ctx context.Context, input string) (data ExtractData, err error)
}

var (
	urlRegex      = regexp.MustCompile(`https?://[^\s'"\x60<>]+`)
	envVarRegex   = regexp.MustCompile(`\b([A-Z][A-Z0-9_]{3,})\s*[:=]\s*['"]?([^\s'"\x60]+)`)
	bearerRegex   = regexp.MustCompile(`(?i)Bearer\s+([A-Za-z0-9_\-\.]{12,})`)
	xApiKeyRegex  = regexp.MustCompile(`(?i)x-api-key\s*[:=]\s*['"]?([A-Za-z0-9_\-\.]{12,})`)
	jsonAPIKeyRe  = regexp.MustCompile(`(?i)"api[_-]?key"\s*:\s*"([^"]+)"`)
	jsonBaseURLRe = regexp.MustCompile(`(?i)"base[_-]?url"\s*:\s*"([^"]+)"`)
	// Catch-all: alphanumeric strings 12+ chars that look like API tokens
	tokenLikeRegex = regexp.MustCompile(`\b[A-Za-z0-9_\-\.]{12,}\b`)
)

// Heuristic minimum length for env-var values to qualify as token candidates.
// Below this, values like `LANG=en_US` would be reported as tokens.
const envValueMinLen = 12

// Names of env vars whose values are URLs, not tokens. Anything else gets
// treated as a possible token if its value is long enough.
var urlEnvVarNames = map[string]bool{
	"OPENAI_BASE_URL":     true,
	"OPENAI_API_BASE":     true,
	"ANTHROPIC_BASE_URL":  true,
	"DEEPSEEK_BASE_URL":   true,
	"OPENROUTER_BASE_URL": true,
	"BASE_URL":            true,
	"API_BASE":            true,
	"API_BASE_URL":        true,
	"HTTPS_PROXY":         true,
	"HTTP_PROXY":          true,
	"NO_PROXY":            true,
}

// RuleExtractor implements Extractor with pure regex matching.
// No LLM, no network calls, no vendor-specific assumptions.
type RuleExtractor struct{}

// NewRuleExtractor builds a new RuleExtractor. The signature accepts a
// dependency placeholder (any) for future-proofing — the v1 implementation
// has no dependencies.
func NewRuleExtractor() *RuleExtractor {
	return &RuleExtractor{}
}

// Extract scans the input text for URLs and possible API tokens. It is
// deliberately vendor-agnostic: no scoring, no provider matching, no
// warnings. The caller decides what to do with the raw signals.
func (e *RuleExtractor) Extract(_ context.Context, input string) (ExtractData, error) {
	if strings.TrimSpace(input) == "" {
		return ExtractData{URLs: []string{}, Tokens: []TokenCandidate{}}, nil
	}

	// --- URLs ---
	var urls []string
	for _, raw := range urlRegex.FindAllString(input, -1) {
		urls = append(urls, cleanURL(raw))
	}
	for _, m := range jsonBaseURLRe.FindAllStringSubmatch(input, -1) {
		if len(m) >= 2 {
			urls = append(urls, cleanURL(m[1]))
		}
	}
	for _, m := range envVarRegex.FindAllStringSubmatch(input, -1) {
		if len(m) < 3 {
			continue
		}
		val := m[2]
		if isURLString(val) {
			urls = append(urls, cleanURL(val))
		}
	}
	urls = dedupeStrings(urls)

	// --- Tokens ---
	var tokens []TokenCandidate
	for _, m := range bearerRegex.FindAllStringSubmatch(input, -1) {
		if len(m) >= 2 {
			tokens = append(tokens, TokenCandidate{Value: m[1], Source: "bearer"})
		}
	}
	for _, m := range xApiKeyRegex.FindAllStringSubmatch(input, -1) {
		if len(m) >= 2 {
			tokens = append(tokens, TokenCandidate{Value: m[1], Source: "x-api-key"})
		}
	}
	for _, m := range jsonAPIKeyRe.FindAllStringSubmatch(input, -1) {
		if len(m) >= 2 {
			tokens = append(tokens, TokenCandidate{Value: m[1], Source: "json:api_key"})
		}
	}
	for _, tok := range tokenLikeRegex.FindAllString(input, -1) {
		if isURLString(tok) {
			continue
		}
		if isLowEntropy(tok) {
			continue
		}
		tokens = append(tokens, TokenCandidate{Value: tok, Source: "token_like"})
	}
	for _, m := range envVarRegex.FindAllStringSubmatch(input, -1) {
		if len(m) < 3 {
			continue
		}
		name := strings.ToUpper(m[1])
		val := m[2]
		if isURLString(val) {
			continue
		}
		if urlEnvVarNames[name] {
			continue
		}
		if len(val) < envValueMinLen {
			continue
		}
		tokens = append(tokens, TokenCandidate{Value: val, Source: "env:" + name})
	}
	tokens = dedupeTokens(tokens)
	for i := range tokens {
		tokens[i].Preview = maskToken(tokens[i].Value)
	}

	if urls == nil {
		urls = []string{}
	}
	if tokens == nil {
		tokens = []TokenCandidate{}
	}
	return ExtractData{URLs: urls, Tokens: tokens}, nil
}

// --- helpers ---

func isURLString(s string) bool {
	low := strings.ToLower(s)
	return strings.HasPrefix(low, "http://") || strings.HasPrefix(low, "https://")
}

// isLowEntropy filters out strings that definitely aren't API tokens.
// Since users can pick from candidates, we only filter obvious garbage.
func isLowEntropy(s string) bool {
	if len(s) == 0 {
		return true
	}

	// Skip all-digit strings (timestamps, simple IDs)
	allDigits := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigits = false
			break
		}
	}
	if allDigits {
		return true
	}

	// Skip strings with < 2 unique characters (obvious repetition like "aaaaa...")
	unique := make(map[rune]bool, len(s))
	for _, c := range s {
		unique[c] = true
	}
	if len(unique) < 2 {
		return true
	}

	// Skip things that look like domain names (contain dots but no other token-like chars)
	if strings.Contains(s, ".") && !strings.ContainsAny(s, "_-") {
		// Check if it looks like a domain
		parts := strings.Split(s, ".")
		if len(parts) >= 2 {
			allAlpha := true
			for _, part := range parts {
				if len(part) == 0 {
					allAlpha = false
					break
				}
				for _, c := range part {
					if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
						allAlpha = false
						break
					}
				}
			}
			if allAlpha {
				return true // looks like a domain name
			}
		}
	}

	return false
}

// cleanURL trims trailing punctuation that often follows URLs in prose
// ("see https://api.anthropic.com.") and surrounding quotes from JSON.
func cleanURL(raw string) string {
	s := strings.TrimSpace(raw)
	s = strings.Trim(s, `"'`+"`")
	for len(s) > 0 {
		last := s[len(s)-1]
		if last == '.' || last == ',' || last == ';' || last == ')' || last == ']' || last == '}' {
			s = s[:len(s)-1]
			continue
		}
		break
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return s
	}
	return s
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

// dedupeTokens dedupes by token value. When the same value appears under
// multiple sources we keep the more informative source — Bearer / x-api-key
// / env-var-name beat the generic key_prefix detector.
func dedupeTokens(in []TokenCandidate) []TokenCandidate {
	idx := make(map[string]int, len(in))
	out := make([]TokenCandidate, 0, len(in))
	for _, t := range in {
		if t.Value == "" {
			continue
		}
		if i, ok := idx[t.Value]; ok {
			if sourcePriority(t.Source) > sourcePriority(out[i].Source) {
				out[i].Source = t.Source
			}
			continue
		}
		idx[t.Value] = len(out)
		out = append(out, t)
	}
	return out
}

func sourcePriority(src string) int {
	switch {
	case strings.HasPrefix(src, "env:"):
		return 4
	case src == "x-api-key":
		return 3
	case src == "bearer":
		return 3
	case src == "json:api_key":
		return 2
	case src == "token_like":
		return 1
	}
	return 0
}

// maskToken returns a redacted preview safe for display. Short tokens get
// fully bulleted; longer tokens show their first 4 and last 4 characters.
func maskToken(raw string) string {
	if len(raw) == 0 {
		return ""
	}
	if len(raw) <= 8 {
		return strings.Repeat("•", len(raw))
	}
	return raw[:4] + "…" + raw[len(raw)-4:]
}
