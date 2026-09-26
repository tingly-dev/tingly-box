package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/forwarding"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/anthropicbridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/openaibridge"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/responsesbridge"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// upstreamRequest is the part of a provider-bound HTTP request the wire
// contract covers.
type upstreamRequest struct {
	Method, Path, Query, AnthropicBeta, Body string
}

// fakeProvider serves a scenario's fixtures by endpoint path and records every
// request it receives.
type fakeProvider struct {
	*httptest.Server
	mu       sync.Mutex
	requests []upstreamRequest
	status   int // non-zero: fail every request with this status
}

func newFakeProvider(t *testing.T, s scenario.Scenario) *fakeProvider {
	t.Helper()
	fake := &fakeProvider{}
	fake.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		fake.mu.Lock()
		fake.requests = append(fake.requests, upstreamRequest{
			Method:        r.Method,
			Path:          r.URL.Path,
			Query:         r.URL.RawQuery,
			AnthropicBeta: r.Header.Get("anthropic-beta"),
			Body:          canonical(t, body),
		})
		status := fake.status
		fake.mu.Unlock()

		format := scenario.FormatOpenAIChat
		switch {
		case strings.HasSuffix(r.URL.Path, "/messages"):
			format = scenario.FormatAnthropic
		case strings.HasSuffix(r.URL.Path, "/responses"):
			format = scenario.FormatOpenAIResponses
		}
		if status != 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
			return
		}
		mock := s.MockResponses[format]
		if strings.Contains(string(body), `"stream":true`) {
			sse.WriteSSEResponse(w, mock.Stream())
			return
		}
		code, payload := mock.NonStream()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(code)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(fake.Close)
	return fake
}

func (f *fakeProvider) take() []upstreamRequest {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := f.requests
	f.requests = nil
	return out
}

func (f *fakeProvider) config(style protocol.APIStyle) Config {
	base := f.URL
	if style == protocol.APIStyleOpenAI {
		base += "/v1"
	}
	return Config{
		Clients: client.NewClientPool(),
		Provider: &typ.Provider{
			UUID: "fake", Name: "fake", APIBase: base, APIStyle: style,
			Token: "test-token", Enabled: true, Timeout: 30,
		},
		Model: "provider-model",
	}
}

func canonical(t *testing.T, raw []byte) string {
	t.Helper()
	if len(raw) == 0 {
		return ""
	}
	var generic any
	require.NoError(t, json.Unmarshal(raw, &generic))
	out, err := json.Marshal(generic)
	require.NoError(t, err)
	return string(out)
}

func drain(t *testing.T, events stage.EventStream) []stage.Event {
	t.Helper()
	defer events.Close()
	var out []stage.Event
	for {
		event, err := events.Next(context.Background())
		if errors.Is(err, io.EOF) {
			return out
		}
		require.NoError(t, err)
		out = append(out, event)
	}
}

func run(t *testing.T, endpoint stage.Endpoint, req any, streaming bool) {
	t.Helper()
	call := stage.Call{Request: req}
	if streaming {
		events, err := endpoint.Stream(context.Background(), call)
		require.NoError(t, err)
		require.NotEmpty(t, drain(t, events))
		return
	}
	_, err := endpoint.Complete(context.Background(), call)
	require.NoError(t, err)
}

