// Package webhook provides an HTTP webhook notification provider
package webhook

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
	"github.com/tingly-dev/tingly-box/pkg/notify/internal/httpx"
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
		name:    "webhook",
		url:     url,
		method:  "POST",
		client:  &http.Client{Timeout: 30 * time.Second},
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

	headers := make(map[string]string, len(p.headers)+1)
	for k, v := range p.headers {
		headers[k] = v
	}
	if p.authHeader != "" {
		headers["Authorization"] = p.authHeader
	}

	status, body, err := httpx.DoJSON(ctx, p.client, p.method, p.url, headers, notification, 1024)
	if err != nil {
		return nil, err
	}

	if status < 200 || status >= 300 {
		sendErr := fmt.Errorf("webhook returned status %d: %s", status, body)
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    sendErr,
		}, sendErr
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
