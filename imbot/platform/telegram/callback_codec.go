package telegram

import (
	"strconv"
	"strings"
	"sync"

	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Telegram's callback_data is at most 64 bytes, and the limit is enforced on
// the whole outbound message: one oversized button and sendMessage fails with
// BUTTON_DATA_INVALID, so the user sees no message at all rather than a
// message with one button missing. That made it the single most damaging
// platform constraint in the system, and nothing in the codebase checked it.
//
// This file makes the limit unreachable. A payload that fits the flat encoding
// travels as before — same wire bytes, so buttons rendered by older releases
// still resolve. A payload that does not fit is parked in a per-bot vault and
// the button carries a short token instead.
//
// The vault is deliberately in-memory and bounded. Callback payloads describe
// a control on a specific message in a specific conversation; they are worth
// no more durability than the flow that produced them, and every such flow in
// this repository already keeps its state in memory with a timeout. What the
// bound buys is the guarantee that a long-lived bot cannot grow this map
// forever.

const (
	// telegramCallbackDataLimit is Telegram's hard cap on callback_data, in
	// bytes.
	telegramCallbackDataLimit = 64

	// tokenPrefix marks a callback_data that is a vault reference rather than
	// an encoded payload. "@" is reserved: no application payload may begin
	// with it, which encodeCallbackData enforces by tokenizing any payload
	// that does.
	tokenPrefix = "@"

	// vaultCapacity is how many parked payloads a bot keeps. Each entry is a
	// handful of short strings, so this costs little; it is sized to be far
	// beyond the number of buttons a conversation realistically leaves live.
	vaultCapacity = 4096
)

// expiredButtonNotice is shown when a token no longer resolves — the bot has
// restarted, or the button is older than vaultCapacity payloads. Saying so is
// the point: the previous behaviour for an unresolvable button was silence,
// which reads to the user as a broken bot.
const expiredButtonNotice = "This button is no longer active. Send the command again."

// callbackVault parks payloads that cannot be encoded into callback_data and
// hands out short tokens that can.
type callbackVault struct {
	mu       sync.Mutex
	entries  map[string]core.Payload
	order    []string // insertion order, for eviction
	next     uint64
	capacity int
}

func newCallbackVault() *callbackVault {
	return &callbackVault{
		entries:  make(map[string]core.Payload),
		capacity: vaultCapacity,
	}
}

// park stores a payload and returns the token that stands for it.
func (v *callbackVault) park(payload core.Payload) string {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.next++
	token := tokenPrefix + strconv.FormatUint(v.next, 36)

	stored := make(core.Payload, len(payload))
	copy(stored, payload)
	v.entries[token] = stored
	v.order = append(v.order, token)

	if len(v.order) > v.capacity {
		evict := v.order[0]
		v.order = v.order[1:]
		delete(v.entries, evict)
	}
	return token
}

// resolve returns the payload a token stands for.
func (v *callbackVault) resolve(token string) (core.Payload, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	payload, ok := v.entries[token]
	return payload, ok
}

// encodeCallbackData renders a payload as callback_data, parking it when the
// flat encoding will not do. The result is always within Telegram's limit.
//
// Three things force a token: a segment containing the ":" separator (the flat
// encoding cannot round-trip it — this is what the old NUL-escaping worked
// around, at the cost of producing bytes that are invalid inside Feishu's JSON
// button values), an encoding over the byte limit, and a payload that would
// collide with the reserved token prefix.
func (v *callbackVault) encodeCallbackData(payload core.Payload) string {
	if payload.IsEmpty() {
		return ""
	}
	flat := payload.FlatCallbackData()
	if !payload.HasSeparator() &&
		len(flat) <= telegramCallbackDataLimit &&
		!strings.HasPrefix(flat, tokenPrefix) {
		return flat
	}
	return v.park(payload)
}

// decodeCallbackData turns inbound callback_data back into segments. The
// second result is false only for a token that no longer resolves; plain data
// always decodes, including data produced before this encoding existed.
func (v *callbackVault) decodeCallbackData(data string) (core.Payload, bool) {
	if strings.HasPrefix(data, tokenPrefix) {
		return v.resolve(data)
	}
	return core.PayloadFromCallbackData(data), true
}