// The terminal endpoints must reach the provider exactly as the forwarding
// calls the legacy handlers make: same method, path, query, beta header and
// body. For the V1 wire the legacy request is the client's own V1 request and
// the endpoint receives its Beta upgrade, so this pins the whole
// upgrade -> chain -> downgrade round trip on the wire.
func TestUpstreamWireMatchesDirectForward(t *testing.T) {
	anthropicBodies := map[string]string{
		"text": `{"model":"provider-model","max_tokens":64,"messages":[{"role":"user","content":"hi"}]}`,
		"system_tools": `{"model":"provider-model","max_tokens":64,
			"system":[{"type":"text","text":"sys","cache_control":{"type":"ephemeral"}}],
			"tools":[{"name":"get_weather","input_schema":{"type":"object","properties":{"location":{"type":"string"}}}}],
			"tool_choice":{"type":"auto"},
			"messages":[{"role":"user","content":"weather?"},
				{"role":"assistant","content":[{"type":"tool_use","id":"toolu_1","name":"get_weather","input":{"location":"Paris"}}]},
				{"role":"user","content":[{"type":"tool_result","tool_use_id":"toolu_1","content":"18C"}]}]}`,
		"thinking": `{"model":"provider-model","max_tokens":2048,"thinking":{"type":"enabled","budget_tokens":1024},
			"messages":[{"role":"user","content":"think"}]}`,
	}
	fake := newFakeProvider(t, scenario.TextScenario())
	ctx := context.Background()

	for name, body := range anthropicBodies {
		for _, streaming := range []bool{false, true} {
			t.Run("anthropic_v1/"+name+"/"+mode(streaming), func(t *testing.T) {
				config := fake.config(protocol.APIStyleAnthropic)
				var v1 anthropic.MessageNewParams
				require.NoError(t, json.Unmarshal([]byte(body), &v1))
				fc := forwarding.NewForwardContext(ctx, config.Provider)
				wrapper := config.Clients.GetAnthropicClient(ctx, config.Provider, config.Model)
				if streaming {
					stream, cancel, err := forwarding.ForwardAnthropicV1Stream(fc, wrapper, &v1)
					require.NoError(t, err)
					for stream.Next() {
					}
					require.NoError(t, stream.Err())
					cancel()
				} else {
					_, cancel, err := forwarding.ForwardAnthropicV1(fc, wrapper, &v1)
					require.NoError(t, err)
					cancel()
				}
				legacy := fake.take()

				beta, err := request.ConvertAnthropicV1ToBetaRequestWithError(&v1)
				require.NoError(t, err)
				endpoint, err := NewAnthropic(config, AnthropicWireV1)
				require.NoError(t, err)
				run(t, endpoint, beta, streaming)
				require.Equal(t, legacy, fake.take())
				require.Len(t, legacy, 1)
				require.Empty(t, legacy[0].Query, "V1 wire must not use ?beta=true")
			})

			t.Run("anthropic_beta/"+name+"/"+mode(streaming), func(t *testing.T) {
				config := fake.config(protocol.APIStyleAnthropic)
				var beta anthropic.BetaMessageNewParams
				require.NoError(t, json.Unmarshal([]byte(body), &beta))
				fc := forwarding.NewForwardContext(ctx, config.Provider)
				wrapper := config.Clients.GetAnthropicClient(ctx, config.Provider, config.Model)
				if streaming {
					stream, cancel, err := forwarding.ForwardAnthropicV1BetaStream(fc, wrapper, &beta)
					require.NoError(t, err)
					for stream.Next() {
					}
					require.NoError(t, stream.Err())
					cancel()
				} else {
					_, cancel, err := forwarding.ForwardAnthropicV1Beta(fc, wrapper, &beta)
					require.NoError(t, err)
					cancel()
				}
				legacy := fake.take()

				endpoint, err := NewAnthropic(config, AnthropicWireBeta)
				require.NoError(t, err)
				run(t, endpoint, &beta, streaming)
				require.Equal(t, legacy, fake.take())
				require.Equal(t, "beta=true", legacy[0].Query)
			})
		}
	}

	for _, streaming := range []bool{false, true} {
		t.Run("openai_chat/"+mode(streaming), func(t *testing.T) {
			config := fake.config(protocol.APIStyleOpenAI)
			newRequest := func() *openai.ChatCompletionNewParams {
				var req openai.ChatCompletionNewParams
				require.NoError(t, json.Unmarshal([]byte(`{"model":"provider-model","messages":[{"role":"user","content":"hi"}],"temperature":0.2}`), &req))
				if streaming {
					req.StreamOptions.IncludeUsage = openai.Bool(true)
				}
				return &req
			}
			fc := forwarding.NewForwardContext(ctx, config.Provider)
			wrapper := config.Clients.GetOpenAIClient(ctx, config.Provider, config.Model)
			if streaming {
				stream, cancel, err := forwarding.ForwardOpenAIChatStream(fc, wrapper, newRequest())
				require.NoError(t, err)
				for stream.Next() {
				}
				require.NoError(t, stream.Err())
				cancel()
			} else {
				_, cancel, err := forwarding.ForwardOpenAIChat(fc, wrapper, newRequest())
				require.NoError(t, err)
				cancel()
			}
			legacy := fake.take()

			endpoint, err := NewOpenAIChat(config)
			require.NoError(t, err)
			run(t, endpoint, newRequest(), streaming)
			require.Equal(t, legacy, fake.take())
		})

		t.Run("openai_responses/"+mode(streaming), func(t *testing.T) {
			config := fake.config(protocol.APIStyleOpenAI)
			var req responses.ResponseNewParams
			require.NoError(t, json.Unmarshal([]byte(`{"model":"provider-model","input":[{"type":"message","role":"user","content":[{"type":"input_text","text":"hi"}]}]}`), &req))
			fc := forwarding.NewForwardContext(ctx, config.Provider)
			wrapper := config.Clients.GetOpenAIClient(ctx, config.Provider, config.Model)
			if streaming {
				stream, cancel, err := forwarding.ForwardOpenAIResponsesStream(fc, wrapper, req)
				require.NoError(t, err)
				for stream.Next() {
				}
				require.NoError(t, stream.Err())
				cancel()
			} else {
				_, cancel, err := forwarding.ForwardOpenAIResponses(fc, wrapper, req)
				require.NoError(t, err)
				cancel()
			}
			legacy := fake.take()

			endpoint, err := NewOpenAIResponses(config)
			require.NoError(t, err)
			run(t, endpoint, &req, streaming)
			require.Equal(t, legacy, fake.take())
		})
	}
}

