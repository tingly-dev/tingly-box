package tracestore

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tingly-dev/tingly-box/internal/recording/capture"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// memBlocks is a blockSink that records every distinct block.
type memBlocks map[string]json.RawMessage

func (m memBlocks) put(data []byte) string {
	h := hashOf(data)
	m[h] = append(json.RawMessage(nil), data...)
	return h
}

func compact(t *testing.T, b []byte) string {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, b); err != nil {
		t.Fatalf("compact %q: %v", b, err)
	}
	return buf.String()
}

func TestBodyRoundTrip(t *testing.T) {
	long := strings.Repeat("x", 400)
	cases := map[string]string{
		"object with chained arrays": `{"model":"m","system":[{"type":"text","text":"` + long + `"}],
			"messages":[{"role":"user","content":"` + long + `"},{"role":"assistant","content":"hi"}],"stream":true}`,
		"large scalar member":  `{"model":"m","prompt":"` + long + `"}`,
		"small object":         `{"a":1,"b":[1,2,3]}`,
		"empty array":          `{"messages":[]}`,
		"key order and escape": `{"z":1,"a":"<tag> & \"q\"","é":2}`,
		"top-level array":      `[` + strings.Repeat(`{"k":"`+long+`"},`, 2) + `{}]`,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			blocks := memBlocks{}
			got, err := decodeBody(encodeBody([]byte(in), blocks), blocks)
			if err != nil {
				t.Fatal(err)
			}
			if compact(t, got) != compact(t, []byte(in)) {
				t.Fatalf("round trip mismatch:\n got %s\nwant %s", got, compact(t, []byte(in)))
			}
		})
	}

	sse := "event: a\ndata: {}\n\n"
	blocks := memBlocks{}
	if got, _ := decodeBody(encodeBody([]byte(sse), blocks), blocks); string(got) != sse {
		t.Fatalf("sse round trip: %q", got)
	}
	bin := []byte{0xff, 0x00, 0xfe}
	if got, _ := decodeBody(encodeBody(bin, blocks), blocks); !bytes.Equal(got, bin) {
		t.Fatalf("binary round trip: %v", got)
	}
}

// conversation builds the request body of turn n of a long agent session:
// a fixed tool list, then n user/assistant pairs, with the prompt-cache
// breakpoint on the last message as Claude Code places it.
func conversation(n int) []byte {
	tools := make([]string, 20)
	for i := range tools {
		tools[i] = fmt.Sprintf(`{"name":"tool_%d","description":"%s","input_schema":{"type":"object"}}`, i, strings.Repeat("d", 500))
	}
	msgs := make([]string, 0, 2*n)
	for i := 0; i < n; i++ {
		msgs = append(msgs,
			fmt.Sprintf(`{"role":"user","content":[{"type":"tool_result","tool_use_id":"t%d","content":"%s"}]}`, i, strings.Repeat("r", 1500)),
			fmt.Sprintf(`{"role":"assistant","content":[{"type":"text","text":"step %d %s"}]}`, i, strings.Repeat("a", 500)))
	}
	last := msgs[len(msgs)-1]
	msgs[len(msgs)-1] = last[:len(last)-1] + `,"cache_control":{"type":"ephemeral"}}`
	return []byte(`{"model":"claude","max_tokens":4096,"stream":true,"tools":[` + strings.Join(tools, ",") +
		`],"messages":[` + strings.Join(msgs, ",") + `]}`)
}

func snapshot(rid string, inbound, upstream []byte) *capture.Snapshot {
	return &capture.Snapshot{
		RequestID: rid,
		Timestamp: time.Date(2026, 10, 6, 12, 0, 0, 0, time.UTC),
		Scenario:  "claude_code:p1",
		Rule:      "rule-1",
		Session:   typ.SessionID{Value: "session-a", Source: "hdr"},
		Mode:      "client_request,upstream_request,upstream_response",
		Inbound:   &capture.Message{Method: "POST", URL: "/v1/messages", Body: inbound, Complete: true},
		Exchanges: []*capture.ExchangeSnapshot{{
			Seq:      1,
			Provider: capture.ProviderInfo{Name: "anthropic", APIStyle: "anthropic"},
			Request:  &capture.Message{Method: "POST", URL: "https://api/v1/messages", Body: upstream, Complete: true},
			Response: &capture.Message{Status: 200, Stream: true, ContentType: "text/event-stream",
				Body: []byte("event: message_stop\ndata: {}\n\n"), Complete: true},
		}},
	}
}

