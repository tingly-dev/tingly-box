package project

import "slices"

// The thinking-effort levels a project can give its coordinator and its
// threads, the same ladder the gateway's rules speak (internal/protocol/
// thinking). "minimal" and "xhigh" are left out like the routing UI's own
// effort picker: outside a handful of the newest models they silently
// collapse onto "low"/"high" server-side.

// EffortLevels lists the selectable levels, default first.
var EffortLevels = []string{"", "low", "medium", "high", "max"}

// ValidEffort reports whether e is a level of the ladder; empty is valid.
func ValidEffort(e string) bool {
	return slices.Contains(EffortLevels, e)
}
