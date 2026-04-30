// Package pairing provides TOFU (Trust On First Use) pairing for bot chats.
// It wraps the imbot/security implementation and provides bot-specific integration.
package pairing

import (
	"io"
	"time"

	"github.com/tingly-dev/tingly-box/imbot"
	"github.com/tingly-dev/tingly-box/imbot/security"
)

// Manager is an alias for security.PairingManager
type Manager = security.PairingManager

// ManagerOption is an alias for security.PairingManagerOption
type ManagerOption = security.PairingManagerOption

// Errors forwarded from imbot/security
var (
	ErrPairCodeMissing  = security.ErrPairCodeMissing
	ErrPairCodeExpired  = security.ErrPairCodeExpired
	ErrPairCodeMismatch = security.ErrPairCodeMismatch
	ErrPairLocked       = security.ErrPairLocked
)

// NewManager creates a new pairing manager with an optional auditor
func NewManager(auditor security.PairingAuditor, opts ...ManagerOption) *Manager {
	return security.NewPairingManager(auditor, opts...)
}

// WithTTL sets the time-to-live for pairing codes
func WithTTL(ttl time.Duration) ManagerOption {
	return security.WithPairingTTL(ttl)
}

// WithCodeLen sets the length of generated pairing codes
func WithCodeLen(length int) ManagerOption {
	return security.WithPairingCodeLength(length)
}

// WithMaxFails sets the maximum number of failed attempts before lockout
func WithMaxFails(max int) ManagerOption {
	return security.WithPairingMaxFails(max)
}

// WithLockout sets the lockout duration after max failures
func WithLockout(duration time.Duration) ManagerOption {
	return security.WithPairingLockout(duration)
}

// WithRand sets the random source for code generation
func WithRand(reader io.Reader) ManagerOption {
	return security.WithPairingRand(reader)
}

// WithClock sets the clock for time calculations
func WithClock(fn func() time.Time) ManagerOption {
	return security.WithPairingClock(fn)
}

// BotIntegration provides bot-specific pairing integration
type BotIntegration struct {
	manager *Manager
	imbot   *imbot.Manager
}

// NewBotIntegration creates a new bot integration for pairing
func NewBotIntegration(manager *Manager, imbot *imbot.Manager) *BotIntegration {
	return &BotIntegration{
		manager: manager,
		imbot:   imbot,
	}
}

// Mint generates a new pairing code for the given bot
func (b *BotIntegration) Mint(botUUID string) (code string, expiresAt time.Time) {
	return b.manager.Mint(botUUID)
}

// Verify verifies a pairing code and pairs the chat if valid
func (b *BotIntegration) Verify(botUUID, code string) error {
	return b.manager.Verify(botUUID, code)
}

// GetManager returns the underlying pairing manager
func (b *BotIntegration) GetManager() *Manager {
	return b.manager
}