// Both Anthropic wires yield the same Beta response, usage and model, so
// every stage above the terminal is wire-agnostic.
func TestUpstreamAnthropicWiresAgree(t *testing.T) {
	fake := newFakeProvider(t, scenario.ToolUseScenario())
	req := &anthropic.BetaMessageNewParams{
		Model:     "provider-model",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("weather?"))},
	}

	type outcome struct {
		Complete    string
		Usage       *protocol.TokenUsage
		Model       string
		Events      []string
		StreamUsage *protocol.TokenUsage
		StreamModel string
	}
	collect := func(wire AnthropicWire) outcome {
		endpoint, err := NewAnthropic(fake.config(protocol.APIStyleAnthropic), wire)
		require.NoError(t, err)

		response, err := endpoint.Complete(context.Background(), stage.Call{Request: req})
		require.NoError(t, err)
		message, ok := response.Value.(*anthropic.BetaMessage)
		require.True(t, ok, "Complete returned %T", response.Value)
		require.Len(t, message.Content, 1)
		require.Equal(t, "get_weather", message.Content[0].Name)

		events, err := endpoint.Stream(context.Background(), stage.Call{Request: req})
		require.NoError(t, err)
		out := outcome{
			Complete: canonical(t, []byte(message.RawJSON())),
			Usage:    response.Usage,
			Model:    response.Model,
		}
		for {
			event, err := events.Next(context.Background())
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			beta, ok := event.Value.(anthropic.BetaRawMessageStreamEventUnion)
			require.True(t, ok, "Stream yielded %T", event.Value)
			out.Events = append(out.Events, canonical(t, []byte(beta.RawJSON())))
		}
		result := events.Result()
		require.NoError(t, events.Close())
		out.StreamUsage, out.StreamModel = result.Usage, result.Model
		return out
	}

	beta := collect(AnthropicWireBeta)
	require.True(t, beta.Usage.HasUsage())
	require.NotNil(t, beta.StreamUsage)
	require.True(t, beta.StreamUsage.HasUsage())
	require.NotEmpty(t, beta.Model)
	require.Equal(t, beta, collect(AnthropicWireV1))
}

