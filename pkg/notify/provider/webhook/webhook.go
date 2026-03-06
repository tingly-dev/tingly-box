// Package webhook provides an HTTP webhook notification provider
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// Provider sends notifications to HTTP webhooks
type Provider struct {
	name       string
	url        string
	method     string
	client     *http.Client
	headers    map[string]string
	authHeader string
}

// Option configures a webhook Provider
type Option func(*Provider)

// WithName sets a custom provider name
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithMethod sets the HTTP method (default: POST)
func WithMethod(method string) Option {
	return func(p *Provider) {
		p.method = method
	}
}

// WithClient sets a custom HTTP client
func WithClient(client *http.Client) Option {
	return func(p *Provider) {
		p.client = client
	}
}

// WithHeaders sets custom HTTP headers
func WithHeaders(headers map[string]string) Option {
	return func(p *Provider) {
		p.headers = headers
	}
}

// WithAuth sets an Authorization header
func WithAuth(auth string) Option {
	return func(p *Provider) {
		p.authHeader = auth
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

// New creates a new webhook provider
func New(url string, opts ...Option) *Provider {
	p := &Provider{
		name:   "webhook",
		url:    url,
		method: "POST",
		client: &http.Client{Timeout: 30 * time.Second},
		headers: make(map[string]string),
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

// Send sends a notification to the webhook
func (p *Provider) Send(ctx context.Context, notification *notify.Notification) (*notify.Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	// Build payload
	payload, err := json.Marshal(notification)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal notification: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, p.method, p.url, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	for k, v := range p.headers {
		req.Header.Set(k, v)
	}
	if p.authHeader != "" {
		req.Header.Set("Authorization", p.authHeader)
	}

	// Send request
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body for error messages
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))

	// Check status
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body)),
		}, fmt.Errorf("webhook returned status %d: %s", resp.StatusCode, string(body))
	}

	return &notify.Result{
		Provider: p.name,
		Success:  true,
		Raw:      string(body),
	}, nil
}

// Close cleans up provider resources
func (p *Provider) Close() error {
	// HTTP client doesn't require cleanup unless using custom transport
	return nil
}
