package client

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/iotest"
)

func TestStreamingUnwrapReader_FragmentedEvents(t *testing.T) {
	input := ": keepalive\r\nevent: message\r\ndata: {\"response\":{\"text\":\"你好\",\"id\":9007199254740993}}\r\n\r\ndata: [DONE]\n\ndata: invalid\n\ndata: {\"plain\":true}\n\ndata: {\"response\":{\"last\":true}}"
	want := ": keepalive\r\nevent: message\r\ndata: {\"text\":\"你好\",\"id\":9007199254740993}\r\n\r\ndata: [DONE]\n\ndata: invalid\n\ndata: {\"plain\":true}\n\ndata: {\"last\":true}"
	// Every possible network boundary, including within UTF-8 and JSON.
	for split := 0; split <= len(input); split++ {
		upstream := io.MultiReader(strings.NewReader(input[:split]), strings.NewReader(input[split:]))
		r := &streamingUnwrapReader{reader: io.NopCloser(upstream)}
		got, err := io.ReadAll(r)
		if err != nil || string(got) != want {
			t.Fatalf("split %d: got %q, err %v; want %q", split, got, err, want)
		}
	}
	// Also exercise an upstream and consumer both reading one byte at a time.
	r := &streamingUnwrapReader{reader: io.NopCloser(iotest.OneByteReader(strings.NewReader(input)))}
	var got bytes.Buffer
	p := make([]byte, 1)
	for {
		n, err := r.Read(p)
		got.Write(p[:n])
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if got.String() != want {
		t.Fatalf("one-byte reads: got %q, want %q", got.String(), want)
	}
}

func TestStreamingUnwrapReader_LargeLineAndTerminalError(t *testing.T) {
	text := strings.Repeat("x", 128*1024)
	input := "data: {\"response\":{\"text\":\"" + text + "\"}}\n\n"
	want := "data: {\"text\":\"" + text + "\"}\n\n"
	r := &streamingUnwrapReader{reader: io.NopCloser(iotest.DataErrReader(strings.NewReader(input)))}
	got, err := io.ReadAll(r)
	if err != nil || string(got) != want {
		t.Fatalf("large event: bytes=%d, err=%v", len(got), err)
	}
	terminal := errors.New("upstream disconnected")
	r = &streamingUnwrapReader{reader: io.NopCloser(io.MultiReader(
		strings.NewReader(`data: {"response":{"text":"partial"}}`), iotest.ErrReader(terminal),
	))}
	got, err = io.ReadAll(r)
	if !errors.Is(err, terminal) || string(got) != `data: {"text":"partial"}` {
		t.Fatalf("terminal error: got %q, err=%v", got, err)
	}
}

func TestUnwrapCodeAssistJSON_PreservesNumbers(t *testing.T) {
	input := []byte(`{"response":{"id":9007199254740993,"value":1.234567890123456789}}`)
	want := `{"id":9007199254740993,"value":1.234567890123456789}`
	if got := string(unwrapCodeAssistJSON(input)); got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
	for _, input := range []string{"", "null", "[]", "data", `{"error":{"code":429}}`} {
		if got := string(unwrapCodeAssistJSON([]byte(input))); got != input {
			t.Fatalf("passthrough: got %q, want %q", got, input)
		}
	}
}
