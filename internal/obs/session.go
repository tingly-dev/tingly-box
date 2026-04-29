package obs

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

// SessionShort returns a privacy-safe 16-hex-char identifier and source label for a session.
// Returns ("", "") when the session is empty.
func SessionShort(sid typ.SessionID) (short, source string) {
	if sid.IsEmpty() {
		return "", ""
	}
	source = string(sid.Source)
	h := sha256.Sum256([]byte(sid.Value))
	short = hex.EncodeToString(h[:])[:16]
	return
}
