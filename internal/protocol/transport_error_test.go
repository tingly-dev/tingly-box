package protocol

import (
	"context"
	"errors"
	"net"
	"net/http"
	"syscall"
	"testing"
)

func TestClassifyTransportError(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantReason TransportFailureReason
		wantOK     bool
	}{
		{"nil", nil, "", false},
		{"dns", &net.DNSError{Err: "no such host", Name: "api.openai.com", IsNotFound: true}, ReasonDNS, true},
		{"connection refused", &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}, ReasonConnectionRefused, true},
		{"deadline exceeded", context.DeadlineExceeded, ReasonTimeout, true},
		{"canceled", context.Canceled, ReasonCanceled, true},
		{"tls", errors.New(`Get "https://api.example.com": tls: failed to verify certificate: x509: certificate signed by unknown authority`), ReasonTLS, true},
		{"generic net error timeout", fakeNetError{timeout: true}, ReasonTimeout, true},
		{"generic net error", fakeNetError{timeout: false}, ReasonNetwork, true},
		{"unrelated error", errors.New("boom"), "", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			reason, ok := ClassifyTransportError(c.err)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if !ok {
				return
			}
			if reason != c.wantReason {
				t.Errorf("reason = %q, want %q", reason, c.wantReason)
			}
			if transportFailureMessages[reason] == "" {
				t.Errorf("expected a non-empty client-safe message for reason %q", reason)
			}
		})
	}
}

func TestUpstreamStatus_TransportFailureMapsToBadGateway(t *testing.T) {
	err := &net.DNSError{Err: "no such host", Name: "api.openai.com", IsNotFound: true}
	if got := UpstreamStatus(err, http.StatusInternalServerError); got != http.StatusBadGateway {
		t.Errorf("UpstreamStatus() = %d, want %d", got, http.StatusBadGateway)
	}
}

func TestUpstreamStatus_UnclassifiedFallsBackToFallback(t *testing.T) {
	err := errors.New("boom")
	if got := UpstreamStatus(err, http.StatusInternalServerError); got != http.StatusInternalServerError {
		t.Errorf("UpstreamStatus() = %d, want %d", got, http.StatusInternalServerError)
	}
}

func TestUpstreamMessage(t *testing.T) {
	dnsErr := &net.DNSError{Err: "no such host", Name: "api.openai.com", IsNotFound: true}
	got := UpstreamMessage(dnsErr)
	if got == dnsErr.Error() {
		t.Errorf("UpstreamMessage() should not leak the raw DNS error verbatim, got %q", got)
	}
	if got == "" {
		t.Error("UpstreamMessage() returned empty string for a classified transport error")
	}

	plain := errors.New("some sdk-typed error message")
	if got := UpstreamMessage(plain); got != plain.Error() {
		t.Errorf("UpstreamMessage() = %q, want passthrough %q", got, plain.Error())
	}
}

// fakeNetError implements net.Error for cases not covered by the standard
// library's own error types.
type fakeNetError struct{ timeout bool }

func (e fakeNetError) Error() string   { return "fake net error" }
func (e fakeNetError) Timeout() bool   { return e.timeout }
func (e fakeNetError) Temporary() bool { return false }
