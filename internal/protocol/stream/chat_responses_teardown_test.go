package stream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/packages/ssestream"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
)

// Keep the real SDK decoder in the path: these errors arise while reading the
// HTTP body, after all preceding SSE frames have been consumed.
type chatTeardownReader struct {
	err   error
	reads int
}

func (r *chatTeardownReader) Read([]byte) (int, error) {
	r.reads++
	return 0, r.err
}

func chatTeardownStream(raw string, tail *chatTeardownReader) *ssestream.Stream[openai.ChatCompletionChunk] {
	return ssestream.NewStream[openai.ChatCompletionChunk](ssestream.NewDecoder(&http.Response{
		Header: http.Header{"Content-Type": []string{"text/event-stream"}},
		Body:   io.NopCloser(io.MultiReader(strings.NewReader(raw), tail)),
	}), nil)
}

const chatTeardownUsage = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":100,\"completion_tokens\":5,\"total_tokens\":105}}\n\n"

func chatTeardownFinish(reason string) string {
	return fmt.Sprintf("data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":%q}]}\n\n", reason)
}

func TestChatToResponsesTransportTeardown(t *testing.T) {
	reset := &net.OpError{Op: "read", Net: "tcp", Err: syscall.ECONNRESET}
	for _, tc := range []struct {
		name, finish, extra, terminal, incomplete string
		readErr                                   error
		usage                                     bool
	}{
		{name: "unexpected EOF with usage", finish: "stop", usage: true, readErr: io.ErrUnexpectedEOF, terminal: "response.completed"},
		{name: "wrapped unexpected EOF", finish: "stop", usage: true, readErr: fmt.Errorf("read response: %w", io.ErrUnexpectedEOF), terminal: "response.completed"},
		{name: "connection reset with usage", finish: "stop", usage: true, readErr: reset, terminal: "response.completed"},
		{name: "connection aborted", finish: "stop", usage: true, readErr: syscall.ECONNABORTED, terminal: "response.completed"},
		{name: "no usage keeps fallback", finish: "stop", readErr: io.ErrUnexpectedEOF, terminal: "response.completed"},
		{name: "clean EOF without DONE", finish: "stop", usage: true, readErr: io.EOF, terminal: "response.completed"},
		{name: "length remains incomplete", finish: "length", usage: true, readErr: reset, terminal: "response.incomplete", incomplete: "max_output_tokens"},
		{name: "filtered remains incomplete", finish: "content_filter", usage: true, readErr: io.ErrUnexpectedEOF, terminal: "response.incomplete", incomplete: "content_filter"},
		{name: "midstream unexpected EOF", readErr: io.ErrUnexpectedEOF},
		{name: "midstream reset", readErr: reset},
		{name: "missing finish on clean EOF", readErr: io.EOF},
		{name: "cancellation", finish: "stop", usage: true, readErr: context.Canceled},
		{name: "deadline", finish: "stop", usage: true, readErr: context.DeadlineExceeded},
		{name: "cancellation wins over EOF", finish: "stop", usage: true, readErr: errors.Join(context.Canceled, io.ErrUnexpectedEOF)},
		{name: "deadline wins over reset", finish: "stop", usage: true, readErr: errors.Join(context.DeadlineExceeded, reset)},
		{name: "socket timeout", finish: "stop", usage: true, readErr: &net.OpError{Op: "read", Net: "tcp", Err: syscall.ETIMEDOUT}},
		{name: "unknown error", finish: "stop", usage: true, readErr: errors.New("unexpected EOF")},
		{name: "provider failure wins over EOF", finish: "stop", usage: true, readErr: errors.Join(&ssestream.StreamError{Message: "provider failed"}, io.ErrUnexpectedEOF)},
		{name: "API failure wins over reset", finish: "stop", usage: true, readErr: errors.Join(&openai.Error{Message: "provider failed"}, reset)},
		{name: "explicit SSE failure", finish: "stop", usage: true, extra: "data: {\"error\":{\"message\":\"provider failed\"}}\n\n", readErr: io.ErrUnexpectedEOF},
		{name: "malformed SSE payload", finish: "stop", usage: true, extra: "data: {broken}\n\n", readErr: io.ErrUnexpectedEOF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw := chatTeardownFinish(tc.finish)
			if tc.usage {
				raw += chatTeardownUsage
			}
			upstream := chatTeardownStream(raw+tc.extra, &chatTeardownReader{err: tc.readErr})
			defer upstream.Close()
			conv := NewChatToResponsesConverter(upstream, "model", nil)
			var terminals []string
			var terminalErr error
			for i := 0; ; i++ {
				require.Less(t, i, 30, "converter must terminate")
				event, done, err := conv.Next()
				if err != nil || done {
					terminalErr = err
					break
				}
				var response *wire.ResponsesWireResponse
				switch e := event.(type) {
				case wire.ResponsesCompletedEvent:
					terminals = append(terminals, e.Type)
					response = &e.Response
				case wire.ResponsesIncompleteEvent:
					terminals = append(terminals, e.Type)
					response = &e.Response
					require.Equal(t, tc.incomplete, e.Response.IncompleteDetails.Reason)
				}
				if response != nil {
					require.Equal(t, "hello", response.Output[0].Content[0].Text)
					if tc.usage {
						require.EqualValues(t, 100, response.Usage.InputTokens)
						require.EqualValues(t, 5, response.Usage.OutputTokens)
					} else {
						require.EqualValues(t, 1, response.Usage.OutputTokens, "retain existing estimation when usage is absent")
					}
				}
			}
			if tc.terminal == "" {
				require.Error(t, terminalErr)
				require.Empty(t, terminals, "failure must not emit a success/incomplete terminal event")
				return
			}
			require.NoError(t, terminalErr)
			require.Equal(t, []string{tc.terminal}, terminals)
			_, done, err := conv.Next()
			require.NoError(t, err, "terminal state must not expose the recovered SDK error on a later Next")
			require.True(t, done)
		})
	}
}

