// Package email provides an SMTP email notification provider
package email

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/tingly-dev/tingly-box/pkg/notify"
)

// Config holds SMTP email configuration
type Config struct {
	// SMTP server host:port
	Host string `json:"host"`
	// SMTP username (empty for no auth)
	Username string `json:"username,omitempty"`
	// SMTP password (empty for no auth)
	Password string `json:"password,omitempty"`
	// From email address
	From string `json:"from"`
	// To email addresses (default recipients)
	To []string `json:"to"`
	// CC email addresses
	CC []string `json:"cc,omitempty"`
	// BCC email addresses
	BCC []string `json:"bcc,omitempty"`
	// Use TLS (default: true for port 465, false otherwise)
	UseTLS bool `json:"use_tls,omitempty"`
	// Skip TLS certificate verification (not recommended for production)
	InsecureSkipVerify bool `json:"insecure_skip_verify,omitempty"`
}

// Provider sends notifications via email
type Provider struct {
	name   string
	config Config
	dialer *Dialer
}

// Option configures an email Provider
type Option func(*Provider)

// WithName sets a custom provider name
func WithName(name string) Option {
	return func(p *Provider) {
		p.name = name
	}
}

// WithDialer sets a custom dialer
func WithDialer(dialer *Dialer) Option {
	return func(p *Provider) {
		p.dialer = dialer
	}
}

// New creates a new email provider
func New(config Config, opts ...Option) *Provider {
	p := &Provider{
		name:   "email",
		config: config,
	}

	// Create default dialer if not provided
	if config.UseTLS || containsPort(config.Host, "465") {
		p.dialer = &Dialer{
			Host:     config.Host,
			Username: config.Username,
			Password: config.Password,
			TLS:      true,
		}
	} else {
		p.dialer = &Dialer{
			Host:     config.Host,
			Username: config.Username,
			Password: config.Password,
		}
	}

	if config.InsecureSkipVerify {
		p.dialer.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	for _, opt := range opts {
		opt(p)
	}

	return p
}

// Dialer handles SMTP connections
type Dialer struct {
	Host      string
	Username  string
	Password  string
	TLS       bool
	TLSConfig *tls.Config
	Timeout   time.Duration
}

// Dial connects to the SMTP server
func (d *Dialer) Dial(ctx context.Context) (*smtp.Client, error) {
	host, _, _ := net.SplitHostPort(d.Host)
	timeout := d.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	var conn net.Conn
	var err error

	// Use dialer with context for cancellation support
	netDialer := &net.Dialer{Timeout: timeout}
	if d.TLS {
		tlsDialer := &tls.Dialer{
			NetDialer: netDialer,
			Config:    d.TLSConfig,
		}
		conn, err = tlsDialer.DialContext(ctx, "tcp", d.Host)
	} else {
		conn, err = netDialer.DialContext(ctx, "tcp", d.Host)
	}
	if err != nil {
		return nil, err
	}

	client, err := smtp.NewClient(conn, host)
	if err != nil {
		conn.Close()
		return nil, err
	}

	if d.Username != "" {
		auth := smtp.PlainAuth("", d.Username, d.Password, host)
		if err := client.Auth(auth); err != nil {
			client.Close()
			return nil, err
		}
	}

	return client, nil
}

// Name returns the provider name
func (p *Provider) Name() string {
	return p.name
}

// Send sends a notification via email
func (p *Provider) Send(ctx context.Context, notification *notify.Notification) (*notify.Result, error) {
	if err := notification.Validate(); err != nil {
		return nil, fmt.Errorf("invalid notification: %w", err)
	}

	// Determine recipients - handle both []string and []interface{} (from JSON)
	to := p.config.To
	if v, ok := notification.Metadata["to"]; ok {
		switch v := v.(type) {
		case []string:
			to = v
		case []interface{}:
			to = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					to = append(to, s)
				}
			}
		case string:
			to = []string{v}
		}
	}
	if len(to) == 0 {
		return nil, fmt.Errorf("no recipients specified")
	}

	// Build email
	subject := p.buildSubject(notification)
	body := p.buildBody(notification)

	// Send email
	if err := p.sendEmail(ctx, to, subject, body); err != nil {
		return &notify.Result{
			Provider: p.name,
			Success:  false,
			Error:    err,
		}, err
	}

	return &notify.Result{
		Provider: p.name,
		Success:  true,
	}, nil
}

// sendEmail sends an email to the specified recipients
func (p *Provider) sendEmail(ctx context.Context, to []string, subject, body string) error {
	// Include CC and BCC recipients
	allRecipients := make([]string, len(to))
	copy(allRecipients, to)
	allRecipients = append(allRecipients, p.config.CC...)
	allRecipients = append(allRecipients, p.config.BCC...)

	client, err := p.dialer.Dial(ctx)
	if err != nil {
		return fmt.Errorf("failed to connect to SMTP server: %w", err)
	}
	defer client.Close()

	// Set sender and recipients
	if err := client.Mail(p.config.From); err != nil {
		return err
	}
	for _, addr := range allRecipients {
		if err := client.Rcpt(addr); err != nil {
			return err
		}
	}

	// Get writer
	w, err := client.Data()
	if err != nil {
		return err
	}
	defer w.Close()

	// Write headers and body
	_, err = fmt.Fprintf(w, "From: %s\r\n", p.config.From)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "To: %s\r\n", joinStrings(to))
	if err != nil {
		return err
	}
	if len(p.config.CC) > 0 {
		_, err = fmt.Fprintf(w, "Cc: %s\r\n", joinStrings(p.config.CC))
		if err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "Subject: %s\r\n", subject)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "MIME-Version: 1.0\r\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "Content-Type: text/plain; charset=UTF-8\r\n")
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(w, "\r\n%s\r\n", body)
	if err != nil {
		return err
	}

	return nil
}

// buildSubject creates the email subject line
func (p *Provider) buildSubject(notification *notify.Notification) string {
	if notification.Title != "" {
		return fmt.Sprintf("[%s] %s", strings.ToUpper(string(notification.Level)), notification.Title)
	}
	return fmt.Sprintf("[%s] Notification", strings.ToUpper(string(notification.Level)))
}

// buildBody creates the email body
func (p *Provider) buildBody(notification *notify.Notification) string {
	var buf bytes.Buffer

	buf.WriteString(notification.Message)
	buf.WriteString("\n\n")

	if notification.Category != "" {
		buf.WriteString(fmt.Sprintf("Category: %s\n", notification.Category))
	}
	if len(notification.Tags) > 0 {
		buf.WriteString(fmt.Sprintf("Tags: %s\n", joinStrings(notification.Tags)))
	}
	if !notification.Timestamp.IsZero() {
		buf.WriteString(fmt.Sprintf("Time: %s\n", notification.Timestamp.Format(time.RFC3339)))
	}

	if len(notification.Links) > 0 {
		buf.WriteString("\nLinks:\n")
		for _, link := range notification.Links {
			buf.WriteString(fmt.Sprintf("  - %s: %s\n", link.Text, link.URL))
		}
	}

	return buf.String()
}

// Close cleans up provider resources
func (p *Provider) Close() error {
	return nil
}

// Helper functions

func containsPort(hostPort, port string) bool {
	_, p, _ := net.SplitHostPort(hostPort)
	return p == port
}

func joinStrings(strs []string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for _, s := range strs[1:] {
		result += ", " + s
	}
	return result
}
