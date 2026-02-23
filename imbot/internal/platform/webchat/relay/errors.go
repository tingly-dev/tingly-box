package relay

import (
	"errors"
)

// Errors
var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionClosed   = errors.New("session closed")
	ErrInvalidBotID    = errors.New("invalid bot ID")
	ErrBotNotFound     = errors.New("bot not found")
)
