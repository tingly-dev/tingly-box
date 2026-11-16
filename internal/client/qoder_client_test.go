package client

import (
	"bufio"
	"context"
	"crypto/md5"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

type qoderTestTransport func(*http.Request) (*http.Response, error)

func (f qoderTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func decodeQoderBody(s string) ([]byte, error) {
	n := len(s) / 3
	s = s[len(s)-n:] + s[n:len(s)-n] + s[:n]
	const standard = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789+/"
	b := []byte(s)
	for i, c := range b {
		if c == '$' {
			b[i] = '='
		} else {
			j := strings.IndexByte(qoderAlphabet, c)
			if j < 0 {
				return nil, io.ErrUnexpectedEOF
			}
			b[i] = standard[j]
		}
	}
	return base64.StdEncoding.DecodeString(string(b))
}
func TestQoderTransport(t *testing.T) {
	p := &typ.Provider{APIBase: "https://api2.qoder.sh", OAuthDetail: &ai.OAuthDetail{AccessToken: "secret", ExtraFields: map[string]any{"account_id": "user", oauth.QoderMachineID: "machine", "qoder_organization_id": "org", "qoder_organization_tags": []any{"tag"}, "qoder_data_policy_agreed": true}}}
	rt := &qoderTransport{provider: p, inner: qoderTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path != "/algo"+qoderPath || r.URL.Query().Get("Encode") != "1" {
			t.Errorf("wrong upstream URL: %v", r.URL)
		}
		if r.Header.Get("Cosy-User") != "user" || r.Header.Get("Cosy-MachineId") != "machine" || r.Header.Get("X-Model-Key") != "test-model" {
			t.Error("missing identity or model headers")
		}
		body, _ := io.ReadAll(r.Body)
		auth := strings.Split(r.Header.Get("Authorization"), ".")
		if len(auth) != 3 {
			t.Fatal("invalid authorization")
		}
		sum := md5.Sum([]byte(strings.Join([]string{auth[1], r.Header.Get("Cosy-Key"), r.Header.Get("Cosy-Date"), string(body), qoderPath}, "\n")))
		if auth[2] != hex.EncodeToString(sum[:]) {
			t.Error("wrong signature")
		}
		raw, err := decodeQoderBody(string(body))
		if err != nil {
			t.Fatal(err)
		}
		var parsed map[string]any
		if err = json.Unmarshal(raw, &parsed); err != nil {
			t.Fatal(err)
		}
		if parsed["system"] != "be helpful" || parsed["agent_id"] != "agent_common" {
			t.Errorf("bad request: %s", raw)
		}
		stream := "data: {\"statusCodeValue\":200,\"body\":\"[DONE]\"}\n\ndata: {\"statusCodeValue\":200,\"body\":\"{\\\"id\\\":\\\"id1\\\",\\\"object\\\":\\\"chat.completion.chunk\\\",\\\"choices\\\":[{\\\"index\\\":0,\\\"delta\\\":{\\\"content\\\":\\\"hi\\\"}}]}\"}\n\nevent:finish\ndata: {}\n\n"
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(stream))}, nil
	})}
	req, _ := http.NewRequestWithContext(context.Background(), "POST", "https://api2.qoder.sh/chat/completions", strings.NewReader(`{"model":"test-model","messages":[{"role":"system","content":"be helpful"},{"role":"user","content":"hello"}],"stream":true}`))
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"content":"hi"`) || !strings.Contains(string(out), `"model":"test-model"`) || !strings.HasSuffix(string(out), "data: [DONE]\n\n") {
		t.Fatalf("bad stream: %s", out)
	}
}
func TestQoderStreamRejectsMissingFinish(t *testing.T) {
	b := &qoderSSEBody{src: io.NopCloser(strings.NewReader("")), reader: bufio.NewReader(strings.NewReader("data: {\"statusCodeValue\":200,\"body\":\"[DONE]\"}\n\n"))}
	_, err := io.ReadAll(b)
	if err != io.ErrUnexpectedEOF {
		t.Fatalf("error = %v", err)
	}
}

func TestQoderClientCompletesStreamingAndNonStreaming(t *testing.T) {
	provider := &typ.Provider{APIBase: "https://api2.qoder.sh", OAuthDetail: &ai.OAuthDetail{AccessToken: "secret", ExtraFields: map[string]any{"account_id": "user", oauth.QoderMachineID: "machine"}}}
	inner := qoderTestTransport(func(r *http.Request) (*http.Response, error) {
		s := `data: {"statusCodeValue":200,"body":"{\"id\":\"chat-id\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"auto\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"hello\"},\"finish_reason\":null}]}"}` + "\n\n" + `data: {"statusCodeValue":200,"body":"{\"id\":\"chat-id\",\"object\":\"chat.completion.chunk\",\"created\":1,\"model\":\"auto\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}"}` + "\n\nevent:finish\ndata: {}\n\n"
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(s)), Request: r}, nil
	})
	base, err := newOpenAIClientWithTransport(provider, provider.APIBase, &qoderTransport{provider: provider, inner: inner})
	if err != nil {
		t.Fatal(err)
	}
	client := &QoderClient{OpenAIClient: base}
	var params openai.ChatCompletionNewParams
	if err = json.Unmarshal([]byte(`{"model":"auto","messages":[{"role":"user","content":"hello"}]}`), &params); err != nil {
		t.Fatal(err)
	}
	result, err := client.ChatCompletionsNew(context.Background(), params)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Choices) != 1 || result.Choices[0].Message.Content != "hello" {
		t.Fatalf("completion: %#v", result)
	}
	stream := client.ChatCompletionsNewStreaming(context.Background(), params)
	defer stream.Close()
	count := 0
	for stream.Next() {
		count++
	}
	if err := stream.Err(); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("got %d chunks", count)
	}
}

func TestQoderEncodingVector(t *testing.T) {
	if got := string(qoderEncode([]byte("hello"))); got != "q$FruHPH" {
		t.Fatalf("encoded = %q", got)
	}
}

func TestQoderSystemMessagesAreCombinedFirst(t *testing.T) {
	chat := map[string]any{"messages": []any{map[string]any{"role": "user", "content": "hi"}, map[string]any{"role": "developer", "content": "first"}, map[string]any{"role": "system", "content": "second"}}}
	raw, err := qoderChatBody(chat, "auto")
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		System   string           `json:"system"`
		Messages []map[string]any `json:"messages"`
	}
	if err = json.Unmarshal(raw, &body); err != nil {
		t.Fatal(err)
	}
	if body.System != "first\n\nsecond" || len(body.Messages) != 2 || body.Messages[0]["role"] != "system" || body.Messages[0]["content"] != body.System {
		t.Fatalf("incorrect system messages: %s", raw)
	}
}
