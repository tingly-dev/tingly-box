package decision

import (
	"net/url"
	"strings"
)

type Target struct {
	Dialect  Dialect
	BasePath string
	Endpoint string
}

// ResolveTarget uses exact, known official hosts. Unknown providers stay opaque.
func ResolveTarget(base string) Target {
	u, err := url.Parse(base)
	if err != nil {
		return Target{Dialect: Unknown, Endpoint: "decisions"}
	}
	switch strings.ToLower(u.Hostname()) {
	case "api.typesafe.ai":
		return Target{SystemOne, "/v1/", "systemone"}
	case "openrouter.ai":
		return Target{SystemOne, "/api/alpha/", "decisions"}
	case "api.openai.com", "ai-gateway.vercel.sh":
		// A host can expose other dialects on separate prefixes (for example,
		// Vercel's TypeSafe-compatible surface). Recognize only the standard
		// OpenAI base; custom prefixes remain opaque.
		path := strings.TrimRight(u.EscapedPath(), "/")
		if path == "" || path == "/v1" {
			return Target{OpenAI, "", "decisions"}
		}
		return Target{Dialect: Unknown, Endpoint: "decisions"}
	default:
		return Target{Dialect: Unknown, Endpoint: "decisions"}
	}
}
