package capture

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tingly-dev/tingly-box/internal/typ"
)

var testProvider = ProviderInfo{Name: "p", UUID: "u-1", APIStyle: "anthropic"}

func newServer(t *testing.T, h http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func enabledTrace(mode typ.RecordingMode) *Trace {
	tr := New()
	tr.Enable(EnableOptions{Mode: mode, Scenario: "claude_code", Rule: "r-1"})
	return tr
}

func do(t *testing.T, tr *Trace, url, body string, readAll bool) {
	t.Helper()
	client := &http.Client{Transport: WrapTransport(http.DefaultTransport, testProvider)}
	req, err := http.NewRequestWithContext(WithTrace(context.Background(), tr), http.MethodPost, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-0123456789abcdef")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if readAll {
		_, _ = io.ReadAll(resp.Body)
	} else {
		buf := make([]byte, 4)
		_, _ = resp.Body.Read(buf)
	}
	_ = resp.Body.Close()
}

func TestDisabledTraceRecordsNothing(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, `{}`) })
	tr := New()
	tr.SetInboundRequest("POST", "/v1/messages", nil, []byte(`{"a":1}`))
	do(t, tr, srv.URL, `{"x":1}`, true)
	if s := tr.Finish("rid"); s != nil {
		t.Fatalf("disabled trace produced a snapshot: %+v", s)
	}
	// A nil Trace in context is also a plain pass-through.
	do(t, nil, srv.URL, `{"x":1}`, true)
}

func TestNonStreamExchange(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"echo":%s}`, body)
	})
	tr := enabledTrace("client_request,upstream_request,upstream_response")
	tr.SetInboundRequest("POST", "/tingly/claude_code/v1/messages", map[string][]string{"Authorization": {"Bearer secret-token-value"}}, []byte(`{"in":1}`))
	do(t, tr, srv.URL, `{"out":1}`, true)

	s := tr.Finish("rid-1")
	if s == nil || s.RequestID != "rid-1" || s.Scenario != "claude_code" || s.Rule != "r-1" {
		t.Fatalf("bad snapshot meta: %+v", s)
	}
	if s.Inbound == nil || string(s.Inbound.Body) != `{"in":1}` {
		t.Fatalf("inbound body: %+v", s.Inbound)
	}
	if got := s.Inbound.Headers["Authorization"]; got == "Bearer secret-token-value" || !strings.Contains(got, "***") {
		t.Fatalf("inbound authorization not redacted: %q", got)
	}
	if len(s.Exchanges) != 1 {
		t.Fatalf("exchanges = %d, want 1", len(s.Exchanges))
	}
	ex := s.Exchanges[0]
	if ex.Seq != 1 || ex.Provider != testProvider {
		t.Fatalf("exchange meta: %+v", ex)
	}
	if string(ex.Request.Body) != `{"out":1}` {
		t.Fatalf("upstream request body = %q", ex.Request.Body)
	}
	if got := ex.Request.Headers["X-Api-Key"]; !strings.Contains(got, "***") {
		t.Fatalf("upstream api key not redacted: %q", got)
	}
	if ex.Response.Status != 200 || ex.Response.Stream || !ex.Response.Complete {
		t.Fatalf("response meta: %+v", ex.Response)
	}
	if string(ex.Response.Body) != `{"echo":{"out":1}}` {
		t.Fatalf("upstream response body = %q", ex.Response.Body)
	}
}

func TestStreamExchangeCompleteAndIncomplete(t *testing.T) {
	const sse = "event: message_start\ndata: {\"a\":1}\n\nevent: message_stop\ndata: {}\n\n"
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, sse)
	})

	tr := enabledTrace("upstream_response")
	do(t, tr, srv.URL, `{"stream":true}`, true)
	do(t, tr, srv.URL, `{"stream":true}`, false) // closed after 4 bytes
	s := tr.Finish("rid")
	if len(s.Exchanges) != 2 {
		t.Fatalf("exchanges = %d, want 2", len(s.Exchanges))
	}
	full, cut := s.Exchanges[0].Response, s.Exchanges[1].Response
	if !full.Stream || !full.Complete || string(full.Body) != sse {
		t.Fatalf("full stream: stream=%v complete=%v body=%q", full.Stream, full.Complete, full.Body)
	}
	if !cut.Stream || cut.Complete {
		t.Fatalf("closed-early stream should be incomplete: %+v", cut)
	}
	// upstream_request was not selected: metadata only.
	if s.Exchanges[0].Request == nil || s.Exchanges[0].Request.Body != nil {
		t.Fatalf("request body captured without upstream_request: %+v", s.Exchanges[0].Request)
	}
	if s.Inbound != nil {
		t.Fatalf("inbound captured without client_request")
	}
}

func TestErrorStatusIsNonStream(t *testing.T) {
	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":"rate"}`)
	})
	tr := enabledTrace("upstream_response")
	do(t, tr, srv.URL, `{"stream":true}`, true)
	r := tr.Finish("rid").Exchanges[0].Response
	if r.Status != 429 || r.Stream || string(r.Body) != `{"error":"rate"}` {
		t.Fatalf("error response: %+v body=%q", r, r.Body)
	}
}

func TestTruncation(t *testing.T) {
	old := maxBodyBytes
	maxBodyBytes = 8
	t.Cleanup(func() { maxBodyBytes = old })

	srv := newServer(t, func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, "0123456789abcdef") })
	tr := enabledTrace("upstream_request,upstream_response")
	do(t, tr, srv.URL, "0123456789", true)
	ex := tr.Finish("rid").Exchanges[0]
	if !ex.Request.Truncated || len(ex.Request.Body) != 8 || ex.Request.Size != 10 {
		t.Fatalf("request truncation: %+v", ex.Request)
	}
	if !ex.Response.Truncated || len(ex.Response.Body) != 8 || ex.Response.Size != 16 {
		t.Fatalf("response truncation: %+v", ex.Response)
	}
}

func TestFinishFreezes(t *testing.T) {
	tr := enabledTrace("upstream_request")
	if tr.Finish("a") == nil {
		t.Fatal("first Finish should snapshot")
	}
	if tr.Finish("b") != nil || tr.Enabled() {
		t.Fatal("Trace must be frozen after Finish")
	}
	// Enable after Finish is a no-op; a disabled mode never enables.
	tr2 := New()
	tr2.Enable(EnableOptions{Mode: ""})
	if tr2.Enabled() {
		t.Fatal("empty mode must not enable")
	}
}
