// Package security provides reusable security primitives for imbot applications.
//
// PairingManager implements trust-on-first-use (TOFU) pairing: it mints
// short, time-limited, single-use codes per bot UUID and verifies them with
// constant-time comparison and per-bot rate-limiting / lockout. Codes are
// kept only in memory — a process restart deliberately invalidates any
// outstanding codes.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base32"
	"errors"
	"io"
	"strings"
	"sync"
	"time"
)

// Sentinel errors returned by PairingManager.Verify.
var (
	ErrPairCodeMissing  = errors.New("no pairing code is currently active for this bot")
	ErrPairCodeExpired  = errors.New("pairing code has expired")
	ErrPairCodeMismatch = errors.New("pairing code does not match")
	ErrPairLocked       = errors.New("too many failed attempts, try again later")
)

// PairingAuditor is the minimal interface used by PairingManager to emit
// structured pairing events. LogAuditor (this package) is the production
// implementation, backed by the regular application logger.
type PairingAuditor interface {
	Info(action, userID, clientIP, message string, details map[string]interface{})
	Warn(action, userID, clientIP, message string, details map[string]interface{})
}

// PairingManagerOption configures PairingManager construction.
type PairingManagerOption func(*PairingManager)

// WithPairingTTL sets the lifetime of a freshly minted code.
func WithPairingTTL(d time.Duration) PairingManagerOption {
	return func(p *PairingManager) {
		if d > 0 {
			p.ttl = d
		}
	}
}

// WithPairingCodeLength sets the number of base32 characters used for codes.
func WithPairingCodeLength(n int) PairingManagerOption {
	return func(p *PairingManager) {
		if n >= 4 {
			p.codeLen = n
		}
	}
}

// WithPairingMaxFails sets the failure threshold before a lockout starts.
func WithPairingMaxFails(n int) PairingManagerOption {
	return func(p *PairingManager) {
		if n >= 1 {
			p.maxFails = n
		}
	}
}

// WithPairingLockout sets how long a bot stays locked after threshold breach.
func WithPairingLockout(d time.Duration) PairingManagerOption {
	return func(p *PairingManager) {
		if d > 0 {
			p.lockoutFor = d
		}
	}
}

// WithPairingRand overrides the entropy source (for tests).
func WithPairingRand(r io.Reader) PairingManagerOption {
	return func(p *PairingManager) {
		if r != nil {
			p.rng = r
		}
	}
}

// WithPairingClock overrides the time source (for tests).
func WithPairingClock(now func() time.Time) PairingManagerOption {
	return func(p *PairingManager) {
		if now != nil {
			p.now = now
		}
	}
}

// PairingManager owns the in-memory state for active pairing codes.
type PairingManager struct {
	mu         sync.Mutex
	codes      map[string]*pairEntry    // botUUID -> active code
	attempts   map[string]*attemptState // botUUID -> failure / lockout state
	ttl        time.Duration
	codeLen    int
	maxFails   int
	lockoutFor time.Duration
	rng        io.Reader
	now        func() time.Time
	audit      PairingAuditor
}

type pairEntry struct {
	code      string
	expiresAt time.Time
}

type attemptState struct {
	fails         int
	lockedUntil   time.Time
	lockoutLogged bool
}

