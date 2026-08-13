// Package peer implements the Peer resource: an external tool registered on
// tingly-box as if tingly-box were an IM platform. A peer gets an identity in
// chat, a scoped credential, and two verbs — send (outbound, attributed) and
// updates (inbound long-poll stream). See .design/peer.md.
//
// tingly-box never hosts, schedules, or triggers the tool — this package is
// the platform side only: who may pass, as whom, into which chat, and how
// the answer comes home.
package peer

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// TokenPrefix is the scoped-credential prefix, following the tb-user- /
// tb-share- naming family. A tb-peer- token is valid only on its own peer's
// data-plane endpoints.
const TokenPrefix = "tb-peer-"

// CurrentAgentPrefix marks a chat's CurrentAgent as pointing at a peer
// ("peer:<uuid>"). The uuid (not the name) keeps the sticky state stable
// across renames.
const CurrentAgentPrefix = "peer:"

// UpdateTypeMessage is the one v1 update type: a human's chat message routed
// to this peer. Future affordances (inline-button callbacks, membership
// events) arrive as new types in the same stream, never as a second channel
// (spec §5).
const UpdateTypeMessage = "message"

// nameRe is the mention-word shape: short, lowercase, no whitespace — it has
// to read naturally after an @ in chat.
var nameRe = regexp.MustCompile(`^[a-z0-9_-]{2,32}$`)

// reservedNames are mention words already owned by the agent handoff
// (@cc/@tb, plus @mock used by the test harness) and the /peers command.
var reservedNames = map[string]bool{"cc": true, "tb": true, "mock": true, "peers": true}

// Peer is the resource. Its own small table on purpose — not a Scenarios row
// and not a premature BotCapability merge (spec §3).
type Peer struct {
	UUID string `json:"uuid"`
	// Name is the mention word (@name) and the attribution prefix (【name】).
	Name    string `json:"name"`
	BotUUID string `json:"bot_uuid"`
	// ChatID is the bound external chat id — the same identifier the channel
	// layer speaks. The binding IS the authorization: a peer can never reach,
	// or be reached from, any other chat.
	ChatID string `json:"chat_id"`
	// Exclusive routes every plain message in the bound chat to this peer
	// (dedicated-chat mode, addressing tier 1).
	Exclusive bool `json:"exclusive"`
	Enabled   bool `json:"enabled"`
	// TokenHash is the sha256 hex of the tb-peer- token. Plaintext is shown
	// exactly once, at create/rotate.
	TokenHash string `json:"-"`
	// AckedUpdateID is the server-side updates cursor: updates with a greater
	// id are undelivered-or-unconfirmed.
	AckedUpdateID int64     `json:"acked_update_id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// CurrentAgentValue is the CurrentAgent marker for this peer.
func (p Peer) CurrentAgentValue() string { return CurrentAgentPrefix + p.UUID }

// AttributionPrefix is the outbound message marker, e.g. "【report】".
func (p Peer) AttributionPrefix() string { return "【" + p.Name + "】" }

// ValidateName reports whether name is an acceptable mention word.
func ValidateName(name string) error {
	if !nameRe.MatchString(name) {
		return fmt.Errorf("invalid name %q: want 2-32 chars of [a-z0-9_-]", name)
	}
	if reservedNames[name] {
		return fmt.Errorf("name %q is reserved", name)
	}
	return nil
}

// PeerUUIDFromCurrentAgent extracts the peer uuid from a chat's CurrentAgent
// value, or "" when the value is not a peer marker.
func PeerUUIDFromCurrentAgent(agent string) string {
	if !strings.HasPrefix(agent, CurrentAgentPrefix) {
		return ""
	}
	return strings.TrimPrefix(agent, CurrentAgentPrefix)
}

// NewToken mints a fresh scoped token and its storage hash.
func NewToken() (plaintext, hash string, err error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("mint peer token: %w", err)
	}
	plaintext = TokenPrefix + hex.EncodeToString(b[:])
	return plaintext, HashToken(plaintext), nil
}

// HashToken returns the storage hash for a plaintext token.
func HashToken(plaintext string) string {
	sum := sha256.Sum256([]byte(plaintext))
	return hex.EncodeToString(sum[:])
}

// VerifyToken reports whether plaintext matches the stored hash, in constant
// time over the hashes.
func VerifyToken(plaintext, storedHash string) bool {
	if storedHash == "" || !strings.HasPrefix(plaintext, TokenPrefix) {
		return false
	}
	computed := HashToken(plaintext)
	return subtle.ConstantTimeCompare([]byte(computed), []byte(storedHash)) == 1
}

// Update is one entry in a peer's inbound stream: a typed envelope, persisted
// and delivered at-least-once in id order. v1 carries only
// UpdateTypeMessage; the Type field is what lets the stream grow new event
// kinds without a protocol break.
type Update struct {
	ID       int64  `json:"update_id"`
	PeerUUID string `json:"-"`
	BotUUID  string `json:"-"`
	Type     string `json:"type"`
	ChatID   string `json:"chat_id"`
	SenderID string `json:"sender_id"`
	// MessageID is the platform message id of the inbound message; a send
	// with reply_to_update_id threads to it.
	MessageID string `json:"message_id,omitempty"`
	Text      string `json:"text"`
	// ContextToken carries the platform reply-context token (Weixin/WeCom)
	// so a threaded reply is attributed correctly. Opaque passthrough.
	ContextToken string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// MaxQueuedUpdates bounds a peer's unconfirmed backlog. Oldest updates
// beyond the cap are dropped oldest-first (and the drop is logged by the
// store caller) — a tool that never confirms must not grow the table
// forever.
const MaxQueuedUpdates = 1000
