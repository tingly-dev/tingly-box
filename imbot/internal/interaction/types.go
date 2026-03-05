package interaction

import (
	"fmt"
	"time"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// ActionType represents the type of user action
type ActionType string

const (
	ActionSelect   ActionType = "select"   // User selected an option
	ActionConfirm  ActionType = "confirm"  // User confirmed yes/no
	ActionCancel   ActionType = "cancel"   // User cancelled
	ActionNavigate ActionType = "navigate" // User navigated (prev/next)
	ActionInput    ActionType = "input"    // User provided text input
	ActionCustom   ActionType = "custom"   // Custom action
)

// InteractionMode controls how interactions are presented to users
type InteractionMode string

const (
	// ModeAuto automatically chooses the best available mode for the platform
	// - Uses native interactions if available (Telegram keyboards, Discord components)
	// - Falls back to text mode if not available (DingTalk)
	ModeAuto InteractionMode = "auto"

	// ModeInteractive forces use of native interactive elements
	// - Returns error if platform doesn't support interactions
	// - Useful when you explicitly want interactive UI
	ModeInteractive InteractionMode = "interactive"

	// ModeText forces text-based numbered replies
	// - Works on ALL platforms
	// - Useful for accessibility, testing, or user preference
	ModeText InteractionMode = "text"
)

// IsValid checks if the interaction mode is valid
func (m InteractionMode) IsValid() bool {
	switch m {
	case ModeAuto, ModeInteractive, ModeText:
		return true
	default:
		return false
	}
}

// Interaction represents a platform-agnostic interactive element
type Interaction struct {
	ID      string         // Unique identifier for this interaction
	Type    ActionType     // Type of action
	Label   string         // Display label
	Value   string         // Associated value
	Options []Option       // For select actions
	Meta    map[string]any // Additional data
}

// Option represents a selectable option
type Option struct {
	ID    string // Option ID
	Label string // Display label
	Value string // Associated value
}

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

// InteractionResponse represents the user's response
type InteractionResponse struct {
	RequestID string       // Original request ID
	Action    Interaction  // The action user took
	Input     string       // Text input if any
	Timestamp time.Time    // When user responded
}

// IsCancel returns true if the user cancelled
func (r *InteractionResponse) IsCancel() bool {
	return r.Action.Type == ActionCancel
}

// IsConfirm returns true if the user confirmed
func (r *InteractionResponse) IsConfirm() bool {
	return r.Action.Type == ActionConfirm && r.Action.Value == "true"
}

// GetSelectedIndex returns the selected index for numbered text responses
// Returns -1 if not a numbered selection
func (r *InteractionResponse) GetSelectedIndex() int {
	// For numbered text responses, the value might be a number string
	// This is used by text mode parsing
	return -1
}

// Errors
var (
	ErrNotInteraction     = fmt.Errorf("not an interaction response")
	ErrBotNotFound        = fmt.Errorf("bot not found")
	ErrNoAdapter          = fmt.Errorf("no adapter for platform")
	ErrNotSupported       = fmt.Errorf("not supported by platform")
	ErrRequestNotFound    = fmt.Errorf("pending request not found")
	ErrRequestExpired     = fmt.Errorf("pending request expired")
	ErrTimeout            = fmt.Errorf("request timed out")
	ErrChannelClosed      = fmt.Errorf("response channel closed")
	ErrInvalidMode        = fmt.Errorf("invalid interaction mode for platform")
	ErrPendingRequestNotFound = fmt.Errorf("pending request not found")
)