// NewPairingManager constructs a manager with sensible defaults.
// auditLog may be nil; if non-nil it must implement PairingAuditor.
func NewPairingManager(auditLog PairingAuditor, opts ...PairingManagerOption) *PairingManager {
	p := &PairingManager{
		codes:      make(map[string]*pairEntry),
		attempts:   make(map[string]*attemptState),
		ttl:        10 * time.Minute,
		codeLen:    8,
		maxFails:   5,
		lockoutFor: 10 * time.Minute,
		rng:        rand.Reader,
		now:        func() time.Time { return time.Now().UTC() },
		audit:      auditLog,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Mint generates and returns a fresh pairing code for the given bot. Any
// previous code or failure state is discarded.
func (p *PairingManager) Mint(botUUID string) (string, time.Time) {
	if botUUID == "" {
		return "", time.Time{}
	}
	code := p.generateCode()
	expires := p.now().Add(p.ttl)

	p.mu.Lock()
	p.codes[botUUID] = &pairEntry{code: code, expiresAt: expires}
	delete(p.attempts, botUUID)
	p.mu.Unlock()

	p.auditInfo("imbot.pair.code_minted", "", "pairing code minted",
		map[string]interface{}{
			"bot_uuid":   botUUID,
			"ttl":        p.ttl.String(),
			"expires_at": expires.Format(time.RFC3339),
		})
	return code, expires
}

// Current returns the active code for a bot, if any.
func (p *PairingManager) Current(botUUID string) (string, time.Time, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	entry, ok := p.codes[botUUID]
	if !ok || entry == nil {
		return "", time.Time{}, false
	}
	if !p.now().Before(entry.expiresAt) {
		delete(p.codes, botUUID)
		return "", time.Time{}, false
	}
	return entry.code, entry.expiresAt, true
}

// Verify checks a candidate code and, on success, consumes it (single-use).
// Caller is responsible for recording the binding in the chat store.
func (p *PairingManager) Verify(botUUID, candidate string) error {
	if botUUID == "" {
		return ErrPairCodeMissing
	}
	candidate = strings.ToUpper(strings.TrimSpace(candidate))

	p.mu.Lock()
	now := p.now()

	// Check lockout first.
	if att, ok := p.attempts[botUUID]; ok && att != nil && now.Before(att.lockedUntil) {
		p.mu.Unlock()
		p.auditWarn("imbot.pair.fail", "", "verify rejected by lockout",
			map[string]interface{}{
				"bot_uuid":     botUUID,
				"reason":       "locked",
				"locked_until": att.lockedUntil.Format(time.RFC3339),
			})
		return ErrPairLocked
	}

	entry, ok := p.codes[botUUID]
	if !ok || entry == nil {
		p.mu.Unlock()
		p.recordFailure(botUUID, "missing")
		return ErrPairCodeMissing
	}
	if !now.Before(entry.expiresAt) {
		delete(p.codes, botUUID)
		p.mu.Unlock()
		p.recordFailure(botUUID, "expired")
		return ErrPairCodeExpired
	}

	expected := []byte(entry.code)
	got := []byte(candidate)
	// Pad to equal length for constant-time compare without leaking length.
	maxLen := len(expected)
	if len(got) > maxLen {
		maxLen = len(got)
	}
	exp := make([]byte, maxLen)
	cand := make([]byte, maxLen)
	copy(exp, expected)
	copy(cand, got)

	if subtle.ConstantTimeCompare(exp, cand) != 1 || len(expected) != len(got) {
		p.mu.Unlock()
		p.recordFailure(botUUID, "mismatch")
		return ErrPairCodeMismatch
	}

	// Success: consume code (single-use) and clear attempts.
	delete(p.codes, botUUID)
	delete(p.attempts, botUUID)
	p.mu.Unlock()
	return nil
}

// Revoke discards any active code and resets failure state.
func (p *PairingManager) Revoke(botUUID string) {
	if botUUID == "" {
		return
	}
	p.mu.Lock()
	delete(p.codes, botUUID)
	delete(p.attempts, botUUID)
	p.mu.Unlock()
}

func (p *PairingManager) recordFailure(botUUID, reason string) {
	p.mu.Lock()
	att, ok := p.attempts[botUUID]
	if !ok || att == nil {
		att = &attemptState{}
		p.attempts[botUUID] = att
	}
	att.fails++
	locked := false
	if att.fails >= p.maxFails && att.lockedUntil.IsZero() {
		att.lockedUntil = p.now().Add(p.lockoutFor)
		locked = !att.lockoutLogged
		att.lockoutLogged = true
	}
	fails := att.fails
	until := att.lockedUntil
	p.mu.Unlock()

	p.auditWarn("imbot.pair.fail", "", "verify failed",
		map[string]interface{}{
			"bot_uuid": botUUID,
			"reason":   reason,
			"fails":    fails,
		})
	if locked {
		p.auditWarn("imbot.pair.locked", "", "pairing locked after repeated failures",
			map[string]interface{}{
				"bot_uuid":     botUUID,
				"locked_until": until.Format(time.RFC3339),
			})
	}
}

func (p *PairingManager) auditInfo(action, userID, message string, details map[string]interface{}) {
	if p.audit != nil {
		p.audit.Info(action, userID, "", message, details)
	}
}

func (p *PairingManager) auditWarn(action, userID, message string, details map[string]interface{}) {
	if p.audit != nil {
		p.audit.Warn(action, userID, "", message, details)
	}
}

// generateCode produces a base32 (Crockford-like) uppercase code with a
// dash inserted in the middle for human readability.
func (p *PairingManager) generateCode() string {
	// Read enough bytes for the requested length. base32 encodes 5 bytes
	// into 8 characters; pad upward and trim.
	n := (p.codeLen*5 + 7) / 8
	if n < 5 {
		n = 5
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(p.rng, buf); err != nil {
		// Fallback: not great, but never empty so callers still get a code.
		// crypto/rand should not fail in practice.
		now := p.now().UnixNano()
		for i := range buf {
			buf[i] = byte(now >> (uint(i%8) * 8))
		}
	}
	enc := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(buf)
	enc = strings.ToUpper(enc)
	// Replace easy-to-confuse characters.
	enc = strings.NewReplacer("0", "Z", "1", "Y", "8", "X").Replace(enc)
	if len(enc) > p.codeLen {
		enc = enc[:p.codeLen]
	}
	if p.codeLen >= 8 {
		mid := p.codeLen / 2
		return enc[:mid] + "-" + enc[mid:]
	}
	return enc
}
