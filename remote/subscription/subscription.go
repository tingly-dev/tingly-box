// Package subscription implements the Subscription resource: a named
// association between an external tool and a bot + chat that gives the tool
// an identity in chat, a scoped credential, an attributed outbound path, and
// an inbound mailbox. See .design/subscription.md.
//
// tingly-box never hosts, schedules, or triggers the tool — this package is
// the switchboard side only: who may pass, as whom, into which chat, and how
// the answer comes home.
package subscription

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
// tb-share- naming family. A tb-sub- token is valid only on its own
// subscription's data-plane endpoints.
const TokenPrefix = "tb-sub-"

// CurrentAgentPrefix marks a chat's CurrentAgent as pointing at a
// subscription ("sub:<uuid>"). The uuid (not the name) keeps the sticky
// state stable across renames.
const CurrentAgentPrefix = "sub:"

// nameRe is the mention-word shape: short, lowercase, no whitespace — it has
// to read naturally after an @ in chat.
var nameRe = regexp.MustCompile(`^[a-z0-9_-]{2,32}$`)

// reservedNames are mention words already owned by the agent handoff
// (@cc/@tb, plus @mock used by the test harness).
var reservedNames = map[string]bool{"cc": true, "tb": true, "mock": true, "subs": true}

// Subscription is the resource. Its own small table on purpose — not a
// Scenarios row and not a premature BotCapability merge (spec §3).
type Subscription struct {
	UUID string `json:"uuid"`
	// Name is the mention word (@name) and the attribution prefix (【name】).
	Name    string `json:"name"`
	BotUUID string `json:"bot_uuid"`
	// ChatID is the bound external chat id — the same identifier the channel
	// layer speaks. The binding IS the authorization: a subscription can
	// never reach, or be reached from, any other chat.
	ChatID string `json:"chat_id"`
	// Exclusive routes every plain message in the bound chat to this
	// subscription (dedicated-chat mode, addressing tier 1).
	Exclusive bool `json:"exclusive"`
	Enabled   bool `json:"enabled"`
	// TokenHash is the sha256 hex of the tb-sub- token. Plaintext is shown
	// exactly once, at create/rotate.
	TokenHash string `json:"-"`
	// AckedEventID is the server-side mailbox cursor: events with a greater
	// id are undelivered-or-unacked.
	AckedEventID int64     `json:"acked_event_id"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// CurrentAgentValue is the CurrentAgent marker for this subscription.
func (s Subscription) CurrentAgentValue() string { return CurrentAgentPrefix + s.UUID }

// AttributionPrefix is the outbound message marker, e.g. "【report】".
func (s Subscription) AttributionPrefix() string { return "【" + s.Name + "】" }

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

// SubscriptionUUIDFromCurrentAgent extracts the subscription uuid from a
// chat's CurrentAgent value, or "" when the value is not a subscription
// marker.
func SubscriptionUUIDFromCurrentAgent(agent string) string {
	if !strings.HasPrefix(agent, CurrentAgentPrefix) {
		return ""
	}
	return strings.TrimPrefix(agent, CurrentAgentPrefix)
}

// NewToken mints a fresh scoped token and its storage hash.
func NewToken() (plaintext, hash string, err error) {
	var b [24]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", "", fmt.Errorf("mint subscription token: %w", err)
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

// Event is one inbound mailbox entry: a chat message addressed to the
// subscription. Persisted; delivered at-least-once in id order.
type Event struct {
	ID               int64     `json:"id"`
	SubscriptionUUID string    `json:"subscription_uuid"`
	BotUUID          string    `json:"bot_uuid"`
	ChatID           string    `json:"chat_id"`
	SenderID         string    `json:"sender_id"`
	// MessageID is the platform message id of the inbound message; reply
	// threads to it.
	MessageID string `json:"message_id,omitempty"`
	Text      string `json:"text"`
	// ContextToken carries the platform reply-context token (Weixin/WeCom)
	// so a threaded reply is attributed correctly. Opaque passthrough.
	ContextToken string    `json:"-"`
	CreatedAt    time.Time `json:"created_at"`
}

// MaxQueuedEvents bounds a subscription's unacked backlog. Oldest events
// beyond the cap are dropped oldest-first (and the drop is logged by the
// store caller) — a tool that never acks must not grow the table forever.
const MaxQueuedEvents = 1000
