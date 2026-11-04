package access

import "strings"

// Alias and display-name provenance. A user-set value is authoritative and is
// never replaced by what a platform reports; a platform-sourced value is only a
// default that a later, different platform name may refresh.
const (
	AliasSourceUser     = "user"
	AliasSourcePlatform = "platform"
)

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

// PlatformAliasUpdate decides how an inbound platform name affects an alias.
// It returns the alias and source to store and whether anything changes.
//
//   - no usable name: nothing changes (an empty name never wipes a value);
//   - an alias that is user-set, or of unknown origin, is kept (the platform
//     name only refreshes a cache the caller keeps elsewhere);
//   - otherwise the platform name becomes the default alias.
func PlatformAliasUpdate(currentAlias, currentSource, name, externalID string) (alias, source string, changed bool) {
	usable := UsableName(name, externalID)
	if usable == "" {
		return currentAlias, currentSource, false
	}
	// A non-empty alias whose source is not "platform" is the user's (or of
	// unknown origin): protect it. Only an empty alias or one we know came from
	// the platform may be replaced.
	if currentAlias != "" && currentSource != AliasSourcePlatform {
		return currentAlias, currentSource, false
	}
	if currentAlias == usable && currentSource == AliasSourcePlatform {
		return currentAlias, currentSource, false
	}
	return usable, AliasSourcePlatform, true
}
