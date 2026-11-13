package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

const (
	blockMessage = "This content was flagged for possible cybersecurity risk."
	blockCreated = `{"type":"response.created","sequence_number":0,"response":{"id":"resp_x","object":"response","created_at":1,"status":"in_progress","model":"m","output":[]}}`
	blockDelta   = `{"type":"response.output_text.delta","sequence_number":1,"item_id":"i","output_index":0,"content_index":0,"delta":"Sure, "}`
)

// blockNested is the in-stream error event the backend sends, the shape seen
// in production; blockFlat is the documented flat Responses error event.
func blockNested(code, message string) string {
	return fmt.Sprintf(`{"type":"error","sequence_number":2,"error":{"type":"invalid_request","code":%q,"message":%q,"param":null}}`, code, message)
}

func blockFlat(code, message string) string {
	return fmt.Sprintf(`{"type":"error","sequence_number":2,"code":%q,"message":%q,"param":null}`, code, message)
}

func blockSSE(events ...string) string {
	var b strings.Builder
	for _, e := range events {
		b.WriteString("data: " + e + "\n\n")
	}
	return b.String()
}

func bodyOf(s string) func() io.Reader {
	return func() io.Reader { return strings.NewReader(s) }
}

// sseUpstream answers every request with HTTP 200 and a fresh body.
func sseUpstream(contentType string, body func() io.Reader) func(*http.Request) (*http.Response, error) {
	return func(req *http.Request) (*http.Response, error) {
		h := http.Header{}
		if contentType != "" {
			h.Set("Content-Type", contentType)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: h, Body: io.NopCloser(body()), Request: req}, nil
	}
}

// blockTestClient is a CodexClient whose transport is the real
// codexRoundTripper over a stub upstream (the codex_compaction_test.go setup).
func blockTestClient(upstream func(*http.Request) (*http.Response, error)) *CodexClient {
	return &CodexClient{OpenAIClient: &OpenAIClient{client: openai.NewClient(
		option.WithBaseURL("https://codex.test/backend-api/"), option.WithAPIKey("test"), option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: &codexRoundTripper{RoundTripper: rtPtr(upstream)}}),
	)}}
}

func drainBlockTestStream(stream *ssestream.Stream[responses.ResponseStreamEventUnion]) (types []string, err error) {
	defer stream.Close()
	for stream.Next() {
		types = append(types, stream.Current().Type)
	}
	return types, stream.Err()
}

func streamThroughCodex(contentType string, body func() io.Reader) ([]string, error) {
	c := blockTestClient(sseUpstream(contentType, body))
	return drainBlockTestStream(c.ResponsesNewStreaming(context.Background(), responses.ResponseNewParams{Model: "gpt-5-codex"}))
}

// An in-stream content-policy block ends the stream with a 400 *openai.Error
// that carries the upstream code and message; the events before it are
// delivered (#2417).
func TestCodexStreamPolicyBlock(t *testing.T) {
	for _, code := range []string{"cyber_policy", "bio_policy", "misalignment_policy_violation"} {
		for _, shape := range []string{"nested", "flat"} {
			t.Run(code+"/"+shape, func(t *testing.T) {
				event := blockNested(code, blockMessage)
				if shape == "flat" {
					event = blockFlat(code, blockMessage)
				}
				types, err := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, blockDelta, event)))
				assert.Equal(t, []string{"response.created", "response.output_text.delta"}, types)

				var apiErr *openai.Error
				require.ErrorAs(t, err, &apiErr)
				assert.Equal(t, http.StatusBadRequest, apiErr.StatusCode)
				assert.Equal(t, code, apiErr.Code)
				assert.Equal(t, blockMessage, apiErr.Message)
				assert.Contains(t, apiErr.RawJSON(), code, "the server log keeps the upstream object")
				if shape == "nested" {
					assert.Equal(t, "invalid_request", apiErr.Type)
				} else {
					assert.Empty(t, apiErr.Type, "the flat event's type is the event name")
				}

				// Usage tracking and health monitoring see the code, not the vendor text.
				assert.Equal(t, "OpenAI API error: 400 Bad Request (code="+code+", in-stream)", err.Error())

				// The gateway answers an upstream 400, through the wrapping on the way.
				failure := protocol.ClassifyUpstreamFailure(fmt.Errorf("stream error: %w", err), http.StatusInternalServerError)
				assert.Equal(t, http.StatusBadRequest, failure.Status)
				assert.Contains(t, failure.Message, "code="+code)
				assert.Contains(t, failure.Message, blockMessage)
			})
		}
	}
}

