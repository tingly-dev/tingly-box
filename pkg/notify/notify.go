// Package notify provides a unified notification system for sending messages
// to various channels like webhooks, Slack, Discord, email, and system notifications.
//
// Basic usage:
//
//	// Create a notifier
//	notifier := notify.NewMultiplexer()
//
//	// Add providers
//	notifier.AddProvider(webhook.New("https://hooks.example.com/notify"))
//	notifier.AddProvider(slack.New(slack.Config{Token: "xoxb-...", Channel: "#alerts"}))
//
//	// Send notification
//	err := notifier.Send(context.Background(), &notify.Notification{
//		Title:   "Deployment Complete",
//		Message: "App v1.2.3 deployed successfully",
//		Level:   notify.LevelInfo,
//	})
package notify

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Notification levels
const (
	LevelDebug   Level = "debug"
	LevelInfo    Level = "info"
	LevelWarning Level = "warning"
	LevelError   Level = "error"
	LevelCritical Level = "critical"
)

// Level represents the severity level of a notification
type Level string

// String returns the string representation of the level
func (l Level) String() string {
	return string(l)
}

// Notification represents a notification message to be sent
type Notification struct {
	// Title is the notification title/subject
	Title string `json:"title,omitempty"`
	// Message is the main notification content
	Message string `json:"message"`
	// Level indicates the severity (debug, info, warning, error, critical)
	Level Level `json:"level,omitempty"`
	// Category for grouping related notifications
	Category string `json:"category,omitempty"`
	// Tags for filtering and routing
	Tags []string `json:"tags,omitempty"`
	// Timestamp of the notification (defaults to now if not set)
	Timestamp time.Time `json:"timestamp,omitempty"`
	// Metadata for additional context
	Metadata map[string]interface{} `json:"metadata,omitempty"`
	// Links to include in the notification
	Links []Link `json:"links,omitempty"`
	// Image URL to include (supported by some providers)
	ImageURL string `json:"image_url,omitempty"`
	// Provider-specific overrides
	ProviderData map[string]interface{} `json:"provider_data,omitempty"`
}

// Link represents a clickable link in a notification
type Link struct {
	Text string `json:"text"`
	URL  string `json:"url"`
}

// Validate checks if the notification is valid
func (n *Notification) Validate() error {
	if n.Message == "" {
		return errors.New("notification message is required")
	}
	if n.Timestamp.IsZero() {
		n.Timestamp = time.Now()
	}
	if n.Level == "" {
		n.Level = LevelInfo
	}
	return nil
}

// Result represents the result of sending a notification
type Result struct {
	// Provider that sent the notification
	Provider string `json:"provider"`
	// Success indicates if the send was successful
	Success bool `json:"success"`
	// MessageID returned by the provider (if applicable)
	MessageID string `json:"message_id,omitempty"`
	// Error if the send failed
	Error error `json:"error,omitempty"`
	// Latency for the send operation
	Latency time.Duration `json:"latency,omitempty"`
	// Raw response from provider (for debugging)
	Raw interface{} `json:"raw,omitempty"`
}

// Provider defines the interface for notification providers
type Provider interface {
	// Name returns the provider name
	Name() string
	// Send sends a notification
	Send(ctx context.Context, notification *Notification) (*Result, error)
	// Close cleans up provider resources
	Close() error
}

// Notifier defines the interface for sending notifications
type Notifier interface {
	// Send sends a notification to all configured providers
	Send(ctx context.Context, notification *Notification) ([]*Result, error)
	// SendTo sends a notification to a specific provider by name
	SendTo(ctx context.Context, providerName string, notification *Notification) (*Result, error)
	// AddProvider adds a notification provider
	AddProvider(provider Provider)
	// RemoveProvider removes a provider by name
	RemoveProvider(name string) bool
	// GetProvider returns a provider by name
	GetProvider(name string) Provider
	// ListProviders returns all registered provider names
	ListProviders() []string
}

// Error types
var (
	// ErrProviderNotFound is returned when a provider is not found
	ErrProviderNotFound = errors.New("provider not found")
	// ErrSendFailed is returned when sending a notification fails
	ErrSendFailed = errors.New("failed to send notification")
)

// ProviderConfig holds common provider configuration
type ProviderConfig struct {
	// Name overrides the default provider name
	Name string `json:"name,omitempty"`
	// Enabled controls whether the provider is active
	Enabled bool `json:"enabled"`
	// Timeout for send operations (default: 30 seconds)
	Timeout time.Duration `json:"timeout,omitempty"`
	// RetryCount for failed sends (default: 0, no retry)
	RetryCount int `json:"retry_count,omitempty"`
	// RetryDelay between retries (default: 1 second)
	RetryDelay time.Duration `json:"retry_delay,omitempty"`
}

// FormatError formats an error with provider context
func FormatError(provider string, err error) error {
	return fmt.Errorf("%s: %w", provider, err)
}

// IsLevelAtLeast checks if a level is at least as severe as another
func IsLevelAtLeast(level, min Level) bool {
	severity := map[Level]int{
		LevelDebug:    0,
		LevelInfo:     1,
		LevelWarning:  2,
		LevelError:    3,
		LevelCritical: 4,
	}
	return severity[level] >= severity[min]
}
