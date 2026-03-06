// Package system provides desktop system notification provider using beeep
package system

import (
	"context"
	"fmt"

	"github.com/gen2brain/beeep"
	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// Config holds system notification configuration
type Config struct {
	// AppName is the name of the application (default: "notify")
	AppName string `json:"app_name,omitempty"`
	// Sound to play (path to sound file, empty for default)
	Sound string `json:"sound,omitempty"`
	// Icon is the path to icon file
	Icon string `json:"icon,omitempty"`
}

// Provider sends desktop system notifications using beeep
type Provider struct {
	name   string
	config Config
}

// Option configures a system Provider
type Option func(*Provider)

// WithName sets a custom provider name
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithSound sets the sound file path
func WithSound(path string) Option {
	return func(p *Provider) {
		p.config.Sound = path
	}
}

// WithIcon sets the icon file path
func WithIcon(path string) Option {
	return func(p *Provider) {
		p.config.Icon = path
	}
}

// New creates a new system notification provider
func New(config Config, opts ...Option) *Provider {
	p := &Provider{
		name:   "system",
		config: config,
	}

	if p.config.AppName == "" {
		p.config.AppName = "notify"
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Name returns the provider name
func (p *Provider) Name() string {
	return p.name
}

// Send sends a desktop notification using beeep
func (p *Provider) Send(ctx context.Context, notification *notify.Notification) (*notify.Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	// Build title
	title := notification.Title
	if title == "" {
		title = string(notification.Level)
	}

	// Get icon from config or notification
	icon := p.config.Icon
	if notification.ImageURL != "" {
		icon = notification.ImageURL
	}

	// Get sound from config or notification metadata
	sound := p.config.Sound
	if s, ok := notification.Metadata["sound"].(string); ok {
		sound = s
	}

	// Send notification
	err := beeep.Notify(title, notification.Message, icon)
	if err != nil {
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    err,
		}, err
	}

	// Play beep if sound is configured
	if sound != "" {
		_ = beeep.Beep(523.25, 50)
	}

	return &notify.Result{
		Provider: p.name,
		Success:  true,
	}, nil
}

// Close cleans up provider resources
func (p *Provider) Close() error {
	return nil
}

// IsSupported checks if system notifications are supported on the current platform
// beeep supports: Windows, macOS, Linux, BSD, Unix
func IsSupported() bool {
	return true
}
