package client

import (
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
	buffer []byte
	err    error
}

func (r *streamingUnwrapReader) Read(p []byte) (n int, err error) {
	if len(r.buffer) > 0 {
		n = copy(p, r.buffer)
		r.buffer = r.buffer[n:]
		return n, nil
	}

	if r.err != nil {
		return 0, r.err
	}

	buf := make([]byte, 4096)
	var lineBuffer bytes.Buffer

	for {
		nn, readErr := r.reader.Read(buf)
		if nn > 0 {
			lineBuffer.Write(buf[:nn])
		}
		if readErr != nil {
			if readErr == io.EOF {
				if lineBuffer.Len() > 0 {
					r.buffer = r.processBuffer(lineBuffer.Bytes())
					n = copy(p, r.buffer)
					r.buffer = r.buffer[n:]
					if len(r.buffer) == 0 {
						r.err = io.EOF
					}
					return n, nil
				}
				return 0, io.EOF
			}
			r.err = readErr
			return 0, readErr
		}

		processed := r.processBuffer(lineBuffer.Bytes())
		if len(processed) > 0 {
			n = copy(p, processed)
			r.buffer = processed[n:]
			return n, nil
		}

		if lineBuffer.Len() > 40960 {
			n = copy(p, lineBuffer.Bytes())
			lineBuffer.Next(n)
			return n, nil
		}
	}
}

func (r *streamingUnwrapReader) processBuffer(data []byte) []byte {
	lines := bytes.Split(data, []byte("\n"))
	var result bytes.Buffer

	for i, line := range lines {
		if bytes.HasPrefix(line, []byte("data:")) {
			jsonData := bytes.TrimSpace(line[5:])
			if len(jsonData) == 0 {
				result.Write(line)
			} else {
				var wrapped map[string]any
				if err := json.Unmarshal(jsonData, &wrapped); err == nil {
					if innerResponse, ok := wrapped["response"]; ok {
						unwrapped, err := json.Marshal(innerResponse)
						if err == nil {
							result.Write([]byte("data: "))
							result.Write(unwrapped)
						} else {
							result.Write(line)
						}
					} else {
						result.Write(line)
					}
				} else {
					result.Write(line)
				}
			}
		} else {
			result.Write(line)
		}

		if i < len(lines)-1 || data[len(data)-1] == '\n' {
			result.WriteByte('\n')
		}
	}

	return result.Bytes()
}

func (r *streamingUnwrapReader) Close() error {
	return r.reader.Close()
}

// unwrapCodeAssistJSON strips the `"response"` wrapper from a non-streaming
// Code Assist JSON body. Returns the raw bytes unchanged when parsing fails or
// no wrapper is present, so callers always get a body they can forward.
func unwrapCodeAssistJSON(body []byte) []byte {
	var wrapped map[string]any
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return body
	}
	inner, ok := wrapped["response"]
	if !ok {
		return body
	}
	out, err := json.Marshal(inner)
	if err != nil {
		return body
	}
	return out
}
