package protocol

import (
	"context"
	"errors"
	"net"
	"strings"
	"syscall"
)

// TransportFailureReason categorizes an error that happened before any HTTP
// response was received from the upstream provider — DNS, TCP connect, TLS
// handshake, or a deadline/cancellation. Errors like these carry no provider
// status code, so today they're flattened into the same 500 as a genuine
// internal bug and logged as a raw, hard-to-scan Go error string (e.g.
// `dial tcp: lookup api.openai.com: no such host`).
type TransportFailureReason string

const (
	ReasonDNS               TransportFailureReason = "dns_error"
	ReasonConnectionRefused TransportFailureReason = "connection_refused"
	ReasonTLS               TransportFailureReason = "tls_error"
	ReasonTimeout           TransportFailureReason = "timeout"
	ReasonCanceled          TransportFailureReason = "canceled"
	ReasonNetwork           TransportFailureReason = "network_error"
)

// transportFailureMessages gives each reason a short, stable description
// safe to hand back to an API caller — no host, path, or raw dial/DNS text,
// which the underlying Go error usually embeds and which can be confusing
// (or leak internal detail) outside a server log.
var transportFailureMessages = map[TransportFailureReason]string{
	ReasonDNS:               "could not resolve the upstream provider's hostname",
	ReasonConnectionRefused: "the upstream provider refused the connection",
	ReasonTLS:               "TLS handshake with the upstream provider failed",
	ReasonTimeout:           "the upstream provider did not respond in time",
	ReasonCanceled:          "the request was canceled before the upstream provider responded",
	ReasonNetwork:           "a network error occurred while contacting the upstream provider",
}

// ClassifyTransportError inspects err for the transport-level failure shapes
// produced when a provider call never got as far as an HTTP response.
// ClassifyUpstreamFailure falls back to this once no vendor SDK's typed HTTP
// error matches. ok is false when err doesn't look like a transport failure
// at all (nil, or already an SDK-typed HTTP error). Only the reason is
// returned; the human-readable sentence lives in transportFailureMessages,
// looked up by the one caller that needs it.
func ClassifyTransportError(err error) (reason TransportFailureReason, ok bool) {
	if err == nil {
		return "", false
	}

	switch {
	case errors.Is(err, context.Canceled):
		return ReasonCanceled, true
	case errors.Is(err, context.DeadlineExceeded):
		return ReasonTimeout, true
	case isDNSError(err):
		return ReasonDNS, true
	case errors.Is(err, syscall.ECONNREFUSED):
		return ReasonConnectionRefused, true
	case isTLSError(err):
		return ReasonTLS, true
	default:
		var netErr net.Error
		if !errors.As(err, &netErr) {
			return "", false
		}
		if netErr.Timeout() {
			return ReasonTimeout, true
		}
		return ReasonNetwork, true
	}
}

func isDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

// isTLSError reports certificate/handshake failures. Go's TLS and x509
// errors don't share one common type worth exhaustively enumerating with
// errors.As, so this matches the well-known substrings the standard
// library's own error text always contains.
func isTLSError(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "x509:") || strings.Contains(msg, "tls:")
}
