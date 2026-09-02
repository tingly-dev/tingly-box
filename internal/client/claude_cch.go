package client

import (
	"bytes"
	"fmt"
	"io"
	"math/bits"
	"net/http"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/option"
)

// cch: Claude Code's request-body hash in the billing header.
//
// The JS bundle renders "cch=00000;" as a placeholder; the native (Bun/Zig)
// layer of the official binary rewrites it on the wire with a hash of the
// outgoing body. Reverse-engineered and verified against three live
// captures of the 2.1.258 Linux binary (.design/claude-code-client-compat.md
// §3.3.4), the rule is:
//
//	preimage = final wire JSON (placeholder still in place)
//	           with top-level "model" set to "" and "max_tokens" removed
//	cch      = xxHash64(preimage, seed 0x4D659218E32A3268) & 0xFFFFF, 5 lowercase hex
//
// Two traps: the seed is release-specific (unchanged since 2.1.138), and the
// hash is Bun/Zig's xxHash64 whose PRIME64_4 differs from the reference
// implementation (0x85EBCA77C2B2AE63 instead of 0x85EBCA6B3B7B36EF) — a
// stock xxhash library computes the wrong value.
//
// tingly-box mirrors the official split: ops emits the placeholder in the
// system block, and this file's middleware hashes the exact bytes the Go SDK
// is about to send and patches the placeholder in place. Hashing our own
// bytes makes the value independent of the SDK's key order; the body is
// first normalized to JavaScript's JSON.stringify escaping so a server-side
// re-serialization (if any) sees the same bytes we hashed.

const (
	// claudeCodeCCHPlaceholder is the exact substring ops emits and the
	// native layer replaces.
	claudeCodeCCHPlaceholder = "cch=00000;"

	// claudeCodeCCHSeed is the xxHash64 seed baked into the 2.1.138+ binaries.
	claudeCodeCCHSeed uint64 = 0x4D659218E32A3268

	xxh64Prime1    uint64 = 0x9E3779B185EBCA87
	xxh64Prime2    uint64 = 0xC2B2AE3D27D4EB4F
	xxh64Prime3    uint64 = 0x165667B19E3779F9
	xxh64Prime4Zig uint64 = 0x85EBCA77C2B2AE63 // Bun/Zig variant; reference is 0x85EBCA6B3B7B36EF
	xxh64Prime5    uint64 = 0x27D4EB2F165667C5
)

// xxhash64Zig is xxHash64 with Bun/Zig's PRIME64_4 constant.
func xxhash64Zig(data []byte, seed uint64) uint64 {
	round := func(acc, input uint64) uint64 {
		acc += input * xxh64Prime2
		acc = bits.RotateLeft64(acc, 31)
		return acc * xxh64Prime1
	}
	merge := func(acc, val uint64) uint64 {
		acc ^= round(0, val)
		return acc*xxh64Prime1 + xxh64Prime4Zig
	}
	le := func(b []byte) uint64 {
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24 |
			uint64(b[4])<<32 | uint64(b[5])<<40 | uint64(b[6])<<48 | uint64(b[7])<<56
	}
	le32 := func(b []byte) uint64 {
		return uint64(b[0]) | uint64(b[1])<<8 | uint64(b[2])<<16 | uint64(b[3])<<24
	}

	n := len(data)
	i := 0
	var h uint64
	if n >= 32 {
		v1 := seed + xxh64Prime1 + xxh64Prime2
		v2 := seed + xxh64Prime2
		v3 := seed
		v4 := seed - xxh64Prime1
		for ; i+32 <= n; i += 32 {
			v1 = round(v1, le(data[i:]))
			v2 = round(v2, le(data[i+8:]))
			v3 = round(v3, le(data[i+16:]))
			v4 = round(v4, le(data[i+24:]))
		}
		h = bits.RotateLeft64(v1, 1) + bits.RotateLeft64(v2, 7) + bits.RotateLeft64(v3, 12) + bits.RotateLeft64(v4, 18)
		h = merge(h, v1)
		h = merge(h, v2)
		h = merge(h, v3)
		h = merge(h, v4)
	} else {
		h = seed + xxh64Prime5
	}
	h += uint64(n)
	for ; i+8 <= n; i += 8 {
		h ^= round(0, le(data[i:]))
		h = bits.RotateLeft64(h, 27)*xxh64Prime1 + xxh64Prime4Zig
	}
	if i+4 <= n {
		h ^= le32(data[i:]) * xxh64Prime1
		h = bits.RotateLeft64(h, 23)*xxh64Prime2 + xxh64Prime3
		i += 4
	}
	for ; i < n; i++ {
		h ^= uint64(data[i]) * xxh64Prime5
		h = bits.RotateLeft64(h, 11) * xxh64Prime1
	}
	h ^= h >> 33
	h *= xxh64Prime2
	h ^= h >> 29
	h *= xxh64Prime3
	h ^= h >> 32
	return h
}

// formatClaudeCodeCCH renders the low 20 bits as the 5-hex cch value.
func formatClaudeCodeCCH(h uint64) string {
	return fmt.Sprintf("%05x", h&0xFFFFF)
}

