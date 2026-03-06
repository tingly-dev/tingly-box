package interaction

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
	"github.com/tingly-dev/tingly-box/imbot/internal/itx"
)

// Re-export interaction types from itx package
type ActionType = itx.ActionType
type InteractionMode = itx.InteractionMode
type Interaction = itx.Interaction
type Option = itx.Option
type InteractionResponse = itx.InteractionResponse

// Re-export constants
const (
	ActionSelect   = itx.ActionSelect
	ActionConfirm  = itx.ActionConfirm
	ActionCancel   = itx.ActionCancel
	ActionNavigate = itx.ActionNavigate
	ActionInput    = itx.ActionInput
	ActionCustom   = itx.ActionCustom

	ModeAuto        = itx.ModeAuto
	ModeInteractive = itx.ModeInteractive
	ModeText        = itx.ModeText
)

// Re-export errors from itx
var (
	ErrNotSupported = itx.ErrNotSupported
)

// InteractionRequest represents a request for user interaction
type InteractionRequest struct {
	ID           string              // Unique request ID
	ChatID       string              // Target chat ID
	Platform     core.Platform       // Target platform
	BotUUID      string              // Bot UUID to use
	Message      string              // Main message text
	ParseMode    core.ParseMode     // Text formatting
	Mode         InteractionMode     // Interaction mode (auto/interactive/text)
	Interactions []Interaction       // Interactive elements
	Timeout      time.Duration       // Request timeout
	Meta         map[string]any      // Additional metadata
}

// Validate validates the interaction request
func (r *InteractionRequest) Validate() error {
	if r.ID == "" {
		return fmt.Errorf("request ID cannot be empty")
	}
	if r.Message == "" {
		return fmt.Errorf("message cannot be empty")
	}
	if r.Mode != "" && !r.Mode.IsValid() {
		return fmt.Errorf("invalid interaction mode: %s", r.Mode)
	}
	if len(r.Interactions) == 0 {
		return fmt.Errorf("at least one interaction is required")
	}
	return nil
}

// Errors
var (
	ErrNotInteraction       = itx.ErrNotInteraction
	ErrBotNotFound          = fmt.Errorf("bot not found")
	ErrNoAdapter            = fmt.Errorf("no adapter for platform")
	ErrRequestNotFound      = itx.ErrRequestNotFound
	ErrRequestExpired       = itx.ErrRequestExpired
	ErrTimeout              = itx.ErrTimeout
	ErrChannelClosed        = itx.ErrChannelClosed
	ErrInvalidMode          = itx.ErrInvalidMode
	ErrPendingRequestNotFound = fmt.Errorf("pending request not found")
)
