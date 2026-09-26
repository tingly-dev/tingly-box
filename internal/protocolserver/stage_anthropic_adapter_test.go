package protocolserver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tingly-dev/tingly-box/internal/client"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/request"
	"github.com/tingly-dev/tingly-box/internal/protocol/sse"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/toolround"
	"github.com/tingly-dev/tingly-box/internal/protocol/stage/upstream"
	"github.com/tingly-dev/tingly-box/internal/typ"
	"github.com/tingly-dev/tingly-box/vmodel/benchmark/scenario"
)

// The Anthropic HTTP adapter is checked against the golden wire snapshots of
// the legacy gateway (internal/protocoltest/testdata/golden/wire): the same
// scenarios, served by a provider with the same fixtures, must reach the
// client as the same bytes. The pipeline is the real upstream endpoint under
// a Tool Round Stage with nothing to do, i.e. what a Beta→Beta route becomes.

// goldenCase is one "=== scenario stream=bool" section of a golden file.
type goldenCase struct {
	upstream string
	status   int
	body     string
}

func readGolden(t *testing.T, file string) map[string]goldenCase {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "protocoltest", "testdata", "golden", "wire", file))
	require.NoError(t, err)
	cases := map[string]goldenCase{}
	header := regexp.MustCompile(`(?m)^=== (.+)\n--- upstream request\n`)
	locs := header.FindAllStringSubmatchIndex(string(raw), -1)
	for i, loc := range locs {
		end := len(raw)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		name := string(raw[loc[2]:loc[3]])
		section := string(raw[loc[1]:end])
		split := regexp.MustCompile(`\n--- client response \(status (\d+)\)\n`).FindStringSubmatchIndex(section)
		require.NotNil(t, split, "malformed golden case %s", name)
		var status int
		fmt.Sscanf(section[split[2]:split[3]], "%d", &status)
		cases[name] = goldenCase{
			upstream: section[:split[0]],
			status:   status,
			body:     strings.TrimRight(section[split[1]:], "\n"),
		}
	}
	require.NotEmpty(t, cases)
	return cases
}

// normalizeGoldenBody applies the golden test's normalization (see
// protocoltest/golden_test.go) to one client body.
func normalizeGoldenBody(s string) string {
	ids := map[string]string{}
	s = regexp.MustCompile(`"(id|item_id|call_id|tool_use_id|response_id|previous_response_id|message_id)"\s*:\s*"([^"]*)"`).
		ReplaceAllStringFunc(s, func(m string) string {
			sub := regexp.MustCompile(`"([a-z_]+)"\s*:\s*"([^"]*)"`).FindStringSubmatch(m)
			key, val := sub[1], sub[2]
			if val == "" {
				return m
			}
			ph, ok := ids[val]
			if !ok {
				ph = fmt.Sprintf("<ID%d>", len(ids)+1)
				ids[val] = ph
			}
			return fmt.Sprintf(`"%s":"%s"`, key, ph)
		})
	s = regexp.MustCompile(`"(obfuscation)"\s*:\s*"[^"]*"`).ReplaceAllString(s, `"$1":"<RANDOM>"`)
	return regexp.MustCompile(`"(created|created_at|completed_at)"\s*:\s*[0-9]+`).ReplaceAllString(s, `"$1":<TS>`)
}

// scenarioProvider serves a scenario's Anthropic fixtures the way the
// harness's virtual server does and records the last request.
type scenarioProvider struct {
	*httptest.Server
	mu   sync.Mutex
	last *http.Request
	body []byte
}

func newScenarioProvider(t *testing.T, s scenario.Scenario) *scenarioProvider {
	t.Helper()
	p := &scenarioProvider{}
	mock := s.MockResponses[scenario.FormatAnthropic]
	p.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		p.mu.Lock()
		p.last, p.body = r, body
		p.mu.Unlock()
		streaming := strings.Contains(string(body), `"stream":true`)
		if streaming && mock.StreamHTTPError < 400 {
			var lines []string
			if mock.StreamFor != nil {
				lines = mock.StreamFor(body)
			} else {
				lines = mock.Stream()
			}
			sse.WriteSSEResponse(w, lines)
			return
		}
		var status int
		var payload []byte
		if mock.NonStreamFor != nil {
			status, payload = mock.NonStreamFor(body)
		} else {
			status, payload = mock.NonStream()
		}
		if streaming {
			status = mock.StreamHTTPError
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write(payload)
	}))
	t.Cleanup(p.Close)
	return p
}

