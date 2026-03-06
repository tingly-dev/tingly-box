// Package discord provides a Discord notification provider
package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// Config holds Discord provider configuration
type Config struct {
	// WebhookURL is the Discord webhook URL
	// Format: https://discord.com/api/webhooks/{id}/{token}
	WebhookURL string `json:"webhook_url"`
	// Username to display for the bot
	Username string `json:"username,omitempty"`
	// AvatarURL for the bot
	AvatarURL string `json:"avatar_url,omitempty"`
}

// Provider sends notifications to Discord via webhooks
type Provider struct {
	name   string
	config Config
	client *http.Client
}

// Option configures a Discord Provider
type Option func(*Provider)

// WithName sets a custom provider name
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithClient sets a custom HTTP client
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// WithTimeout sets the request timeout
func WithTimeout(timeout time.Duration) Option {
	return func(p *Provider) {
		if p.client == nil {
			p.client = &http.Client{}
		}
		p.client.Timeout = timeout
	}
}

// New creates a new Discord provider
func New(config Config, opts ...Option) *Provider {
	p := &Provider{
		name:   "discord",
		config: config,
		client: &http.Client{Timeout: 30 * time.Second},
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

// Send sends a notification to Discord
func (p *Provider) Send(ctx context.Context, notification *notify.Notification) (*notify.Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	// Build webhook payload
	payload := p.buildPayload(notification)

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to Discord webhook
	req, err := http.NewRequestWithContext(ctx, "POST", p.config.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Discord returns 204 No Content on success
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		var errBody bytes.Buffer
		_, _ = errBody.ReadFrom(resp.Body)
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    fmt.Errorf("discord returned status %d: %s", resp.StatusCode, errBody.String()),
		}, fmt.Errorf("discord returned status %d: %s", resp.StatusCode, errBody.String())
	}

	return &notify.Result{
		Provider: p.name,
		Success:  true,
	}, nil
}

// buildPayload creates a Discord webhook payload from a notification
func (p *Provider) buildPayload(notification *notify.Notification) map[string]interface{} {
	payload := make(map[string]interface{})

	// Set username if configured
	if p.config.Username != "" {
		payload["username"] = p.config.Username
	}
	if p.config.AvatarURL != "" {
		payload["avatar_url"] = p.config.AvatarURL
	}

	// Build embed
	embed := make(map[string]interface{})

	// Title
	if notification.Title != "" {
		embed["title"] = notification.Title
	}

	// Description
	embed["description"] = notification.Message

	// Color based on level
	embed["color"] = p.levelColor(notification.Level)

	// Timestamp
	embed["timestamp"] = notification.Timestamp.Format(time.RFC3339)

	// Footer with metadata
	footer := []string{}
	if notification.Category != "" {
		footer = append(footer, "Category: "+notification.Category)
	}
	if len(notification.Tags) > 0 {
		footer = append(footer, "Tags: "+strings.Join(notification.Tags, ", "))
	}
	if len(footer) > 0 {
		embed["footer"] = map[string]interface{}{
			"text": strings.Join(footer, " | "),
		}
	}

	// Image
	if notification.ImageURL != "" {
		embed["image"] = map[string]interface{}{
			"url": notification.ImageURL,
		}
	}

	// Fields for metadata
	if len(notification.Metadata) > 0 {
		fields := []map[string]interface{}{}
		for k, v := range notification.Metadata {
			fields = append(fields, map[string]interface{}{
				"name":  k,
				"value": fmt.Sprintf("%v", v),
			})
		}
		embed["fields"] = fields
	}

	payload["embeds"] = []map[string]interface{}{embed}

	// Add links as components (buttons)
// Discord allows max 5 buttons per action row
if len(notification.Links) > 0 {
	components := []map[string]interface{}{}

	// Split links into rows of 5 buttons each
	const maxButtonsPerRow = 5
	for i := 0; i < len(notification.Links); i += maxButtonsPerRow {
		end := i + maxButtonsPerRow
		if end > len(notification.Links) {
			end = len(notification.Links)
		}
		components = append(components, map[string]interface{}{
			"type": 1, // Action Row
			"components": p.buildButtons(notification.Links[i:end]),
		})
	}

	payload["components"] = components
}

	return payload
}

// buildButtons creates Discord button components from links
func (p *Provider) buildButtons(links []notify.Link) []map[string]interface{} {
	buttons := []map[string]interface{}{}
	for _, link := range links {
		buttons = append(buttons, map[string]interface{}{
			"type":  2, // Button
			"style": 5, // Link button
			"label": link.Text,
			"url":   link.URL,
		})
	}
	return buttons
}

// levelColor returns a color for the notification level
func (p *Provider) levelColor(level notify.Level) int {
	// Discord colors are decimal integers (hex RGB)
	switch level {
	case notify.LevelDebug:
		return 0x808080 // Gray
	case notify.LevelInfo:
		return 0x5865F2 // Discord Blue
	case notify.LevelWarning:
		return 0xFEE75C // Yellow
	case notify.LevelError:
		return 0xED4245 // Red
	case notify.LevelCritical:
		return 0xFF0000 // Bright Red
	default:
		return 0x5865F2 // Default Blue
	}
}

// Close cleans up provider resources
func (p *Provider) Close() error {
	return nil
}
