package adapter

import (
	"context"
	"fmt"

	"github.com/tingly-dev/tingly-box/imbot/internal/core"
)

// EventAdapter converts platform-specific events to unified core.Message
type EventAdapter[RawEventT any] interface {
	// Adapt converts a raw platform event to core.Message
	Adapt(ctx context.Context, event RawEventT) (*core.Message, error)

	// Platform returns the platform this adapter is for
	Platform() core.Platform
}

// MessageAdapter converts platform-specific messages to unified core.Message
// This is a more specific version of EventAdapter for message types
type MessageAdapter[RawMessageT any] interface {
	// AdaptMessage converts a raw platform message to core.Message
	AdaptMessage(ctx context.Context, msg RawMessageT) (*core.Message, error)

	// AdaptCallback converts a platform callback to core.Message (for button clicks, etc.)
	AdaptCallback(ctx context.Context, callback RawMessageT) (*core.Message, error)

	// Platform returns the platform this adapter is for
	Platform() core.Platform
}

// BaseAdapter provides common functionality for all adapters
type BaseAdapter struct {
	config *core.Config
	logger core.Logger
}

// NewBaseAdapter creates a new base adapter
func NewBaseAdapter(config *core.Config) *BaseAdapter {
	return &BaseAdapter{
		config: config,
		logger: core.NewLogger(config.Logging),
	}
}

// Config returns the adapter's config
func (a *BaseAdapter) Config() *core.Config {
	return a.config
}

// Logger returns the adapter's logger
func (a *BaseAdapter) Logger() core.Logger {
	return a.logger
}

// Platform returns the platform from config
func (a *BaseAdapter) Platform() core.Platform {
	return a.config.Platform
}

// AdaptError represents an error during adaptation
type AdaptError struct {
	Platform  core.Platform
	Operation string
	Raw       interface{}
	Cause     error
}

// Error implements error interface
func (e *AdaptError) Error() string {
	return fmt.Sprintf("adapter error on %s: %s", e.Platform, e.Operation)
}

// Unwrap returns the underlying cause
func (e *AdaptError) Unwrap() error {
	return e.Cause
}

// NewAdaptError creates a new adaptation error
func NewAdaptError(platform core.Platform, operation string, raw interface{}, cause error) *AdaptError {
	return &AdaptError{
		Platform:  platform,
		Operation: operation,
		Raw:       raw,
		Cause:     cause,
	}
}