// A block inside the stream is reported like the backend's HTTP 400 for the
// same block, so status mapping and the client message treat the two alike.
func TestCodexStreamPolicyBlockMatchesHTTP400(t *testing.T) {
	_, streamErr := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, blockNested("cyber_policy", blockMessage))))

	httpBlock := blockTestClient(func(req *http.Request) (*http.Response, error) {
		body := `{"error":{"type":"invalid_request","code":"cyber_policy","message":"` + blockMessage + `","param":null}}`
		return &http.Response{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": {"application/json"}},
			Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})
	_, httpErr := drainBlockTestStream(httpBlock.ResponsesNewStreaming(context.Background(), responses.ResponseNewParams{Model: "gpt-5-codex"}))

	require.Error(t, httpErr)
	assert.Equal(t,
		protocol.ClassifyUpstreamFailure(httpErr, http.StatusInternalServerError),
		protocol.ClassifyUpstreamFailure(streamErr, http.StatusInternalServerError))
}

// Everything else is left as it was: other codes stay the SDK's StreamError,
// and a response.failed or a delta that mentions a policy code is an ordinary
// event.
func TestCodexStreamNotPolicyBlock(t *testing.T) {
	// The last two pass the "_policy" prefilter, so only the allowlist keeps
	// them from becoming a 400.
	for name, event := range map[string]string{
		"other code":                   blockNested("server_error", "overloaded"),
		"other policy-like code":       blockNested("content_policy_violation", "flagged"),
		"transient code, _policy text": blockNested("server_error", "see usage_policy"),
	} {
		t.Run(name, func(t *testing.T) {
			types, err := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, event)))
			assert.Equal(t, []string{"response.created"}, types)
			var streamErr *ssestream.StreamError
			require.ErrorAs(t, err, &streamErr)
			var apiErr *openai.Error
			assert.False(t, errors.As(err, &apiErr))
		})
	}
	t.Run("response.failed", func(t *testing.T) {
		failed := `{"type":"response.failed","sequence_number":2,"response":{"id":"resp_x","object":"response","created_at":1,"status":"failed","model":"m","output":[],"error":{"code":"cyber_policy","message":"m"}}}`
		types, err := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, failed)))
		require.NoError(t, err)
		assert.Equal(t, []string{"response.created", "response.failed"}, types)
	})
	t.Run("delta mentioning a code", func(t *testing.T) {
		delta := `{"type":"response.output_text.delta","sequence_number":1,"item_id":"i","output_index":0,"content_index":0,"delta":"the code was cyber_policy"}`
		types, err := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, delta)))
		require.NoError(t, err)
		assert.Equal(t, []string{"response.created", "response.output_text.delta"}, types)
	})
	t.Run("flat event with other code", func(t *testing.T) {
		types, err := streamThroughCodex("text/event-stream", bodyOf(blockSSE(blockCreated, blockFlat("server_error", "overloaded"))))
		require.NoError(t, err)
		assert.Equal(t, []string{"response.created", "error"}, types)
	})
}

// The recognition depends on one piece of wiring: the Content-Type that
// codexRoundTripper sets selects codexEventDecoder. If either side changes, or
// an SDK update changes how decoders are matched, the SDK falls back to its
// plain decoder without an error and blocks become 5xx again.
func TestCodexStreamDecoderSelected(t *testing.T) {
	d := ssestream.NewDecoder(&http.Response{Header: http.Header{"Content-Type": {codexSSEContentType}}, Body: io.NopCloser(strings.NewReader(""))})
	assert.IsType(t, &codexEventDecoder{}, d)

	sse := blockSSE(blockCreated, blockNested("cyber_policy", blockMessage))
	for _, upstreamType := range []string{"", "text/event-stream", "text/event-stream; charset=utf-8", "Text/Event-Stream"} {
		t.Run(fmt.Sprintf("upstream %q", upstreamType), func(t *testing.T) {
			_, err := streamThroughCodex(upstreamType, bodyOf(sse))
			var apiErr *openai.Error
			assert.ErrorAs(t, err, &apiErr)
		})
	}

	// Control: the same bytes through a client without codexRoundTripper.
	plain := openai.NewClient(option.WithBaseURL("https://api.test/v1/"), option.WithAPIKey("test"), option.WithMaxRetries(0),
		option.WithHTTPClient(&http.Client{Transport: rtPtr(sseUpstream("text/event-stream", bodyOf(sse)))}))
	_, err := drainBlockTestStream(plain.Responses.NewStreaming(context.Background(), responses.ResponseNewParams{Model: "m"}))
	var streamErr *ssestream.StreamError
	assert.ErrorAs(t, err, &streamErr)
}

