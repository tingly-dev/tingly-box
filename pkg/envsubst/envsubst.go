// Package envsubst expands ${VAR} and $VAR references in strings against a
// caller-supplied lookup. It mirrors the syntax of [os.Expand] (both braced
// ${VAR} and bare $VAR forms, with VAR = [A-Za-z_][A-Za-z0-9_]*), but differs
// from the standard library in two ways that callers in this repo rely on:
//
//   - An unset variable (lookup returns ok=false) is left as the literal
//     reference (${"${VAR}"} or $VAR) rather than replaced with the empty
//     string. This lets callers surface "VAR was not set" instead of silently
//     substituting nothing.
//   - Expand returns the sorted list of variable names that were unset, so the
//     caller can report them. (os.Expand's mapper has no ok return and thus
//     cannot distinguish "empty value" from "unset".)
//
// Use ExpandOS for the common case of resolving against [os.LookupEnv] with no
// interest in the unset list.
package envsubst

import (
	"os"
	"sort"
	"strings"
)

// Expand replaces ${VAR} and $VAR references in s using lookup to resolve each
// name. A name that lookup reports as unset (ok=false) is left as the original
// literal reference and is included in the returned missing slice (sorted,
// de-duplicated). The expanded string and the missing list are returned.
//
// lookup mirrors os.LookupEnv's contract: returning (value, true) substitutes
// value (which may itself be empty); returning (_, false) marks the name unset.
func Expand(s string, lookup func(name string) (string, bool)) (string, []string) {
	if !strings.ContainsRune(s, '$') {
		return s, nil
	}
	var b strings.Builder
	b.Grow(len(s))
	missing := map[string]struct{}{}

	for i := 0; i < len(s); {
		if s[i] != '$' {
			b.WriteByte(s[i])
			i++
			continue
		}
		// s[i] == '$': try to read a reference starting here.
		ref, name, rest := readRef(s, i)
		if ref == "" {
			// Not a reference (e.g. "$5", "$" at end-of-string, "${1abc}").
			// Emit the '$' literally and continue from the next byte.
			b.WriteByte(s[i])
			i++
			continue
		}
		if v, ok := lookup(name); ok {
			b.WriteString(v)
		} else {
			b.WriteString(ref) // leave literal
			missing[name] = struct{}{}
		}
		i = rest
	}

	var miss []string
	if len(missing) > 0 {
		miss = make([]string, 0, len(missing))
		for name := range missing {
			miss = append(miss, name)
		}
		sort.Strings(miss)
	}
	return b.String(), miss
}

// ExpandOS is a convenience wrapper around Expand that resolves references
// against the process environment ([os.LookupEnv]) and returns only the
// expanded string. Unset variables are left as their literal reference.
func ExpandOS(s string) string {
	out, _ := Expand(s, os.LookupEnv)
	return out
}

// readRef attempts to read a ${VAR} or $VAR reference beginning at s[i] (which
// must be '$'). Returns:
//   - ref:  the full original reference text (e.g. "${FOO}", "$FOO"), or "" if
//           the bytes at s[i] are not a valid reference;
//   - name: the extracted variable name (e.g. "FOO");
//   - rest: the index just past the reference (where to continue scanning).
//
// VAR must match [A-Za-z_][A-Za-z0-9_]*. For the braced form the closing '}' is
// required and the name must be non-empty; otherwise the '$' is treated as a
// literal and readRef returns ("", "", i).
func readRef(s string, i int) (ref, name string, rest int) {
	// s[i] == '$'. Caller guarantees len(s) > i.
	if i+1 >= len(s) {
		return "", "", i + 1
	}
	if s[i+1] == '{' {
		// Braced form: ${NAME}. NAME must start with a letter/underscore.
		j := i + 2
		start := j
		if j >= len(s) || !isNameStart(s[j]) {
			// "${" with no valid name start — not a reference.
			return "", "", i + 1
		}
		for j < len(s) && isNameByte(s[j]) {
			j++
		}
		if j >= len(s) || s[j] != '}' {
			// No closing brace — not a reference.
			return "", "", i + 1
		}
		return s[i : j+1], s[start:j], j + 1
	}
	// Bare form: $NAME
	if !isNameStart(s[i+1]) {
		return "", "", i + 1
	}
	j := i + 1
	start := j
	for j < len(s) && isNameByte(s[j]) {
		j++
	}
	return s[i:j], s[start:j], j
}

func isNameStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isNameByte(c byte) bool {
	return isNameStart(c) || (c >= '0' && c <= '9')
}
