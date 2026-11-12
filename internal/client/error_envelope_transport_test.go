package client

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go/v2"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type errorShape struct {
	name        string
	status      int
	contentType string
	body        string
	wantMessage string
}

// Every body an upstream, proxy or the gateway itself is seen returning for a
// failed call. None of them may surface as a JSON decode error; wantMessage is
// only set where the OpenAI SDK itself keeps the upstream text.
var errorShapes = []errorShape{
	{"error object", 429, "application/json", `{"error":{"message":"slow down","type":"rate_limit_error","code":"rate_limited","param":""}}`, "slow down"},
	{"error object with numeric code", 400, "application/json", `{"error":{"message":"bad param","code":400}}`, "bad param"},
	{"error string", 401, "application/json", `{"error":"invalid api key"}`, "invalid api key"},
	{"error string with success flag", 400, "application/json", `{"success":false,"error":"unknown profile 'x' for scenario 'team'"}`, "unknown profile 'x' for scenario 'team'"},
	{"detail string", 403, "application/json", `{"detail":"Forbidden for this account"}`, ""},
	{"detail list", 422, "application/json", `{"detail":[{"loc":["body","model"],"msg":"field required"}]}`, ""},
	{"top-level message", 502, "application/json", `{"message":"bad gateway upstream"}`, ""},
	{"error null", 500, "application/json", `{"error":null,"message":"boom"}`, ""},
	{"json string body", 500, "application/json", `"just a string"`, ""},
	{"plain text", 503, "text/plain", "upstream connect error", ""},
	{"html", 502, "text/html", "<html><body>Bad Gateway</body></html>", ""},
	{"empty body", 504, "", ``, "Gateway Timeout"},
}

func errorServer(s errorShape) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.contentType != "" {
			w.Header().Set("Content-Type", s.contentType)
		}
		w.WriteHeader(s.status)
		_, _ = w.Write([]byte(s.body))
	}))
}

func TestOpenAIClient_NonEnvelopeErrorBodiesKeepStatusAndMessage(t *testing.T) {
	for _, shape := range errorShapes {
		for _, stream := range []bool{false, true} {
			name := shape.name
			if stream {
				name += "/stream"
			}
			t.Run(name, func(t *testing.T) {
				srv := errorServer(shape)
				defer srv.Close()

				provider := &typ.Provider{UUID: "p", Name: "p", APIBase: srv.URL, APIStyle: protocol.APIStyleOpenAI, Token: "k", Enabled: true}
				c, err := NewOpenAIClient(provider, "m", typ.SessionID{})
				if err != nil {
					t.Fatalf("NewOpenAIClient: %v", err)
				}
				params := openai.ChatCompletionNewParams{
					Model:    "m",
					Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
				}
				if stream {
					s := c.ChatCompletionsNewStreaming(context.Background(), params)
					for s.Next() {
					}
					err = s.Err()
				} else {
					_, err = c.ChatCompletionsNew(context.Background(), params)
				}
				assertUpstreamError(t, err, shape)
			})
		}
	}
}

func TestAnthropicClient_NonEnvelopeErrorBodiesKeepStatusAndMessage(t *testing.T) {
	for _, shape := range errorShapes {
		t.Run(shape.name, func(t *testing.T) {
			srv := errorServer(shape)
			defer srv.Close()

			provider := &typ.Provider{UUID: "p", Name: "p", APIBase: srv.URL, APIStyle: protocol.APIStyleAnthropic, Token: "k", Enabled: true}
			c, err := NewAnthropicClient(provider, "m", typ.SessionID{})
			if err != nil {
				t.Fatalf("NewAnthropicClient: %v", err)
			}
			_, err = c.MessagesNew(context.Background(), &anthropic.MessageNewParams{
				Model:     "m",
				MaxTokens: 8,
				Messages:  []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))},
			})
			assertUpstreamError(t, err, shape)
		})
	}
}

func assertUpstreamError(t *testing.T, err error, shape errorShape) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	if strings.Contains(err.Error(), "cannot unmarshal") || strings.Contains(err.Error(), "unexpected end of JSON") || strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("decode error leaked to the caller: %v", err)
	}
	if got := protocol.UpstreamStatus(err, 0); got != shape.status {
		t.Errorf("status = %d, want %d (err: %v)", got, shape.status, err)
	}
	if msg := protocol.UpstreamMessage(err); shape.wantMessage != "" && !strings.Contains(msg, shape.wantMessage) {
		t.Errorf("message %q does not carry upstream text %q", msg, shape.wantMessage)
	}
}

func TestEnvelopeErrorTransport_LeavesSuccessAndWellFormedErrorsUntouched(t *testing.T) {
	const wellFormed = `{"error":{"message":"slow down","type":"rate_limit_error","code":"x","param":""}}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/ok" {
			_, _ = w.Write([]byte(`{"error":"not an error, a 200 body"}`))
			return
		}
		w.WriteHeader(429)
		_, _ = w.Write([]byte(wellFormed))
	}))
	defer srv.Close()

	hc := &http.Client{Transport: wrapWithStringErrorEnvelope(http.DefaultTransport)}
	for path, want := range map[string]string{"/ok": `{"error":"not an error, a 200 body"}`, "/bad": wellFormed} {
		resp, err := hc.Get(srv.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if string(b) != want {
			t.Errorf("%s: body rewritten to %q", path, b)
		}
	}
}
