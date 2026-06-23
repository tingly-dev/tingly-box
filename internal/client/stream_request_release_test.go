package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type streamRequestReleaseRoundTripper func(*http.Request) (*http.Response, error)

func (f streamRequestReleaseRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestStreamRequestReleaseTransportClearsResponseRequestBody(t *testing.T) {
	body := io.NopCloser(strings.NewReader("large marshaled request body"))
	req, err := http.NewRequest(http.MethodPost, "https://example.test/v1/messages", body)
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(strings.NewReader("large marshaled request body")), nil
	}
	req.ContentLength = int64(len("large marshaled request body"))

	wrapped := &streamRequestReleaseTransport{base: streamRequestReleaseRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			Body:       io.NopCloser(strings.NewReader("event: message_stop\ndata: {}\n\n")),
			Request:    req,
		}, nil
	})}

	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Request == nil {
		t.Fatal("expected response request metadata to remain available")
	}
	if resp.Request.Body != nil {
		t.Fatal("expected response request body to be cleared")
	}
	if resp.Request.GetBody != nil {
		t.Fatal("expected response request GetBody closure to be cleared")
	}
	if resp.Request.ContentLength != 0 {
		t.Fatalf("expected response request content length to be reset, got %d", resp.Request.ContentLength)
	}
	if resp.Request.Method != http.MethodPost || resp.Request.URL.String() != "https://example.test/v1/messages" {
		t.Fatalf("expected method and URL to remain for error reporting, got %s %s", resp.Request.Method, resp.Request.URL.String())
	}
}

func TestStreamRequestReleaseTransportKeepsRetryableErrorBody(t *testing.T) {
	req, err := http.NewRequest(http.MethodPost, "https://example.test/v1/messages", io.NopCloser(strings.NewReader("body")))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("body")), nil }

	wrapped := &streamRequestReleaseTransport{base: streamRequestReleaseRoundTripper(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusTooManyRequests,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("{}")),
			Request:    req,
		}, nil
	})}

	resp, err := wrapped.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Request.Body == nil {
		t.Fatal("expected retryable non-SSE response request body to remain available")
	}
	if resp.Request.GetBody == nil {
		t.Fatal("expected retryable non-SSE response request GetBody to remain available")
	}
}
