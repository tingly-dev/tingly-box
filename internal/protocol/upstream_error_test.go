package protocol

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
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

	got := UpstreamMessage(oaiErr)
	if strings.Contains(got, "internal-proxy.example.com") || strings.Contains(got, "leak-me") {
		t.Errorf("UpstreamMessage() leaked the outbound request URL: %q", got)
	}
	if !strings.Contains(got, "429") || !strings.Contains(got, "Rate limit reached for requests") {
		t.Errorf("UpstreamMessage() dropped useful detail: %q", got)
	}
	if !strings.HasPrefix(got, "Upstream error: ") || strings.Contains(strings.ToLower(got), "openai") {
		t.Errorf("UpstreamMessage() prefix must be vendor-neutral: %q", got)
	}
	for _, want := range []string{"type=rate_limit_error", "code=rate_limit_exceeded"} {
		if !strings.Contains(got, want) {
			t.Errorf("UpstreamMessage() = %q, missing %q", got, want)
		}
	}
}

// The client message is assembled from an allowlist of fields; param and the
// raw body must never be relayed, and an oversized vendor message is capped.
func TestUpstreamMessage_OpenAIErrorAllowlistAndCap(t *testing.T) {
	long := strings.Repeat("x", 2000)
	oaiErr := newOpenAIError(t, 400, `{"code":"c","message":"`+long+`","param":"secret-param","type":"invalid_request_error"}`)

	got := UpstreamMessage(oaiErr)
	if strings.Contains(got, "secret-param") {
		t.Errorf("UpstreamMessage() relayed param: %q", got)
	}
	if len(got) > maxClientDetailLen+200 {
		t.Errorf("UpstreamMessage() not capped, len=%d", len(got))
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

// TestUpstreamMessage_AnthropicExplicitFormat pins the client message built
// from typed fields in the existing wire shape; it deliberately does not depend on the SDK's Error()
// text, which embeds the URL and raw body and may change between versions.
func TestUpstreamMessage_AnthropicExplicitFormat(t *testing.T) {
	anthErr := newAnthropicError(t, 500, "req_789", "ws_012", `{"type":"error","error":{"type":"api_error","message":"Internal server error"}}`)

	want := `POST "REDACTED": 500 Internal Server Error (Request-ID: req_789) (Workspace-ID: ws_012) {"type":"error","error":{"type":"api_error","message":"Internal server error"}}`
	if got := UpstreamMessage(anthErr); got != want {
		t.Errorf("UpstreamMessage() = %q, want %q", got, want)
	}
}

// The log wrapper carries what clients must not see, and stays transparent
// to errors.As so status classification is unchanged.
func TestLoggableUpstreamError_AddsDetailForLogOnly(t *testing.T) {
	oaiErr := newOpenAIError(t, 400, `{"code":"c","message":"bad","param":"secret-param","type":"invalid_request_error"}`)

	logged := LoggableUpstreamError(oaiErr)
	if !strings.Contains(logged.Error(), "secret-param") || !strings.Contains(logged.Error(), `"message":"bad"`) {
		t.Errorf("log text missing detail: %q", logged.Error())
	}
	if strings.Contains(UpstreamMessage(logged), "secret-param") {
		t.Errorf("client message relayed log-only detail: %q", UpstreamMessage(logged))
	}
	if got := UpstreamStatus(logged, http.StatusInternalServerError); got != 400 {
		t.Errorf("status through wrapper = %d, want 400", got)
	}

	plain := errors.New("boom")
	if LoggableUpstreamError(plain) != plain {
		t.Error("errors without SDK detail must be returned unchanged")
	}
	if LoggableUpstreamError(nil) != nil {
		t.Error("nil must stay nil")
	}
}

func TestUpstreamStatus_SDKErrorsUnaffectedByURLStripping(t *testing.T) {
	oaiErr := newOpenAIError(t, 401, `{"code":"invalid_api_key","message":"Incorrect API key provided","param":"","type":"invalid_request_error"}`)
	if got := UpstreamStatus(oaiErr, http.StatusInternalServerError); got != 401 {
		t.Errorf("UpstreamStatus() = %d, want 401", got)
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

func TestUpstreamMessage_TransportFailureAndPassthrough(t *testing.T) {
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

// TestClassifyUpstreamFailure_MatchesIndividualAccessors pins
// ClassifyUpstreamFailure as the single source of truth: its Status/Message
// must equal what UpstreamStatus/UpstreamMessage return separately, for
// every error shape UpstreamStatus/UpstreamMessage handle. Callers needing
// both should get identical results whether they classify once via this or
// twice via the two wrappers — only the number of classification passes
// should differ.
func TestClassifyUpstreamFailure_MatchesIndividualAccessors(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"openai error", newOpenAIError(t, 429, `{"code":"rate_limit_exceeded","message":"Rate limit reached","param":"","type":"rate_limit_error"}`)},
		{"anthropic error", newAnthropicError(t, 529, "req_1", "ws_1", `{"type":"error","error":{"type":"overloaded_error","message":"Overloaded"}}`)},
		{"transport error", &net.DNSError{Err: "no such host", Name: "api.openai.com", IsNotFound: true}},
		{"unclassified error", errors.New("boom")},
		{"nil", nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			wantStatus := UpstreamStatus(c.err, http.StatusInternalServerError)
			wantMessage := UpstreamMessage(c.err)

			got := ClassifyUpstreamFailure(c.err, http.StatusInternalServerError)
			if got.Status != wantStatus {
				t.Errorf("ClassifyUpstreamFailure(...).Status = %d, want %d", got.Status, wantStatus)
			}
			if got.Message != wantMessage {
				t.Errorf("ClassifyUpstreamFailure(...).Message = %q, want %q", got.Message, wantMessage)
			}
		})
	}
}

func TestClassifyUpstreamFailure_EmptyResponse(t *testing.T) {
	empty := &EmptyResponseError{Err: errors.New("stream ended without a final response")}
	if got := ClassifyUpstreamFailure(empty, http.StatusInternalServerError); got.Status != http.StatusBadGateway {
		t.Errorf("empty response status = %d, want %d", got.Status, http.StatusBadGateway)
	}

	// The provider's own status still wins over the empty-response default.
	sdk := &EmptyResponseError{Err: newOpenAIError(t, 429, `{"code":"rate_limit_exceeded","message":"slow down","param":"","type":"rate_limit_error"}`)}
	if got := ClassifyUpstreamFailure(sdk, http.StatusInternalServerError); got.Status != 429 {
		t.Errorf("empty response with provider status = %d, want 429", got.Status)
	}
}

// Codex, gateways and TB's own bare-message writers answer with bodies that are
// not the OpenAI {"error":{...}} envelope. The SDK must still surface the HTTP
// status instead of "cannot unmarshal string into ... apierror.Error".
func TestUpstreamMessage_OpenAINonEnvelopeBodies(t *testing.T) {
	for name, body := range map[string]string{
		"string error": `{"error":"model not supported"}`,
		"detail":       `{"detail":"model not supported"}`,
		"plain text":   `model not supported`,
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()

			c := openai.NewClient(option.WithBaseURL(srv.URL), option.WithAPIKey("k"), option.WithMaxRetries(0))
			_, err := c.Chat.Completions.New(context.Background(), openai.ChatCompletionNewParams{
				Model:    "m",
				Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
			})
			if err == nil {
				t.Fatal("expected an error")
			}
			if strings.Contains(err.Error(), "cannot unmarshal") {
				t.Fatalf("decode error leaked instead of upstream error: %v", err)
			}
			f := ClassifyUpstreamFailure(err, 0)
			if f.Status != http.StatusBadRequest || !strings.Contains(f.Message, "model not supported") {
				t.Errorf("got status=%d message=%q", f.Status, f.Message)
			}
		})
	}
}
