// PairingManager and related types live in imbot/core so that any imbot
// application can reuse the TOFU pairing mechanism independently of the
// remote-control service. The aliases below keep existing code in this
// package unchanged.
package bot

import (
	"github.com/tingly-dev/tingly-box/imbot/core"
)

// Type alias — fully transparent to callers.
type PairingManager = core.PairingManager

// Error sentinels forwarded from imbot/core.
var (
	ErrPairCodeMissing  = core.ErrPairCodeMissing
	ErrPairCodeExpired  = core.ErrPairCodeExpired
	ErrPairCodeMismatch = core.ErrPairCodeMismatch
	ErrPairLocked       = core.ErrPairLocked
)

// NewPairingManager is forwarded from imbot/core. Callers needing the tuning
// options (TTL, code length, …) should use imbot/core directly.
var NewPairingManager = core.NewPairingManager
