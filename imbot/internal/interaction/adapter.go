package interaction

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
)

// Adapter converts platform-agnostic interactions to platform-specific format
//
// The adapter is responsible for:
// - Converting interactions to platform-specific markup (keyboards, cards, components)
// - Creating text-based fallback for platforms without native interactions
// - Parsing user responses into InteractionResponse
// - Updating messages when supported
type Adapter interface {
	// SupportsInteractions returns true if the platform supports native interactions
	//
	// Note: This is about CAPABILITY, not whether to use them.
	// The actual mode is determined by InteractionMode configuration.
	// Even platforms that support interactions can use text mode if configured.
	SupportsInteractions() bool

	// BuildMarkup converts interactions to platform-specific markup
	//
	// Used when Mode=Interactive or ModeAuto with platform support.
	// Returns platform-specific markup (e.g., tgbotapi.InlineKeyboardMarkup, discordgo.Components)
	//
	// Returns error if platform doesn't support interactions.
	BuildMarkup(interactions []Interaction) (any, error)

	// BuildFallbackText creates text-based numbered options
	//
	// Used when Mode=Text or ModeAuto on platforms without native support.
	// Example output: "1. Option A\n2. Option B\n\nReply with number"
	//
	// The format should be:
	// - Original message
	// - Empty line
	// - "Reply with number:" or localized equivalent
	// - Numbered list of options (1-indexed)
	// - "0. Cancel" or localized equivalent
	BuildFallbackText(message string, interactions []Interaction) string

	// ParseResponse parses user response into InteractionResponse
	//
	// For interactive mode: parses callbacks, interactions, etc.
	// For text mode: may return nil and let Handler.parseTextResponse handle it
	//
	// Returns:
	// - (*InteractionResponse, nil) if successfully parsed
	// - (nil, nil) if not an interaction response (delegate to Handler)
	// - (nil, error) if parsing failed
	ParseResponse(msg imbot.Message) (*InteractionResponse, error)

	// UpdateMessage updates a message (optional, for platforms that support it)
	//
	// If interactions is nil/empty, removes any existing markup.
	// Returns error if platform doesn't support message editing.
	UpdateMessage(ctx context.Context, bot imbot.Bot, chatID, messageID, text string, interactions []Interaction) error

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
func (a *BaseAdapter) UpdateMessage(ctx context.Context, bot imbot.Bot, chatID, messageID, text string, interactions []Interaction) error {
	if !a.canEditMessages {
		return ErrNotSupported
	}
	return ErrNotSupported
}
