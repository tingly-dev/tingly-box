package peertransport_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	aoption "github.com/anthropics/anthropic-sdk-go/option"
	openai "github.com/openai/openai-go/v3"
	ooption "github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/quic-go/quic-go/http3"
	"github.com/tingly-dev/tingly-box/internal/peertransport"
)

// Real SDKs, TLS and QUIC; only application responses are synthetic.
func sdkPeer(t *testing.T, handler http.Handler) (string, *http.Client) {
	t.Helper()
	tcp := httptest.NewUnstartedServer(nil)
	_, port, err := net.SplitHostPort(tcp.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	tcp.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil {
			w.Header().Set("Alt-Svc", `h3=":`+port+`"; ma=300`)
		}
		if r.Method == http.MethodGet {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		peertransport.ProbeHandler(handler).ServeHTTP(w, r)
	})
	tcp.StartTLS()
	t.Cleanup(tcp.Close)
	udp, err := net.ListenPacket("udp", tcp.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	h3 := &http3.Server{Handler: tcp.Config.Handler, TLSConfig: tcp.TLS.Clone()}
	go func() { _ = h3.Serve(udp) }()
	t.Cleanup(func() { _ = h3.Close(); _ = udp.Close() })
	roots := x509.NewCertPool()
	roots.AddCert(tcp.Certificate())
	base := http.DefaultTransport.(*http.Transport).Clone()
	base.Proxy = nil
	base.TLSClientConfig = &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}
	transport := peertransport.New(base)
	t.Cleanup(func() { _ = transport.Close() })
	client := &http.Client{Transport: transport, Timeout: 5 * time.Second}
	for _, path := range []string{"/chat/completions", "/responses", "/v1/messages"} {
		deadline := time.Now().Add(4 * time.Second)
		for {
			res, err := client.Get(tcp.URL + path)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, res.Body)
			_ = res.Body.Close()
			if res.ProtoMajor == 3 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("peer never selected HTTP/3 for %s", path)
			}
			time.Sleep(5 * time.Millisecond)
		}
	}
	return tcp.URL, client
}

func sdkSSE(w http.ResponseWriter, events string) {
	w.Header().Set("Content-Type", "text/event-stream")
	_, _ = io.WriteString(w, events)
	w.(http.Flusher).Flush()
}

func TestSDKStreamingOverDiscoveredHTTP3(t *testing.T) {
	baseURL, client := sdkPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warmup" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.ProtoMajor != 3 {
			t.Errorf("SDK request used %s, want HTTP/3", r.Proto)
		}
		switch r.URL.Path {
		case "/chat/completions":
			if r.Header.Get("Authorization") != "Bearer synthetic-openai" {
				t.Error("missing OpenAI credential")
			}
			sdkSSE(w, "data: {\"id\":\"c\",\"object\":\"chat.completion.chunk\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}]}\n\ndata: [DONE]\n\n")
		case "/responses":
			sdkSSE(w, "event: response.output_text.delta\ndata: {\"type\":\"response.output_text.delta\",\"delta\":\"hello\",\"output_index\":0,\"content_index\":0,\"item_id\":\"i\",\"sequence_number\":0}\n\nevent: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"id\":\"r\",\"status\":\"completed\",\"output\":[]},\"sequence_number\":1}\n\n")
		case "/v1/messages":
			if r.Header.Get("X-Api-Key") != "synthetic-anthropic" {
				t.Error("missing Anthropic credential")
			}
			sdkSSE(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\nevent: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		default:
			http.NotFound(w, r)
		}
	}))
	oa := openai.NewClient(ooption.WithoutEnvironmentDefaults(), ooption.WithAPIKey("synthetic-openai"), ooption.WithBaseURL(baseURL), ooption.WithHTTPClient(client), ooption.WithMaxRetries(0))
	t.Run("OpenAI Chat", func(t *testing.T) {
		stream := oa.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{Model: "synthetic", Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")}})
		defer stream.Close()
		var text strings.Builder
		for stream.Next() {
			for _, choice := range stream.Current().Choices {
				text.WriteString(choice.Delta.Content)
			}
		}
		if stream.Err() != nil || text.String() != "hello" {
			t.Fatalf("text=%q error=%v", text.String(), stream.Err())
		}
	})
	t.Run("OpenAI Responses", func(t *testing.T) {
		stream := oa.Responses.NewStreaming(context.Background(), responses.ResponseNewParams{Model: "synthetic"})
		defer stream.Close()
		var events []string
		for stream.Next() {
			events = append(events, stream.Current().Type)
		}
		if stream.Err() != nil || strings.Join(events, ",") != "response.output_text.delta,response.completed" {
			t.Fatalf("events=%v error=%v", events, stream.Err())
		}
	})
	t.Run("Anthropic Messages", func(t *testing.T) {
		ac := anthropic.NewClient(aoption.WithoutEnvironmentDefaults(), aoption.WithAPIKey("synthetic-anthropic"), aoption.WithBaseURL(baseURL), aoption.WithHTTPClient(client), aoption.WithMaxRetries(0))
		stream := ac.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{Model: "synthetic", MaxTokens: 8, Messages: []anthropic.MessageParam{anthropic.NewUserMessage(anthropic.NewTextBlock("hi"))}})
		defer stream.Close()
		var events []string
		for stream.Next() {
			events = append(events, stream.Current().Type)
		}
		if stream.Err() != nil || strings.Join(events, ",") != "content_block_delta,message_stop" {
			t.Fatalf("events=%v error=%v", events, stream.Err())
		}
	})
}

func TestAnthropicSDKHTTP3CancellationReachesServer(t *testing.T) {
	canceled := make(chan struct{})
	baseURL, client := sdkPeer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/warmup" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if r.ProtoMajor != 3 {
			t.Errorf("request used %s", r.Proto)
		}
		sdkSSE(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"hello\"}}\n\n")
		<-r.Context().Done()
		close(canceled)
	}))
	ac := anthropic.NewClient(aoption.WithoutEnvironmentDefaults(), aoption.WithAPIKey("synthetic"), aoption.WithBaseURL(baseURL), aoption.WithHTTPClient(client), aoption.WithMaxRetries(0))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stream := ac.Messages.NewStreaming(ctx, anthropic.MessageNewParams{Model: "synthetic", MaxTokens: 8})
	defer stream.Close()
	if !stream.Next() {
		t.Fatalf("missing first SSE event: %v", stream.Err())
	}
	cancel()
	for stream.Next() {
	}
	if stream.Err() == nil {
		t.Fatal("canceled stream reported success")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("SDK cancellation did not reach the HTTP/3 handler")
	}
}