func TestUpstreamOpenAIUsageAndModel(t *testing.T) {
	fake := newFakeProvider(t, scenario.TextScenario())
	config := fake.config(protocol.APIStyleOpenAI)

	chat, err := NewOpenAIChat(config)
	require.NoError(t, err)
	chatReq := &openai.ChatCompletionNewParams{
		Model:    "provider-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
	}
	response, err := chat.Complete(context.Background(), stage.Call{Request: chatReq})
	require.NoError(t, err)
	require.IsType(t, &openai.ChatCompletion{}, response.Value)
	require.True(t, response.Usage.HasUsage())
	require.Equal(t, "gpt-4o", response.Model)

	chatReq.StreamOptions.IncludeUsage = openai.Bool(true)
	events, err := chat.Stream(context.Background(), stage.Call{Request: chatReq})
	require.NoError(t, err)
	for _, event := range drain(t, events) {
		require.IsType(t, openai.ChatCompletionChunk{}, event.Value)
	}
	require.NotEmpty(t, events.Result().Model)

	respEndpoint, err := NewOpenAIResponses(config)
	require.NoError(t, err)
	respReq := &responses.ResponseNewParams{Model: "provider-model", Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("hi")}}
	response, err = respEndpoint.Complete(context.Background(), stage.Call{Request: respReq})
	require.NoError(t, err)
	require.IsType(t, &responses.Response{}, response.Value)
	require.True(t, response.Usage.HasUsage())

	events, err = respEndpoint.Stream(context.Background(), stage.Call{Request: respReq})
	require.NoError(t, err)
	for _, event := range drain(t, events) {
		require.IsType(t, responses.ResponseStreamEventUnion{}, event.Value)
	}
	require.NotNil(t, events.Result().Usage)
	require.True(t, events.Result().Usage.HasUsage())
}

// Provider errors pass through as the SDK's typed errors, which the bridges'
// ConvertError and the client edge rely on for status mapping.
func TestUpstreamProviderErrorsPassThrough(t *testing.T) {
	fake := newFakeProvider(t, scenario.TextScenario())
	fake.status = http.StatusTooManyRequests
	req := &anthropic.BetaMessageNewParams{
		Model:     "provider-model",
		MaxTokens: 64,
		Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("hi"))},
	}
	for _, wire := range []AnthropicWire{AnthropicWireBeta, AnthropicWireV1} {
		endpoint, err := NewAnthropic(fake.config(protocol.APIStyleAnthropic), wire)
		require.NoError(t, err)

		_, err = endpoint.Complete(context.Background(), stage.Call{Request: req})
		var apiErr *anthropic.Error
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)

		events, err := endpoint.Stream(context.Background(), stage.Call{Request: req})
		require.NoError(t, err)
		_, err = events.Next(context.Background())
		require.ErrorAs(t, err, &apiErr)
		require.Equal(t, http.StatusTooManyRequests, apiErr.StatusCode)
		require.NoError(t, events.Close())
	}

	chat, err := NewOpenAIChat(fake.config(protocol.APIStyleOpenAI))
	require.NoError(t, err)
	_, err = chat.Complete(context.Background(), stage.Call{Request: &openai.ChatCompletionNewParams{
		Model:    "provider-model",
		Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("hi")},
	}})
	var openAIErr *openai.Error
	require.ErrorAs(t, err, &openAIErr)
	require.Equal(t, http.StatusTooManyRequests, openAIErr.StatusCode)
}

