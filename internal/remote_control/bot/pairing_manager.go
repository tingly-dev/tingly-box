// PairingManager and related types live in imbot/security so that any imbot
// application can reuse the TOFU pairing mechanism independently of the
// remote-control service. The aliases below keep existing code in this
// package unchanged.
package bot

import "github.com/tingly-dev/tingly-box/imbot/security"

// Type alias — fully transparent to callers.
type PairingManager = security.PairingManager

// Error sentinels forwarded from imbot/security.
var (
	ErrPairCodeMissing  = security.ErrPairCodeMissing
	ErrPairCodeExpired  = security.ErrPairCodeExpired
	ErrPairCodeMismatch = security.ErrPairCodeMismatch
	ErrPairLocked       = security.ErrPairLocked
)

// Constructor helpers forwarded from imbot/security. Callers needing the
// tuning options (TTL, code length, …) should use imbot/security directly.
var (
	NewPairingManager = security.NewPairingManager
	NewLogAuditor     = security.NewLogAuditor
)
