// Package itx provides interaction types for cross-package use.
// This package contains the core types needed by both the interaction handler
// and platform-specific adapters, avoiding import cycles.
package itx

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
	ModeAuto InteractionMode = "auto"

	// ModeInteractive forces use of native interactive elements
	ModeInteractive InteractionMode = "interactive"

	// ModeText forces text-based numbered replies
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

// Errors
var (
	ErrNotSupported       = fmt.Errorf("not supported by platform")
	ErrNotInteraction     = fmt.Errorf("not an interaction response")
	ErrRequestNotFound    = fmt.Errorf("pending request not found")
	ErrRequestExpired     = fmt.Errorf("pending request expired")
	ErrTimeout            = fmt.Errorf("request timed out")
	ErrChannelClosed      = fmt.Errorf("response channel closed")
	ErrInvalidMode        = fmt.Errorf("invalid interaction mode for platform")
)

// Adapter converts platform-agnostic interactions to platform-specific format
type Adapter interface {
	// SupportsInteractions returns true if the platform supports native interactions
	SupportsInteractions() bool

	// BuildMarkup converts interactions to platform-specific markup
	BuildMarkup(interactions []Interaction) (any, error)

	// BuildFallbackText creates text-based numbered options
	BuildFallbackText(message string, interactions []Interaction) string

	// ParseResponse parses user response into InteractionResponse
	ParseResponse(msg core.Message) (*InteractionResponse, error)

	// UpdateMessage updates a message (optional, for platforms that support it)
	UpdateMessage(ctx interface{}, bot core.Bot, chatID, messageID, text string, interactions []Interaction) error

	// CanEditMessages returns true if platform supports message editing
	CanEditMessages() bool
}

// BaseAdapter provides common functionality for adapters
type BaseAdapter struct {
	supportsInteractions bool
	canEditMessages      bool
}

// NewBaseAdapter creates a new base adapter with the given capabilities
func NewBaseAdapter(supportsInteractions, canEditMessages bool) *BaseAdapter {
	return &BaseAdapter{
		supportsInteractions: supportsInteractions,
		canEditMessages:      canEditMessages,
	}
}

// SupportsInteractions returns true if the platform supports native interactions
func (a *BaseAdapter) SupportsInteractions() bool {
	return a.supportsInteractions
}

// CanEditMessages returns true if platform supports message editing
func (a *BaseAdapter) CanEditMessages() bool {
	return a.canEditMessages
}

// UpdateMessage default implementation returns ErrNotSupported
func (a *BaseAdapter) UpdateMessage(ctx interface{}, bot core.Bot, chatID, messageID, text string, interactions []Interaction) error {
	return ErrNotSupported
}