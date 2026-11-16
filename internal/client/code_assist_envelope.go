package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// Helpers shared by clients that talk to the Google Code Assist host
// (https://cloudcode-pa.googleapis.com): Gemini CLI and Antigravity. The Code
// Assist API takes a wrapper envelope on requests and returns the inner
// response under a "response" key — these helpers handle that envelope.

// isStreamingRequest reports whether the URL targets a Google streaming
// generateContent operation.
func isStreamingRequest(req *http.Request) bool {
	return strings.Contains(req.URL.Path, ":streamGenerateContent")
}

// streamingUnwrapReader strips the Code Assist `"response"` wrapper from each
// SSE data line on the fly:
//
//	data: {"response": {...}}    ->    data: {...}
//
// Lines that don't match (comments, keep-alives, malformed JSON, JSON without
// the wrapper) pass through unchanged.
type streamingUnwrapReader struct {
	reader io.ReadCloser
	lines  *bufio.Reader
	buffer []byte
	err    error
}

func (r *streamingUnwrapReader) Read(p []byte) (n int, err error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(r.buffer) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		if r.lines == nil {
			r.lines = bufio.NewReader(r.reader)
		}
		// A network read can end anywhere inside JSON or a UTF-8 character.
		// Keep reading until a full SSE line arrives, without Scanner's size cap.
		line, readErr := r.lines.ReadBytes('\n')
		r.err = readErr
		r.buffer = line
		if bytes.HasPrefix(line, []byte("data:")) {
			payload := bytes.TrimSpace(line[5:])
			inner := unwrapCodeAssistJSON(payload)
			if !bytes.Equal(payload, inner) {
				ending := ""
				if bytes.HasSuffix(line, []byte("\r\n")) {
					ending = "\r\n"
				} else if bytes.HasSuffix(line, []byte("\n")) {
					ending = "\n"
				}
				r.buffer = append([]byte("data: "), inner...)
				r.buffer = append(r.buffer, ending...)
			}
		}
		if len(r.buffer) == 0 {
			return 0, r.err
		}
	}
	n = copy(p, r.buffer)
	r.buffer = r.buffer[n:]
	return n, nil
}

func (r *streamingUnwrapReader) Close() error {
	return r.reader.Close()
}

// unwrapCodeAssistJSON strips the `"response"` wrapper from a non-streaming
// Code Assist JSON body. Returns the raw bytes unchanged when parsing fails or
// no wrapper is present, so callers always get a body they can forward.
func unwrapCodeAssistJSON(body []byte) []byte {
	var wrapped map[string]json.RawMessage
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return body
	}
	inner, ok := wrapped["response"]
	if !ok {
		return body
	}
	return inner
}
