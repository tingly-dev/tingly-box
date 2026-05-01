package platform

import (
	"context"

	"github.com/tingly-dev/tingly-box/imbot"
)

// Feature represents a platform capability
type Feature string

const (
	FeatureVerbose         Feature = "verbose"          // Verbose intermediate messages
	FeatureInlineKeyboard  Feature = "inline_keyboard"  // Inline keyboard buttons
	FeatureMarkdown        Feature = "markdown"         // Markdown formatting
	FeatureReactions       Feature = "reactions"        // Message reactions
	FeatureMessageEditing  Feature = "message_editing"  // Edit sent messages
	FeatureKeyboardRemoval Feature = "keyboard_removal" // Remove inline keyboards
	FeatureMediaUpload     Feature = "media_upload"     // Upload media files
	FeatureQRAuth          Feature = "qr_auth"          // QR code authentication
	FeatureCardRendering   Feature = "card_rendering"   // Rich card rendering
)

// MessageHandler defines platform-specific message handling behavior
type MessageHandler interface {
	// HandlePlatformMessage handles platform-specific message preprocessing
	// Returns (handled, error) - if handled is true, no further processing is needed
	HandlePlatformMessage(ctx *Context) (bool, error)

	// SendMessage sends a message with platform-specific formatting
	SendMessage(ctx context.Context, chatID string, opts *imbot.SendMessageOptions) error

	// SupportsFeature checks if the platform supports a specific feature
	SupportsFeature(feature Feature) bool

	// Platform returns the platform identifier
	Platform() imbot.Platform
}

// Context provides context for platform-specific message handling
type Context struct {
	Bot       imbot.Bot
	ChatID    string
	SenderID  string
	MessageID string
	Platform  imbot.Platform
	Message   imbot.Message
	// Handler is the bot handler for platform callbacks
	Handler interface{}
}

// IsDirect returns true if the message is a direct message
func (c *Context) IsDirect() bool {
	return c.Message.IsDirectMessage()
}

// IsGroup returns true if the message is a group message
func (c *Context) IsGroup() bool {
	return c.Message.IsGroupMessage()
}

// Text returns the message text
func (c *Context) Text() string {
	return c.Message.GetText()
}

// Registry manages platform-specific handlers
type Registry struct {
	handlers map[imbot.Platform]MessageHandler
}

// NewRegistry creates a new platform handler registry
func NewRegistry() *Registry {
	return &Registry{
		handlers: make(map[imbot.Platform]MessageHandler),
	}
}

// Register registers a platform handler
func (r *Registry) Register(handler MessageHandler) {
	r.handlers[handler.Platform()] = handler
}

// Get returns the handler for a platform, or nil if not registered
func (r *Registry) Get(platform imbot.Platform) MessageHandler {
	return r.handlers[platform]
}

// IsSupported checks if a platform has a registered handler
func (r *Registry) IsSupported(platform imbot.Platform) bool {
	_, ok := r.handlers[platform]
	return ok
}

// SupportedPlatforms returns all platforms with registered handlers
func (r *Registry) SupportedPlatforms() []imbot.Platform {
	platforms := make([]imbot.Platform, 0, len(r.handlers))
	for p := range r.handlers {
		platforms = append(platforms, p)
	}
	return platforms
}

// SupportsFeature checks if any platform supports a specific feature
func (r *Registry) SupportsFeature(platform imbot.Platform, feature Feature) bool {
	handler := r.Get(platform)
	if handler == nil {
		return false
	}
	return handler.SupportsFeature(feature)
}
