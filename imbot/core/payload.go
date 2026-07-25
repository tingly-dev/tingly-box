package core

import "strings"

// Payload is a button's identity: the ordered segments a platform hands back
// when the user taps it.
//
// It exists because the previous identity — Telegram's callback_data — was a
// transport encoding, not an identity. That encoding is a colon-joined string
// capped at 64 bytes, and treating it as the neutral model leaked Telegram's
// budget into every platform and into the application's own data model:
//
//   - Any payload above 64 bytes made Telegram reject the entire message, not
//     just the button. Nothing validated it, so a long project path took the
//     whole reply down with a BUTTON_DATA_INVALID the user never saw.
//   - Because ":" was the separator, a path containing ":" had to be escaped;
//     the escape used a NUL byte, which is invalid inside the JSON that Feishu
//     button values are made of.
//   - The directory browser could not fit a path at all, so it sent array
//     indices and kept a server-side snapshot to resolve them — a whole
//     state machine that existed only to satisfy Telegram, on every platform.
//
// With identity separated from encoding, each platform delivers segments its
// own way: Feishu puts them in the button's JSON value, Telegram joins them
// when they fit and swaps in a short token when they do not.
//
// # Why segments rather than a map
//
// Every dispatch site in this repository is positional — "bind", "dir", path.
// Segments model that exactly, so the change stays mechanical in the one step
// that must not break: a mis-migrated button is a button that silently does
// nothing. A map[string]any would buy named fields, but the names would be
// invented rather than discovered, and it would rewrite every producer and
// consumer at once. Encoding stays platform business either way, which is the
// property that actually matters here.
type Payload []string

// NewPayload builds a payload from its segments.
func NewPayload(segments ...string) Payload {
	return Payload(segments)
}

// Name returns the first segment, which by convention names the action's
// namespace ("bind", "perm", "project"). It is "" for an empty payload, so
// callers can switch on it without a length check.
func (p Payload) Name() string {
	return p.Arg(0)
}

// Arg returns segment i, or "" when it is absent. Bounds-safe by design: a
// stale button from an older release carrying fewer segments should read as
// missing data, not panic the receiving handler.
func (p Payload) Arg(i int) string {
	if i < 0 || i >= len(p) {
		return ""
	}
	return p[i]
}

// IsEmpty reports whether the payload carries nothing.
func (p Payload) IsEmpty() bool {
	return len(p) == 0
}

// legacySeparator is the character the flat callback_data encoding joins on.
// It is Telegram's convention, inherited by every platform that copied the
// shape; it is defined here only so the compatibility helpers below agree.
const legacySeparator = ":"

// FlatCallbackData renders the payload in the historical colon-joined form.
//
// This is a compatibility shim for surfaces that still speak the flat string —
// the interaction and menu packages, and the callback_data metadata key that
// existing consumers read. It is lossy whenever a segment contains ":", which
// is exactly why it is not how payloads travel any more. Platforms encoding
// for the wire must handle that case rather than call this.
func (p Payload) FlatCallbackData() string {
	return strings.Join(p, legacySeparator)
}

// HasSeparator reports whether any segment contains the legacy separator, in
// which case the flat encoding cannot round-trip and the platform must deliver
// the segments some other way.
func (p Payload) HasSeparator() bool {
	for _, seg := range p {
		if strings.Contains(seg, legacySeparator) {
			return true
		}
	}
	return false
}

// PayloadFromCallbackData parses the historical flat encoding back into
// segments. Used on inbound paths that still receive a flat string, and to
// interpret Action.CallbackData from producers that have not migrated.
func PayloadFromCallbackData(data string) Payload {
	if data == "" {
		return nil
	}
	return Payload(strings.Split(data, legacySeparator))
}