func TestChatToResponsesDoneStopsBeforeConnectionClose(t *testing.T) {
	tail := &chatTeardownReader{err: errors.New("read past DONE")}
	upstream := chatTeardownStream(chatTeardownFinish("stop")+chatTeardownUsage+"data: [DONE]\n\n", tail)
	defer upstream.Close()
	conv := NewChatToResponsesConverter(upstream, "model", nil)
	completed := 0
	for i := 0; ; i++ {
		require.Less(t, i, 30)
		event, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		if e, ok := event.(wire.ResponsesCompletedEvent); ok {
			completed++
			require.EqualValues(t, 100, e.Response.Usage.InputTokens)
		}
	}
	require.Equal(t, 1, completed)
	require.Zero(t, tail.reads, "DONE must end iteration without reading to physical EOF")
}

func TestHandleChatToResponsesRecoveredTeardown(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("provider_error=%t", fail), func(t *testing.T) {
			raw := chatTeardownFinish("stop") + chatTeardownUsage
			if fail {
				raw += "data: {\"error\":{\"message\":\"provider failed\"}}\n\n"
			}
			upstream := chatTeardownStream(raw, &chatTeardownReader{err: io.ErrUnexpectedEOF})
			w := &closeNotifyRecorder{ResponseRecorder: httptest.NewRecorder()}
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("POST", "/v1/responses", nil)
			completed, failed := 0, 0
			hc := protocol.NewHandleContext(c, "model").
				WithOnStreamComplete(func() { completed++ }).
				WithOnStreamError(func(error) { failed++ })
			usage, err := HandleOpenAIChatToResponsesStream(hc, upstream, "model")
			require.Equal(t, 100, usage.InputTokens)
			require.Equal(t, 5, usage.OutputTokens)
			if fail {
				require.ErrorContains(t, err, "provider failed")
				require.Zero(t, completed)
				require.Equal(t, 1, failed)
				require.NotContains(t, w.Body.String(), "event: response.completed")
				require.NotContains(t, w.Body.String(), "data: [DONE]")
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, completed)
			require.Zero(t, failed)
			require.Equal(t, 1, strings.Count(w.Body.String(), "event: response.completed"))
			require.Contains(t, w.Body.String(), "data: [DONE]")
			require.NotContains(t, w.Body.String(), "event: error")
		})
	}
}

func TestChatToResponsesTeardownPreservesToolCall(t *testing.T) {
	raw := `data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_1","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]}}]}` + "\n\n" +
		`data: {"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" + chatTeardownUsage
	upstream := chatTeardownStream(raw, &chatTeardownReader{err: io.ErrUnexpectedEOF})
	defer upstream.Close()
	conv := NewChatToResponsesConverter(upstream, "model", nil)
	var completed []wire.ResponsesCompletedEvent
	var argumentsDone []string
	for i := 0; ; i++ {
		require.Less(t, i, 30)
		event, done, err := conv.Next()
		require.NoError(t, err)
		if done {
			break
		}
		switch e := event.(type) {
		case wire.ResponsesFunctionCallArgumentsDoneEvent:
			argumentsDone = append(argumentsDone, e.Arguments)
		case wire.ResponsesCompletedEvent:
			completed = append(completed, e)
		}
	}
	require.Equal(t, []string{`{"id":9007199254740993}`}, argumentsDone)
	require.Len(t, completed, 1)
	response := completed[0].Response
	require.EqualValues(t, 100, response.Usage.InputTokens)
	require.Len(t, response.Output, 1)
	item := response.Output[0]
	require.Equal(t, "function_call", item.Type)
	require.Equal(t, "call_1", item.CallID)
	require.Equal(t, "lookup", item.Name)
	require.NotNil(t, item.Arguments)
	require.Equal(t, `{"id":9007199254740993}`, *item.Arguments)
}