// The SDK parses the SSE framing before the check runs, so framing variants
// are recognized too.
func TestCodexStreamPolicyBlockFraming(t *testing.T) {
	event := blockNested("cyber_policy", blockMessage)
	split := strings.Index(event, `"error":`)
	cases := map[string]func() io.Reader{
		"crlf":                     bodyOf("data: " + blockCreated + "\r\n\r\ndata: " + event + "\r\n\r\n"),
		"event lines and comments": bodyOf(": keep-alive\n\nevent: response.created\ndata: " + blockCreated + "\n\nevent: error\ndata: " + event + "\n\n"),
		"no space after colon":     bodyOf("data:" + blockCreated + "\n\ndata:" + event + "\n\n"),
		"data over two lines":      bodyOf("data: " + blockCreated + "\n\ndata: " + event[:split] + "\ndata: " + event[split:] + "\n\n"),
		"one byte per read": func() io.Reader {
			return iotest.OneByteReader(strings.NewReader(blockSSE(blockCreated, event)))
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			types, err := streamThroughCodex("text/event-stream", body)
			assert.Equal(t, []string{"response.created"}, types)
			var apiErr *openai.Error
			require.ErrorAs(t, err, &apiErr)
			assert.Equal(t, "cyber_policy", apiErr.Code)
		})
	}
}

type closeRecorder struct {
	io.Reader
	closed int
}

func (r *closeRecorder) Close() error { r.closed++; return nil }

// Ending the stream at a block closes the upstream body. The flat shape is
// used because the SDK alone would deliver it, and what follows it, as
// ordinary events.
func TestCodexStreamPolicyBlockClosesBody(t *testing.T) {
	body := &closeRecorder{Reader: strings.NewReader(blockSSE(blockCreated, blockFlat("cyber_policy", blockMessage), blockDelta))}
	c := blockTestClient(func(req *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body, Request: req}, nil
	})
	types, err := drainBlockTestStream(c.ResponsesNewStreaming(context.Background(), responses.ResponseNewParams{Model: "gpt-5-codex"}))
	assert.Equal(t, []string{"response.created"}, types, "nothing after the block is delivered")
	require.Error(t, err)
	assert.Equal(t, 1, body.closed)
}

// codexPolicyBlock skips any event without "_policy" before looking for a
// code, so every listed code must contain it.
func TestCodexPolicyBlockCodesPassPrefilter(t *testing.T) {
	for code := range codexPolicyBlockCodes {
		assert.Contains(t, code, "_policy")
	}
}

// The non-streaming assembly and image generation keep the error reachable
// through their wrapping, so they answer 400 as well.
func TestCodexPolicyBlockThroughClientCalls(t *testing.T) {
	c := blockTestClient(sseUpstream("text/event-stream", bodyOf(blockSSE(blockCreated, blockNested("bio_policy", blockMessage)))))

	_, err := c.ResponsesNew(context.Background(), responses.ResponseNewParams{Model: "gpt-5-codex"})
	var apiErr *openai.Error
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, "bio_policy", apiErr.Code)

	_, err = c.ImagesGenerate(context.Background(), openai.ImageGenerateParams{Model: "gpt-image-1", Prompt: "a cat"})
	require.ErrorAs(t, err, &apiErr)
	assert.Equal(t, http.StatusBadRequest, protocol.ClassifyUpstreamFailure(err, http.StatusInternalServerError).Status)
}
