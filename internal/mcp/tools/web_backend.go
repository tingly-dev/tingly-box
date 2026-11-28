package tools

import (
	"fmt"
	"strings"
)

// WebBackend selects each operation independently. Unset (or obsolete "auto")
// settings retain the original Serper/Jina defaults, independent of credentials.
func WebBackend(env map[string]string, tool string, deepSeekAvailable bool) (string, error) {
	key, legacy := "WEB_SEARCH_BACKEND", "serper"
	if tool == BuiltinWebFetchToolName {
		key, legacy = "WEB_FETCH_BACKEND", "jina"
	}
	backend := strings.ToLower(strings.TrimSpace(env[key]))
	switch backend {
	case "", "auto":
		return legacy, nil
	case "deepseek":
		if !deepSeekAvailable {
			return backend, fmt.Errorf("selected DeepSeek API-key credential is missing, disabled, or unavailable")
		}
		return backend, nil
	case legacy:
		return backend, nil
	default:
		return "", fmt.Errorf("invalid %s: %s", key, backend)
	}
}