func (p *scenarioProvider) upstreamRequest() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.last == nil {
		return "(no upstream request)"
	}
	var v any
	if err := json.Unmarshal(p.body, &v); err != nil {
		return p.last.Method + " " + p.last.URL.Path + "\n" + string(p.body)
	}
	pretty, _ := json.MarshalIndent(v, "", "  ")
	return p.last.Method + " " + p.last.URL.Path + "\n" + string(pretty)
}

// runStageAnthropic serves one client request through the adapter and
// returns the upstream request, status and body as the golden test records
// them.
func runStageAnthropic(t *testing.T, s scenario.Scenario, clientProtocol protocol.APIType, streaming bool) (string, int, string) {
	t.Helper()
	provider := newScenarioProvider(t, s)
	wire := upstream.AnthropicWireBeta
	if clientProtocol == protocol.TypeAnthropicV1 {
		wire = upstream.AnthropicWireV1
	}
	terminal, err := upstream.NewAnthropic(upstream.Config{
		Clients: client.NewClientPool(),
		Provider: &typ.Provider{UUID: "golden", Name: "golden", APIBase: provider.URL, APIStyle: protocol.APIStyleAnthropic,
			Token: "virtual-token", Enabled: true, Timeout: 30},
		Model: "virtual-model-" + s.Name,
	}, wire)
	require.NoError(t, err)
	endpoint, err := stage.Compose(terminal, toolround.New(toolround.Config{}))
	require.NoError(t, err)

	// The request as the client sends it, with the routed model.
	body := fmt.Sprintf(`{"model":"virtual-model-%s","max_tokens":1024,"messages":[{"role":"user","content":[{"type":"text","text":"What is the capital of France?"}]}]}`, s.Name)
	if clientProtocol == protocol.TypeAnthropicV1 {
		body = fmt.Sprintf(`{"model":"virtual-model-%s","max_tokens":1024,"messages":[{"role":"user","content":"What is the capital of France?"}]}`, s.Name)
	}
	var beta anthropic.BetaMessageNewParams
	if clientProtocol == protocol.TypeAnthropicV1 {
		var v1 anthropic.MessageNewParams
		require.NoError(t, json.Unmarshal([]byte(body), &v1))
		upgraded, err := request.ConvertAnthropicV1ToBetaRequestWithError(&v1)
		require.NoError(t, err)
		beta = *upgraded
	} else {
		require.NoError(t, json.Unmarshal([]byte(body), &beta))
	}

	gin.SetMode(gin.TestMode)
	recorder := &closeNotifyRecorder{httptest.NewRecorder()}
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/tingly/anthropic/v1/messages", nil)
	ph := &ProtocolHandler{}
	ph.ServeStageAnthropic(c, endpoint, StageAnthropicAttempt{
		Client:        clientProtocol,
		Request:       &beta,
		Provider:      &typ.Provider{Name: "golden"},
		ActualModel:   "virtual-model-" + s.Name,
		ResponseModel: fmt.Sprintf("pv-%s-to-anthropic_beta-%s", clientProtocol, s.Name),
		Streaming:     streaming,
	})
	return provider.upstreamRequest(), recorder.Code, strings.TrimRight(recorder.Body.String(), "\n")
}

func TestStageAnthropicAdapterMatchesGoldenBeta(t *testing.T) {
	golden := readGolden(t, "anthropic_beta__anthropic_beta.txt")
	for _, s := range scenario.AllScenarios() {
		for _, streaming := range []bool{false, true} {
			s, streaming := s, streaming
			name := fmt.Sprintf("%s stream=%v", s.Name, streaming)
			t.Run(name, func(t *testing.T) {
				want, ok := golden[name]
				require.True(t, ok, "golden case missing")
				upstreamRequest, status, body := runStageAnthropic(t, s, protocol.TypeAnthropicBeta, streaming)
				require.Equal(t, want.status, status)
				require.Equal(t, want.body, normalizeGoldenBody(body), "client bytes differ from the legacy gateway")
				require.Equal(t, strings.TrimRight(want.upstream, "\n"), normalizeGoldenBody(upstreamRequest), "provider request differs from the legacy gateway")
			})
		}
	}
}

func TestStageAnthropicAdapterMatchesGoldenV1(t *testing.T) {
	golden := readGolden(t, "anthropic_v1__anthropic_beta.txt")
	for _, s := range scenario.AllScenarios() {
		for _, streaming := range []bool{false, true} {
			s, streaming := s, streaming
			name := fmt.Sprintf("%s stream=%v", s.Name, streaming)
			t.Run(name, func(t *testing.T) {
				want, ok := golden[name]
				require.True(t, ok, "golden case missing")
				upstreamRequest, status, body := runStageAnthropic(t, s, protocol.TypeAnthropicV1, streaming)
				want.body = v1Unified(want.body, streaming)
				require.Equal(t, want.status, status)
				require.Equal(t, strings.TrimRight(want.upstream, "\n"), normalizeGoldenBody(upstreamRequest), "provider request differs from the legacy gateway")
				if !streaming || status != http.StatusOK {
					require.Equal(t, want.body, normalizeGoldenBody(body), "client bytes differ from the legacy gateway")
					return
				}
				// V1 streams are written by the Beta passthrough writer family
				// (event:X / data:{...}); the legacy V1 path used the MCP
				// interceptor's spaced framing. Same events, same payloads.
				require.Equal(t, sseEvents(t, want.body), sseEvents(t, normalizeGoldenBody(body)))
			})
		}
	}
}

