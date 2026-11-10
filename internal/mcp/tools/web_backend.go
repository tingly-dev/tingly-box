package tools

import (
	"fmt"
	"strings"
)

// WebBackend preserves Serper/Jina when a Serper key exists; otherwise an
// available DeepSeek provider makes both tools work with the same account.
// Overrides live on each source's existing Env, not in a new mode-selection UI.
func WebBackend(env map[string]string, tool string, deepSeekAvailable bool) (string, error) {
	key, legacy := "WEB_SEARCH_BACKEND", "serper"
	if tool == BuiltinWebFetchToolName {
		key, legacy = "WEB_FETCH_BACKEND", "jina"
	}
	backend := strings.ToLower(strings.TrimSpace(env[key]))
	switch backend {
	case "", "auto":
		if env["SERPER_API_KEY"] == "" && deepSeekAvailable {
			return "deepseek", nil
		}
		return legacy, nil
	case "deepseek":
		if !deepSeekAvailable {
			return "", fmt.Errorf("no enabled DeepSeek API-key provider configured")
		}
		return backend, nil
	case legacy:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid %s: %s", key, backend)
	}
}
