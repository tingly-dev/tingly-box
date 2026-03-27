package interaction

import "fmt"

// Errors
var (
	ErrNotSupported    = fmt.Errorf("not supported by platform")
	ErrNotInteraction  = fmt.Errorf("not an interaction response")
	ErrRequestNotFound = fmt.Errorf("pending request not found")
	ErrRequestExpired  = fmt.Errorf("pending request expired")
	ErrTimeout         = fmt.Errorf("request timed out")
	ErrChannelClosed   = fmt.Errorf("response channel closed")
	ErrInvalidMode     = fmt.Errorf("invalid interaction mode for platform")
)

// Errors
var (
	ErrBotNotFound            = fmt.Errorf("bot not found")
	ErrNoAdapter              = fmt.Errorf("no adapter for platform")
	ErrPendingRequestNotFound = fmt.Errorf("pending request not found")
)