// v1Unified applies the intended differences for V1 clients, which now share
// the Beta writers: the legacy V1 path reported a failed non-stream forward
// as a "streaming request" failure, and ended a truncated stream with its own
// error code. Status codes and every other byte are unchanged.
func v1Unified(body string, streaming bool) string {
	if !streaming {
		return strings.Replace(body, `"message":"Failed to create streaming request: `, `"message":"Failed to forward request: `, 1)
	}
	return strings.Replace(body,
		`{"error":{"code":"upstream_truncated","message":"upstream stream truncated: anthropic stream ended without message_stop","type":"stream_error"},"type":"error"}`,
		`{"type":"error","error":{"message":"upstream stream ended before completion","type":"stream_error","code":"incomplete_stream"}}`, 1)
}

// sseEvents parses an SSE body into (event, canonical JSON data) pairs.
func sseEvents(t *testing.T, body string) []string {
	t.Helper()
	var out []string
	event := ""
	for _, line := range strings.Split(body, "\n") {
		switch {
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
			var v any
			if json.Unmarshal([]byte(data), &v) == nil {
				canonical, _ := json.Marshal(v)
				data = string(canonical)
			}
			out = append(out, event+" "+data)
			event = ""
		}
	}
	return out
}

// closeNotifyRecorder adds the CloseNotifier the stream loop expects of a
// real connection.
type closeNotifyRecorder struct{ *httptest.ResponseRecorder }

func (*closeNotifyRecorder) CloseNotify() <-chan bool { return make(chan bool) }

// failingEndpoint fails every call with err.
type failingEndpoint struct{ err error }

func (failingEndpoint) Protocol() protocol.APIType { return protocol.TypeAnthropicBeta }
func (e failingEndpoint) Complete(context.Context, stage.Call) (*stage.Response, error) {
	return nil, e.err
}
func (e failingEndpoint) Stream(context.Context, stage.Call) (stage.EventStream, error) {
	return &erroringStream{err: e.err}, nil
}

type erroringStream struct{ err error }

func (s *erroringStream) Next(context.Context) (stage.Event, error) { return stage.Event{}, s.err }
func (s *erroringStream) Close() error                              { return nil }
func (s *erroringStream) Result() stage.StreamResult                { return stage.StreamResult{} }

// An upstream failure after a server tool ran must not fail over: another
// service would run the tool again. Before any side effect it may.
func TestStageAnthropicAdapterHoldsFailoverAfterSideEffects(t *testing.T) {
	upstreamReq := httptest.NewRequest(http.MethodPost, "http://provider/v1/messages", nil)
	upstreamErr := &anthropic.Error{
		StatusCode: http.StatusServiceUnavailable,
		Request:    upstreamReq,
		Response:   &http.Response{StatusCode: http.StatusServiceUnavailable, Status: "503 Service Unavailable", Request: upstreamReq},
	}
	for _, streaming := range []bool{false, true} {
		for _, committed := range []bool{false, true} {
			t.Run(fmt.Sprintf("stream=%v/committed=%v", streaming, committed), func(t *testing.T) {
				gin.SetMode(gin.TestMode)
				recorder := &closeNotifyRecorder{httptest.NewRecorder()}
				c, _ := gin.CreateTestContext(recorder)
				c.Request = httptest.NewRequest(http.MethodPost, "/tingly/anthropic/v1/messages", nil)
				gate := newFirstChunkGate(c.Writer)
				c.Writer = gate

				ph := &ProtocolHandler{}
				ph.ServeStageAnthropic(c, failingEndpoint{err: stage.WrapCommitted(upstreamErr, committed)}, StageAnthropicAttempt{
					Client: protocol.TypeAnthropicBeta, Request: &anthropic.BetaMessageNewParams{},
					Provider: &typ.Provider{Name: "p"}, ResponseModel: "m", Streaming: streaming,
				})
				require.Equal(t, http.StatusServiceUnavailable, gate.Status())
				require.Equal(t, committed, gate.Committed(), "a committed error holds failover; an uncommitted one leaves it retryable")
			})
		}
	}
}
