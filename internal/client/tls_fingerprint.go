package client

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"

	utls "github.com/refraction-networking/utls"
)

// TLSDialer creates a custom TLS dialer with fingerprint spoofing
type TLSDialer struct {
	fingerprint TLSFingerprint
	config      *tls.Config
}

// NewTLSDialer creates a new TLS dialer with the specified fingerprint
func NewTLSDialer(fingerprint TLSFingerprint, config *tls.Config) *TLSDialer {
	if config == nil {
		config = &tls.Config{}
	}
	return &TLSDialer{
		fingerprint: fingerprint,
		config:      config,
	}
}

// DialTLSContext implements the DialTLSContext interface for http.Transport
func (d *TLSDialer) DialTLSContext(ctx context.Context, network, addr string) (net.Conn, error) {
	// Extract server name for SNI
	serverName := d.config.ServerName
	if serverName == "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			serverName = addr
		} else {
			serverName = host
		}
	}

	// Create underlying TCP connection
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		return nil, fmt.Errorf("failed to establish TCP connection: %w", err)
	}

	// Get ClientHello spec for the fingerprint
	helloSpec, helloID := d.getClientHello()
	if helloSpec == nil && helloID == nil {
		conn.Close()
		return nil, fmt.Errorf("unsupported TLS fingerprint: %s", d.fingerprint)
	}

	// Create uTLS config
	utlsConfig := &utls.Config{
		ServerName:         serverName,
		InsecureSkipVerify: d.config.InsecureSkipVerify,
		RootCAs:            d.config.RootCAs,
	}

	// Create uTLS connection
	var tlsConn *utls.UConn
	if helloSpec != nil {
		// Use custom ClientHelloSpec
		tlsConn = utls.UClient(conn, utlsConfig, utls.HelloCustom)
		if err := tlsConn.ApplyPreset(helloSpec); err != nil {
			conn.Close()
			return nil, fmt.Errorf("failed to apply TLS preset: %w", err)
		}
	} else {
		// Use built-in ClientHelloID
		tlsConn = utls.UClient(conn, utlsConfig, *helloID)
	}

	// Perform handshake with context support
	err = d.handshakeWithContext(ctx, tlsConn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("TLS handshake failed: %w", err)
	}

	return tlsConn, nil
}

// handshakeWithContext performs TLS handshake with context cancellation support
func (d *TLSDialer) handshakeWithContext(ctx context.Context, conn *utls.UConn) error {
	done := make(chan error, 1)

	go func() {
		done <- conn.Handshake()
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

// getClientHello returns either a custom ClientHelloSpec or a built-in ClientHelloID
func (d *TLSDialer) getClientHello() (*utls.ClientHelloSpec, *utls.ClientHelloID) {
	switch d.fingerprint {
	// === Project-specific client fingerprints ===
	case TLSFingerprintAntigravity:
		return GetAntigravityHelloSpec(), nil
	case TLSFingerprintClaudeCode:
		return GetClaudeCodeHelloSpec(), nil
	case TLSFingerprintCodex:
		return GetCodexHelloSpec(), nil
	case TLSFingerprintGeminiCLI:
		return GetGeminiCLIHelloSpec(), nil
	case TLSFingerprintQwenCode:
		return GetQwenCodeHelloSpec(), nil

	// === Generic browser fingerprints (use built-in) ===
	case TLSFingerprintChrome:
		return nil, &utls.HelloChrome_Auto
	case TLSFingerprintFirefox:
		return nil, &utls.HelloFirefox_Auto
	case TLSFingerprintSafari:
		return nil, &utls.HelloSafari_Auto
	case TLSFingerprintIOS:
		return nil, &utls.HelloIOS_Auto
	case TLSFingerprintAndroid:
		return nil, &utls.HelloAndroid_11_OkHttp

	default:
		return nil, nil
	}
}

// NeedsUTLS returns true if the fingerprint requires uTLS
func NeedsUTLS(fingerprint TLSFingerprint) bool {
	return fingerprint != "" && fingerprint != TLSFingerprintDefault
}

// TLSFingerprint represents a TLS client fingerprint for spoofing
type TLSFingerprint string

const (
	// Default - standard Go TLS
	TLSFingerprintDefault TLSFingerprint = ""

	// === Project-specific client fingerprints ===
	TLSFingerprintAntigravity TLSFingerprint = "antigravity"
	TLSFingerprintClaudeCode  TLSFingerprint = "claude_code"
	TLSFingerprintCodex       TLSFingerprint = "codex"
	TLSFingerprintGeminiCLI   TLSFingerprint = "gemini_cli"
	TLSFingerprintQwenCode    TLSFingerprint = "qwen_code"

	// === Generic browser fingerprints ===
	TLSFingerprintChrome  TLSFingerprint = "chrome"
	TLSFingerprintFirefox TLSFingerprint = "firefox"
	TLSFingerprintSafari  TLSFingerprint = "safari"
	TLSFingerprintIOS     TLSFingerprint = "ios"
	TLSFingerprintAndroid TLSFingerprint = "android"
)
