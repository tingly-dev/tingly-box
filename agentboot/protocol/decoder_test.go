package protocol

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"
)

func TestDecoder_GoldenAssistantThenResult(t *testing.T) {
	f, err := os.Open("testdata/assistant_then_result.jsonl")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	d := NewDecoder(f)
	ch, errFn := d.Stream(context.Background())

	var types []string
	for ev := range ch {
		types = append(types, ev.Type)
	}
	if errFn() != nil {
		t.Fatalf("Err() = %v, want nil", errFn())
	}
	want := []string{"system", "assistant", "result"}
	if len(types) != len(want) {
		t.Fatalf("got %d events, want %d (%v)", len(types), len(want), types)
	}
	for i := range want {
		if types[i] != want[i] {
			t.Fatalf("event[%d].Type = %q, want %q", i, types[i], want[i])
		}
	}
}

func TestDecoder_PermissionRequest(t *testing.T) {
	f, err := os.Open("testdata/permission_request.jsonl")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	d := NewDecoder(f)
	ch, errFn := d.Stream(context.Background())

	ev, ok := <-ch
	if !ok {
		t.Fatalf("channel closed before first event")
	}
	if ev.Type != "control_request" {
		t.Fatalf("Type = %q, want control_request", ev.Type)
	}
	req, ok := ev.Data["request"].(map[string]any)
	if !ok {
		t.Fatalf("request field missing")
	}
	if req["subtype"] != "can_use_tool" {
		t.Fatalf("subtype = %v", req["subtype"])
	}

	for range ch {
		// drain
	}
	if errFn() != nil {
		t.Fatalf("Err() = %v, want nil", errFn())
	}
}

func TestDecoder_EmptyInputClosesChannel(t *testing.T) {
	d := NewDecoder(strings.NewReader(""))
	ch, errFn := d.Stream(context.Background())
	if _, ok := <-ch; ok {
		t.Fatalf("expected immediate close on empty input")
	}
	if errFn() != nil {
		t.Fatalf("Err() = %v, want nil", errFn())
	}
}

func TestDecoder_MalformedInputReportsError(t *testing.T) {
	d := NewDecoder(strings.NewReader(`{"type":"ok"} not-json`))
	ch, errFn := d.Stream(context.Background())

	first, ok := <-ch
	if !ok || first.Type != "ok" {
		t.Fatalf("first event = %+v, ok=%v", first, ok)
	}
	for range ch {
		// drain until closure
	}
	if errFn() == nil {
		t.Fatalf("Err() = nil, want decode error")
	}
}

func TestDecoder_CtxCancelStopsStream(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	d := NewDecoder(pr)
	ch, errFn := d.Stream(ctx)

	// Push one valid event so the goroutine is past its first select.
	go func() { _, _ = pw.Write([]byte(`{"type":"x"}` + "\n")) }()
	if _, ok := <-ch; !ok {
		t.Fatalf("channel closed before first event")
	}

	cancel()

	select {
	case _, ok := <-ch:
		if ok {
			// drain remaining
			for range ch {
			}
		}
	case <-time.After(time.Second):
		t.Fatalf("channel did not close after ctx cancel")
	}
	if !errors.Is(errFn(), context.Canceled) {
		t.Fatalf("Err() = %v, want context.Canceled", errFn())
	}
}

func TestEncoder_WritesNewlineDelimited(t *testing.T) {
	var buf bytes.Buffer
	e := NewEncoder(&buf)
	if err := e.Encode(map[string]any{"a": 1}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	if err := e.Encode(map[string]any{"b": 2}); err != nil {
		t.Fatalf("encode: %v", err)
	}
	got := buf.String()
	want := `{"a":1}` + "\n" + `{"b":2}` + "\n"
	if got != want {
		t.Fatalf("encoder output = %q, want %q", got, want)
	}
}

func TestEncoder_CloseIsIdempotentAndRejectsFurtherWrites(t *testing.T) {
	dst := &closeBuffer{}
	encoder := NewEncoder(dst)
	if err := encoder.Encode(map[string]any{"a": 1}); err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if dst.closeCalls != 1 {
		t.Fatalf("close calls = %d, want 1", dst.closeCalls)
	}
	if err := encoder.Encode(map[string]any{"b": 2}); !errors.Is(err, ErrEncoderClosed) {
		t.Fatalf("Encode after Close error = %v, want ErrEncoderClosed", err)
	}
}

type closeBuffer struct {
	bytes.Buffer
	closeCalls int
}

func (b *closeBuffer) Close() error {
	b.closeCalls++
	return nil
}