// A V1-wire provider refuses Beta-only content before any request is sent,
// rather than forwarding a silently truncated request.
func TestUpstreamAnthropicV1RefusesBetaOnly(t *testing.T) {
	fake := newFakeProvider(t, scenario.TextScenario())
	endpoint, err := NewAnthropic(fake.config(protocol.APIStyleAnthropic), AnthropicWireV1)
	require.NoError(t, err)
	var req anthropic.BetaMessageNewParams
	require.NoError(t, json.Unmarshal([]byte(`{"model":"provider-model","max_tokens":64,
		"messages":[{"role":"user","content":"hi"}],
		"context_management":{"edits":[{"type":"clear_tool_uses_20250919"}]}}`), &req))

	_, err = endpoint.Complete(context.Background(), stage.Call{Request: &req})
	require.ErrorContains(t, err, "context_management")
	_, err = endpoint.Stream(context.Background(), stage.Call{Request: &req})
	require.ErrorContains(t, err, "context_management")
	require.Empty(t, fake.take())
}

func TestUpstreamRejectsForeignRequests(t *testing.T) {
	fake := newFakeProvider(t, scenario.TextScenario())
	anthropicEndpoint, err := NewAnthropic(fake.config(protocol.APIStyleAnthropic), AnthropicWireBeta)
	require.NoError(t, err)
	_, err = anthropicEndpoint.Complete(context.Background(), stage.Call{Request: &anthropic.MessageNewParams{}})
	require.ErrorContains(t, err, "want anthropic.BetaMessageNewParams")

	chat, err := NewOpenAIChat(fake.config(protocol.APIStyleOpenAI))
	require.NoError(t, err)
	_, err = chat.Stream(context.Background(), stage.Call{Request: &responses.ResponseNewParams{}})
	require.ErrorContains(t, err, "want openai.ChatCompletionNewParams")
	require.Empty(t, fake.take())

	_, err = NewOpenAIResponses(Config{})
	require.Error(t, err)
	_, err = NewAnthropic(fake.config(protocol.APIStyleAnthropic), AnthropicWire(9))
	require.Error(t, err)
}

func mode(streaming bool) string {
	if streaming {
		return "stream"
	}
	return "nonstream"
}

// Every P2 bridge runs on top of the real terminal endpoint for its target:
// the endpoint's native values are exactly what the bridges consume.
func TestUpstreamUnderBridges(t *testing.T) {
	fake := newFakeProvider(t, scenario.ToolUseScenario())
	anthropicConfig := fake.config(protocol.APIStyleAnthropic)
	openAIConfig := fake.config(protocol.APIStyleOpenAI)
	terminals := map[protocol.APIType]func() (stage.Endpoint, error){
		protocol.TypeAnthropicBeta:   func() (stage.Endpoint, error) { return NewAnthropic(anthropicConfig, AnthropicWireBeta) },
		protocol.TypeOpenAIChat:      func() (stage.Endpoint, error) { return NewOpenAIChat(openAIConfig) },
		protocol.TypeOpenAIResponses: func() (stage.Endpoint, error) { return NewOpenAIResponses(openAIConfig) },
	}
	requests := map[protocol.APIType]func() any{
		protocol.TypeAnthropicBeta: func() any {
			return &anthropic.BetaMessageNewParams{
				Model:     "client-model",
				MaxTokens: 64,
				Messages:  []anthropic.BetaMessageParam{anthropic.NewBetaUserMessage(anthropic.NewBetaTextBlock("weather?"))},
			}
		},
		protocol.TypeOpenAIChat: func() any {
			return &openai.ChatCompletionNewParams{
				Model:    "client-model",
				Messages: []openai.ChatCompletionMessageParamUnion{openai.UserMessage("weather?")},
			}
		},
		protocol.TypeOpenAIResponses: func() any {
			return &responses.ResponseNewParams{Model: "client-model", Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("weather?")}}
		},
	}
	bridges := []stage.Bridge{
		anthropicbridge.NewBetaToOpenAIChat(anthropicbridge.ChatOptions{}),
		anthropicbridge.NewBetaToOpenAIResponses(anthropicbridge.ResponsesOptions{}),
		openaibridge.NewChatToAnthropicBeta(openaibridge.AnthropicOptions{}),
		openaibridge.NewChatToOpenAIResponses(openaibridge.ResponsesOptions{}),
		responsesbridge.NewToAnthropicBeta(responsesbridge.AnthropicOptions{}),
		responsesbridge.NewToOpenAIChat(responsesbridge.ChatOptions{}),
	}
	for _, bridge := range bridges {
		t.Run(string(bridge.Source())+"/"+string(bridge.Target()), func(t *testing.T) {
			terminal, err := terminals[bridge.Target()]()
			require.NoError(t, err)
			endpoint, err := stage.Adapt(terminal, bridge)
			require.NoError(t, err)

			response, err := endpoint.Complete(context.Background(), stage.Call{Request: requests[bridge.Source()]()})
			require.NoError(t, err)
			require.Contains(t, mustJSON(t, response.Value), "get_weather")

			events, err := endpoint.Stream(context.Background(), stage.Call{Request: requests[bridge.Source()]()})
			require.NoError(t, err)
			var wire strings.Builder
			for _, event := range drain(t, events) {
				wire.WriteString(mustJSON(t, event.Value))
			}
			require.Contains(t, wire.String(), "get_weather")
			require.Len(t, fake.take(), 2)
		})
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	if raw, ok := value.(interface{ RawJSON() string }); ok && raw.RawJSON() != "" {
		return raw.RawJSON()
	}
	data, err := json.Marshal(value)
	require.NoError(t, err)
	return string(data)
}