func TestLongSessionDedup(t *testing.T) {
	const turns = 100
	root := t.TempDir()
	w := NewWriter(root)
	ctx := context.Background()

	var naive int
	for n := 1; n <= turns; n++ {
		body := conversation(n)
		// The gateway changed only "model" on the way out.
		upstream := bytes.Replace(body, []byte(`"model":"claude"`), []byte(`"model":"claude-x"`), 1)
		naive += len(body) + len(upstream)
		w.Emit(snapshot(fmt.Sprintf("r%d", n), body, upstream))
		if n%10 == 0 {
			if err := w.Flush(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(ctx); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(root, "claude_code-p1", "2026-10-06")
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	var data string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl.gz") {
			data = filepath.Join(path, e.Name())
		}
	}
	traces, err := ReadPartition(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(traces) != turns {
		t.Fatalf("traces = %d, want %d", len(traces), turns)
	}
	for i, tr := range traces {
		want := conversation(i + 1)
		if compact(t, tr.Inbound.Body) != compact(t, want) {
			t.Fatalf("turn %d inbound body not restored", i+1)
		}
		if !bytes.Contains(tr.Exchanges[0].Request.Body, []byte(`"model":"claude-x"`)) {
			t.Fatalf("turn %d upstream body not restored", i+1)
		}
		if got := string(tr.Exchanges[0].Response.Body); got != "event: message_stop\ndata: {}\n\n" {
			t.Fatalf("turn %d sse body = %q", i+1, got)
		}
	}

	// Raw (pre-gzip) bytes stored: the final context plus a small per-turn
	// increment, instead of the quadratic sum of every request.
	stored := rawSize(t, data)
	final := len(conversation(turns))
	t.Logf("naive=%d KB stored=%d KB final-context=%d KB", naive/1024, stored/1024, final/1024)
	if stored > 3*final {
		t.Fatalf("stored %d bytes for a %d byte final context: dedup not effective", stored, final)
	}

	index, err := ReadIndex(strings.TrimSuffix(data, ".jsonl.gz") + ".index.jsonl")
	if err != nil || len(index) != turns {
		t.Fatalf("index entries = %d (err %v), want %d", len(index), err, turns)
	}
	if e := index[0]; e.RequestID != "r1" || e.Rule != "rule-1" || e.Provider != "anthropic" || e.Status != 200 || e.Exchanges != 1 {
		t.Fatalf("index entry: %+v", e)
	}
}

func rawSize(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var n int
	buf := make([]byte, 32<<10)
	gz := mustGzip(t, f)
	for {
		k, err := gz.Read(buf)
		n += k
		if err != nil {
			break
		}
	}
	return n
}

func TestRestartReStoresBlocksOnce(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	body := conversation(5)

	w1 := NewWriter(root)
	w1.Emit(snapshot("a", body, body))
	_ = w1.Close(ctx)
	w2 := NewWriter(root) // fresh dedup set, same partition file
	w2.Emit(snapshot("b", body, body))
	_ = w2.Close(ctx)

	matches, _ := filepath.Glob(filepath.Join(root, "*", "*", "*.jsonl.gz"))
	if len(matches) != 1 {
		t.Fatalf("partitions = %v", matches)
	}
	traces, err := ReadPartition(matches[0])
	if err != nil || len(traces) != 2 {
		t.Fatalf("traces = %d err = %v", len(traces), err)
	}
	for _, tr := range traces {
		if compact(t, tr.Inbound.Body) != compact(t, body) {
			t.Fatalf("trace %s not restored", tr.RequestID)
		}
	}
}

func TestEmitAfterCloseIsDropped(t *testing.T) {
	w := NewWriter(t.TempDir())
	_ = w.Close(context.Background())
	w.Emit(snapshot("x", []byte(`{}`), []byte(`{}`)))
	var nilWriter *Writer
	nilWriter.Emit(nil)
	if err := nilWriter.Flush(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func mustGzip(t *testing.T, r io.Reader) *gzip.Reader {
	t.Helper()
	gz, err := gzip.NewReader(r)
	if err != nil {
		t.Fatal(err)
	}
	gz.Multistream(true)
	return gz
}
