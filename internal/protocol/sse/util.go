package sse

import (
	"bufio"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// ParseSSEDataPayload checks if a line is an SSE data line and extracts the payload.
// Handles both Gin-style ("data:{json}") and standard SSE ("data: {json}") formats.
func ParseSSEDataPayload(line string) (payload string, ok bool) {
	if strings.HasPrefix(line, "data: ") {
		return strings.TrimPrefix(line, "data: "), true
	}
	if strings.HasPrefix(line, "data:") {
		return strings.TrimPrefix(line, "data:"), true
	}
	return "", false
}

// ReadSSELines reads an SSE response body and returns each non-empty line as a string.
// Blank lines (SSE message separators) are skipped. The raw bytes of all non-empty
// lines (with newlines) are also returned for inspection.
func ReadSSELines(r io.Reader) (lines []string, raw []byte) {
	scanner := bufio.NewScanner(r)
	var sb strings.Builder
	for scanner.Scan() {
		line := scanner.Text()
		if line != "" {
			lines = append(lines, line)
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	return lines, []byte(sb.String())
}

// WriteSSEResponse writes pre-built SSE event strings to an HTTP response.
// Events that start with "event:" are grouped with the following "data:" line
// into a single SSE message (separated by a blank line), matching the SSE spec.
// A 5 ms flush interval is applied between messages.
func WriteSSEResponse(w http.ResponseWriter, events []string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	i := 0
	for i < len(events) {
		line := events[i]
		if strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "event: ") {
			// Write event: line + next data: line as one SSE message
			fmt.Fprintf(w, "%s\n", line)
			i++
			if i < len(events) && (strings.HasPrefix(events[i], "data:") || strings.HasPrefix(events[i], "data: ")) {
				fmt.Fprintf(w, "%s\n\n", events[i])
				i++
			} else {
				fmt.Fprintf(w, "\n")
			}
		} else {
			// Standalone data: line (e.g. "data: [DONE]")
			fmt.Fprintf(w, "%s\n\n", line)
			i++
		}
		flusher.Flush()
		time.Sleep(5 * time.Millisecond)
	}
}
