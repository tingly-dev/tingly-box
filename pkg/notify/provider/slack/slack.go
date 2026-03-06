// Package slack provides a Slack notification provider
package slack

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

// Result is an alias for notify.Result
type Result = notify.Result

// Config holds Slack provider configuration
type Config struct {
	// Token is the Slack bot token (xoxb-...)
	Token string `json:"token"`
	// Channel is the default channel (#general or @user)
	Channel string `json:"channel"`
	// Username to display for the bot
	Username string `json:"username,omitempty"`
	// IconEmoji to use (e.g., ":robot_face:")
	IconEmoji string `json:"icon_emoji,omitempty"`
	// IconURL to use instead of IconEmoji
	IconURL string `json:"icon_url,omitempty"`
}

// Provider sends notifications to Slack
type Provider struct {
	name    string
	config  Config
	client  *http.Client
	baseURL string
}

// Option configures a Slack Provider
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

// WithBaseURL sets a custom Slack API URL (for testing)
func WithBaseURL(url string) Option {
	return func(p *Provider) {
		p.baseURL = url
	}
}

// New creates a new Slack provider
func New(config Config, opts ...Option) *Provider {
	p := &Provider{
		name:    "slack",
		config:  config,
		client:  &http.Client{Timeout: 30 * time.Second},
		baseURL: "https://slack.com/api",
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

// Send sends a notification to Slack
func (p *Provider) Send(ctx context.Context, notification *notify.Notification) (*notify.Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	// Determine channel
	channel := p.config.Channel
	if ch, ok := notification.Metadata["channel"].(string); ok {
		channel = ch
	}
	if channel == "" {
		return &Result{
			Provider: p.name,
			Success:  false,
			Error:    fmt.Errorf("channel is required"),
		}, fmt.Errorf("channel is required")
	}

	// Build message blocks
	blocks := p.buildBlocks(notification)

	// Use chat.postMessage API
	payload := map[string]interface{}{
		"channel": channel,
		"blocks":  blocks,
	}

	if p.config.Username != "" {
		payload["username"] = p.config.Username
	}
	if p.config.IconEmoji != "" {
		payload["icon_emoji"] = p.config.IconEmoji
	}
	if p.config.IconURL != "" {
		payload["icon_url"] = p.config.IconURL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Send to Slack API
	req, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat.postMessage", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.config.Token)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	var slackResp struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error,omitempty"`
		TS      string `json:"ts,omitempty"`
		Channel string `json:"channel,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&slackResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if !slackResp.OK {
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    fmt.Errorf("slack API error: %s", slackResp.Error),
		}, fmt.Errorf("slack API error: %s", slackResp.Error)
	}

	return &notify.Result{
		Provider:  p.name,
		Success:   true,
		MessageID: slackResp.TS,
		Raw:       slackResp,
	}, nil
}

// buildBlocks creates Slack message blocks from a notification
func (p *Provider) buildBlocks(notification *notify.Notification) []map[string]interface{} {
	blocks := []map[string]interface{}{}

	// Add emoji based on level
	emoji := p.levelEmoji(notification.Level)
	if notification.Title != "" {
		blocks = append(blocks, map[string]interface{}{
			"type": "header",
			"text": map[string]interface{}{
				"type":  "plain_text",
				"text":  emoji + " " + notification.Title,
				"emoji": true,
			},
		})
	}

	// Add message content
	blocks = append(blocks, map[string]interface{}{
		"type": "section",
		"text": map[string]interface{}{
			"type": "mrkdwn",
			"text": notification.Message,
		},
	})

	// Add metadata section
	metadata := []string{}
	if notification.Category != "" {
		metadata = append(metadata, "*Category:* "+notification.Category)
	}
	if len(notification.Tags) > 0 {
		metadata = append(metadata, "*Tags:* "+strings.Join(notification.Tags, ", "))
	}
	if !notification.Timestamp.IsZero() {
		metadata = append(metadata, "*Time:* "+notification.Timestamp.Format(time.RFC3339))
	}

	if len(metadata) > 0 {
		blocks = append(blocks, map[string]interface{}{
			"type": "context",
			"elements": []map[string]interface{}{
				{
					"type": "mrkdwn",
					"text": strings.Join(metadata, " | "),
				},
			},
		})
	}

	// Add links as buttons
	if len(notification.Links) > 0 {
		actions := []map[string]interface{}{}
		for _, link := range notification.Links {
			actions = append(actions, map[string]interface{}{
				"type":  "button",
				"text":  map[string]interface{}{"type": "plain_text", "text": link.Text},
				"url":   link.URL,
				"style": "primary",
			})
		}
		blocks = append(blocks, map[string]interface{}{
			"type":   "actions",
			"elements": actions,
		})
	}

	return blocks
}

// levelEmoji returns an emoji for the notification level
func (p *Provider) levelEmoji(level notify.Level) string {
	switch level {
	case notify.LevelDebug:
		return ":mag:"
	case notify.LevelInfo:
		return ":information_source:"
	case notify.LevelWarning:
		return ":warning:"
	case notify.LevelError:
		return ":x:"
	case notify.LevelCritical:
		return ":rotating_light:"
	default:
		return ":bell:"
	}
}

// Close cleans up provider resources
func (p *Provider) Close() error {
	return nil
}
