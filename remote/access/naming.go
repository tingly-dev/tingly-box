package access

import "strings"

// UsableName returns the trimmed platform-supplied name, or "" when it is not
// a real name: empty, or merely the entity's own external id (some adapters
// echo the id when they have no profile data). Callers treat "" as "the
// platform gave no name" and must not overwrite an existing value with it.
func UsableName(name, externalID string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	id := strings.TrimSpace(externalID)
	if id == "" {
		return name
	}
	if name == id {
		return ""
	}
	if i := strings.Index(id, ":"); i >= 0 && name == id[i+1:] {
		return ""
	}
	return name
}