// canonicalizeJSONEscapes rewrites the escape sequences Go's encoder emits
// but JavaScript's JSON.stringify does not, so the bytes on the wire (and in
// the hash preimage) match what a JS client would have produced:
// < > & (HTML-safe escaping) and U+2028 / U+2029 (line separators). Only
// sequences that are real escapes (odd number of preceding backslashes) are
// touched; the text "\\u003c" inside a string is left alone.
func canonicalizeJSONEscapes(body []byte) []byte {
	if !bytes.Contains(body, []byte(`\u`)) {
		return body
	}
	out := make([]byte, 0, len(body))
	backslashes := 0
	for i := 0; i < len(body); i++ {
		c := body[i]
		if c == '\\' {
			backslashes++
			if backslashes%2 == 1 && i+5 < len(body) && body[i+1] == 'u' {
				var repl string
				switch string(body[i+2 : i+6]) {
				case "003c", "003C":
					repl = "<"
				case "003e", "003E":
					repl = ">"
				case "0026":
					repl = "&"
				case "2028":
					repl = "\u2028"
				case "2029":
					repl = "\u2029"
				}
				if repl != "" {
					out = append(out, repl...)
					i += 5
					backslashes = 0
					continue
				}
			}
			out = append(out, c)
			continue
		}
		backslashes = 0
		out = append(out, c)
	}
	return out
}

// jsonMember is a top-level object member located by scanTopLevelMembers.
type jsonMember struct {
	key                string
	start, valueStart  int // start of the key token; start of the value token
	end                int // one past the value token
	prevComma, nextIdx int // index of the comma before the member (-1 if none); index after the trailing comma (or end)
}

// scanTopLevelMembers walks a compact-or-not JSON object and returns its
// top-level members with byte spans. It never allocates a parse tree, so it is
// cheap even on 100 KB bodies. Returns nil if the input is not an object.
func scanTopLevelMembers(body []byte) []jsonMember {
	n := len(body)
	i := 0
	skipWS := func() {
		for i < n && (body[i] == ' ' || body[i] == '\n' || body[i] == '\r' || body[i] == '\t') {
			i++
		}
	}
	// skipString advances i past a JSON string starting at body[i] == '"'.
	skipString := func() bool {
		if i >= n || body[i] != '"' {
			return false
		}
		i++
		for i < n {
			switch body[i] {
			case '\\':
				i += 2
				continue
			case '"':
				i++
				return true
			}
			i++
		}
		return false
	}
	// skipValue advances i past any JSON value.
	skipValue := func() bool {
		if i >= n {
			return false
		}
		switch body[i] {
		case '"':
			return skipString()
		case '{', '[':
			depth := 0
			for i < n {
				switch body[i] {
				case '"':
					if !skipString() {
						return false
					}
					continue
				case '{', '[':
					depth++
				case '}', ']':
					depth--
					if depth == 0 {
						i++
						return true
					}
				}
				i++
			}
			return false
		default:
			for i < n && body[i] != ',' && body[i] != '}' && body[i] != ']' &&
				body[i] != ' ' && body[i] != '\n' && body[i] != '\r' && body[i] != '\t' {
				i++
			}
			return true
		}
	}

	skipWS()
	if i >= n || body[i] != '{' {
		return nil
	}
	i++
	var members []jsonMember
	prevComma := -1
	for {
		skipWS()
		if i >= n {
			return nil
		}
		if body[i] == '}' {
			return members
		}
		m := jsonMember{start: i, prevComma: prevComma}
		keyStart := i
		if !skipString() {
			return nil
		}
		m.key = string(body[keyStart+1 : i-1])
		skipWS()
		if i >= n || body[i] != ':' {
			return nil
		}
		i++
		skipWS()
		m.valueStart = i
		if !skipValue() {
			return nil
		}
		m.end = i
		skipWS()
		if i >= n {
			return nil
		}
		m.nextIdx = i
		if body[i] == ',' {
			prevComma = i
			i++
			m.nextIdx = i
		} else {
			prevComma = -1
		}
		members = append(members, m)
	}
}

// claudeCodeCCHPreimage applies the native layer's edits to the wire body:
// top-level "model" becomes "" and the top-level "max_tokens" member is
// removed. Any other body is returned unchanged.
func claudeCodeCCHPreimage(body []byte) []byte {
	members := scanTopLevelMembers(body)
	if members == nil {
		return body
	}
	out := make([]byte, 0, len(body))
	last := 0
	for _, m := range members {
		switch m.key {
		case "model":
			out = append(out, body[last:m.valueStart]...)
			out = append(out, `""`...)
			last = m.end
		case "max_tokens":
			if m.nextIdx > m.end && body[m.nextIdx-1] == ',' {
				// "max_tokens":N, → drop through the trailing comma
				out = append(out, body[last:m.start]...)
				last = m.nextIdx
			} else if m.prevComma >= 0 {
				// ,"max_tokens":N} → drop from the preceding comma
				out = append(out, body[last:m.prevComma]...)
				last = m.end
			} else {
				out = append(out, body[last:m.start]...)
				last = m.end
			}
		}
	}
	out = append(out, body[last:]...)
	return out
}

// rewriteClaudeCodeCCH canonicalizes body and, when it carries the
// placeholder, returns the body with the computed cch patched in. ok is false
// when there was nothing to rewrite (the canonicalized body is still returned).
func rewriteClaudeCodeCCH(body []byte) (out []byte, cch string, ok bool) {
	out = canonicalizeJSONEscapes(body)
	idx := bytes.Index(out, []byte(claudeCodeCCHPlaceholder))
	if idx < 0 {
		return out, "", false
	}
	cch = formatClaudeCodeCCH(xxhash64Zig(claudeCodeCCHPreimage(out), claudeCodeCCHSeed))
	patched := make([]byte, len(out))
	copy(patched, out)
	copy(patched[idx+len("cch="):], cch)
	return patched, cch, true
}

// claudeCodeCCHMiddleware is the SDK middleware that performs the rewrite on
// the outgoing request, exactly where the official binary does it.
func claudeCodeCCHMiddleware(req *http.Request, next anthropicOption.MiddlewareNext) (*http.Response, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return next(req)
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	out, _, _ := rewriteClaudeCodeCCH(body)
	req.Body = io.NopCloser(bytes.NewReader(out))
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(out)), nil }
	req.ContentLength = int64(len(out))
	return next(req)
}
