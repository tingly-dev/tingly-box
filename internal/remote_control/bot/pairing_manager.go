// PairingManager and related types live in imbot/security so that any imbot
// application can reuse the TOFU pairing mechanism independently of the
// remote-control service. The aliases below keep existing code in this
// package unchanged.
package bot

import "github.com/tingly-dev/tingly-box/imbot/security"

// Type aliases — fully transparent to callers.
type PairingManager = security.PairingManager
type PairingManagerOption = security.PairingManagerOption

// Error sentinels forwarded from imbot/security.
var (
	ErrPairCodeMissing  = security.ErrPairCodeMissing
	ErrPairCodeExpired  = security.ErrPairCodeExpired
	ErrPairCodeMismatch = security.ErrPairCodeMismatch
	ErrPairLocked       = security.ErrPairLocked
)

// Constructor and option helpers forwarded from imbot/security.
var (
	NewPairingManager   = security.NewPairingManager
	NewLogAuditor       = security.NewLogAuditor
	WithPairingTTL      = security.WithPairingTTL
	WithPairingCodeLen  = security.WithPairingCodeLength
	WithPairingMaxFails = security.WithPairingMaxFails
	WithPairingLockout  = security.WithPairingLockout
	WithPairingRand     = security.WithPairingRand
	WithPairingClock    = security.WithPairingClock
)
