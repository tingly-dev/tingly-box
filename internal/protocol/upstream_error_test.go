package protocol

import (
	"net/http"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
)

// newOpenAIError builds an *openai.Error as the SDK itself would: Request set
// to a URL that must never leak into a client-facing message, body set to
// the inner error JSON only (the SDK unwraps the "error" envelope before
// handing the body to apierror.Error.UnmarshalJSON).
func newOpenAIError(t *testing.T, status int, body string) *openai.Error {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://internal-proxy.example.com/v1/chat/completions?api_key=leak-me", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	oaiErr := &openai.Error{Request: req, Response: &http.Response{StatusCode: status, Request: req}, StatusCode: status}
	if err := oaiErr.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	return oaiErr
}

// newAnthropicError mirrors newOpenAIError for anthropic.Error, whose
// UnmarshalJSON expects the full {"type":"error","error":{...}} envelope.
func newAnthropicError(t *testing.T, status int, requestID, workspaceID, body string) *anthropic.Error {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "https://internal-proxy.example.com/v1/messages?api_key=leak-me", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	anthErr := &anthropic.Error{Request: req, Response: &http.Response{StatusCode: status, Request: req}, StatusCode: status, RequestID: requestID, WorkspaceID: workspaceID}
	if err := anthErr.UnmarshalJSON([]byte(body)); err != nil {
		t.Fatalf("UnmarshalJSON: %v", err)
	}
	return anthErr
}

func TestUpstreamMessage_OpenAIErrorOmitsRequestURL(t *testing.T) {
	oaiErr := newOpenAIError(t, 429, `{"code":"rate_limit_exceeded","message":"Rate limit reached for requests","param":"","type":"rate_limit_error"}`)

	// Sanity check: the SDK's own Error() does embed the outbound URL and
	// its query string, confirming this is a real leak if passed through.
	if !strings.Contains(oaiErr.Error(), "internal-proxy.example.com") {
		t.Fatalf("test setup invalid: expected the raw SDK error to contain the request URL, got %q", oaiErr.Error())
	}

	got := UpstreamMessage(oaiErr)
	if strings.Contains(got, "internal-proxy.example.com") || strings.Contains(got, "leak-me") {
		t.Errorf("UpstreamMessage() leaked the outbound request URL: %q", got)
	}
	if !strings.Contains(got, "429") || !strings.Contains(got, "Rate limit reached for requests") {
		t.Errorf("UpstreamMessage() dropped useful detail: %q", got)
	}
}

func TestUpstreamMessage_AnthropicErrorOmitsRequestURL(t *testing.T) {
	anthErr := newAnthropicError(t, 529, "req_123", "ws_456", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)

	if !strings.Contains(anthErr.Error(), "internal-proxy.example.com") {
		t.Fatalf("test setup invalid: expected the raw SDK error to contain the request URL, got %q", anthErr.Error())
	}

	got := UpstreamMessage(anthErr)
	if strings.Contains(got, "internal-proxy.example.com") || strings.Contains(got, "leak-me") {
		t.Errorf("UpstreamMessage() leaked the outbound request URL: %q", got)
	}
	if !strings.Contains(got, "529") || !strings.Contains(got, "Overloaded") {
		t.Errorf("UpstreamMessage() dropped useful detail: %q", got)
	}
	if !strings.Contains(got, "req_123") {
		t.Errorf("UpstreamMessage() dropped the request id, useful for vendor support: %q", got)
	}
	// WorkspaceID is a field anthropic.Error's own Error() prints that a
	// hand-picked reconstruction could silently miss (and once did — see
	// UpstreamMessage's doc comment); delegating to the SDK's real Error()
	// on a redacted copy must carry it through automatically.
	if !strings.Contains(got, "ws_456") {
		t.Errorf("UpstreamMessage() dropped the workspace id: %q", got)
	}
}

// TestUpstreamMessage_MatchesSDKErrorMinusURL pins UpstreamMessage's
// "delegate to the real Error(), URL redacted" strategy: the result must be
// byte-for-byte identical to the SDK's own Error() with only the URL
// swapped out, proving nothing else is dropped or altered.
func TestUpstreamMessage_MatchesSDKErrorMinusURL(t *testing.T) {
	anthErr := newAnthropicError(t, 500, "req_789", "ws_012", `{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`)

	original := anthErr.Error()
	wantURL := anthErr.Request.URL.String()
	want := strings.Replace(original, `"`+wantURL+`"`, `"REDACTED"`, 1)

	got := UpstreamMessage(anthErr)
	if got != want {
		t.Errorf("UpstreamMessage() = %q, want %q (original SDK error: %q)", got, want, original)
	}
}

func TestUpstreamStatus_SDKErrorsUnaffectedByURLStripping(t *testing.T) {
	oaiErr := newOpenAIError(t, 401, `{"code":"invalid_api_key","message":"Incorrect API key provided","param":"","type":"invalid_request_error"}`)
	if got := UpstreamStatus(oaiErr, http.StatusInternalServerError); got != 401 {
		t.Errorf("UpstreamStatus() = %d, want 401", got)
	}
}