// A stream-only provider (Codex) answers a complete call from its stream.
func TestUpstreamResponsesStreamOnly(t *testing.T) {
	fake := newFakeProvider(t, scenario.TextScenario())
	config := fake.config(protocol.APIStyleOpenAI)
	config.StreamOnly = true
	endpoint, err := NewOpenAIResponses(config)
	require.NoError(t, err)
	response, err := endpoint.Complete(context.Background(), stage.Call{Request: &responses.ResponseNewParams{
		Model: "provider-model", Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("hi")},
	}})
	require.NoError(t, err)
	final := response.Value.(*responses.Response)
	require.Equal(t, "The capital of France is Paris.", final.OutputText())
	require.True(t, response.Usage.HasUsage())
	requests := fake.take()
	require.Len(t, requests, 1)
	require.Contains(t, requests[0].Body, `"stream":true`, "the provider is only ever asked to stream")
}

// A stream-only backend (Codex) may end with an empty output and deliver the
// items only as output_item.done; an empty stream is an error (#1316).
func TestUpstreamResponsesStreamOnlyOutputItems(t *testing.T) {
	message := `{"type":"message","id":"msg_1","role":"assistant","status":"completed","content":[{"type":"output_text","text":"Paris","annotations":[]}]}`
	completed := `{"type":"response.completed","sequence_number":3,"response":{"id":"resp_1","object":"response","created_at":1,"model":"provider-model","status":"completed","output":[],"usage":{"input_tokens":3,"output_tokens":1,"total_tokens":4,"input_tokens_details":{"cached_tokens":0},"output_tokens_details":{"reasoning_tokens":0}}}}`
	cases := []struct {
		name   string
		events []string
		want   string
	}{
		{"items fill an empty output", []string{
			"event: response.output_item.done",
			`data: {"type":"response.output_item.done","sequence_number":2,"output_index":0,"item":` + message + `}`,
			"event: response.completed", "data: " + completed,
		}, "Paris"},
		{"no output is an error", []string{"event: response.completed", "data: " + completed}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := newFakeProvider(t, scenario.Scenario{MockResponses: map[scenario.ResponseFormat]scenario.MockResponseBuilder{
				scenario.FormatOpenAIResponses: {Stream: func() []string { return tc.events }},
			}})
			config := fake.config(protocol.APIStyleOpenAI)
			config.StreamOnly = true
			endpoint, err := NewOpenAIResponses(config)
			require.NoError(t, err)
			response, err := endpoint.Complete(context.Background(), stage.Call{Request: &responses.ResponseNewParams{
				Model: "provider-model", Input: responses.ResponseNewParamsInputUnion{OfString: openai.String("hi")},
			}})
			if tc.want == "" {
				require.ErrorContains(t, err, "no output")
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, response.Value.(*responses.Response).OutputText())
		})
	}
}
