package capture

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// answerAnthropic serves POST …/v1/messages for a model id nobody defined,
// echoing the requested id and mirroring the real API's wire shape: SSE with
// ping, `content: []`, null stop_reason in message_start and "end_turn" at the
// end; or the equivalent single JSON message. It reports done=false (with the
// body restored on the returned request) when the request is for something
// else or for a defined model, so the normal runtime answers it.
func answerAnthropic(w http.ResponseWriter, r *http.Request, known func(string) bool) (*http.Request, bool) {
	if !strings.HasSuffix(r.URL.Path, "/v1/messages") {
		return r, false
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	clone := r.Clone(r.Context())
	clone.Body = io.NopCloser(bytes.NewReader(body))
	clone.ContentLength = int64(len(body))
	if err != nil {
		return clone, false
	}
	var req struct {
		Model  string `json:"model"`
		Stream bool   `json:"stream"`
	}
	if json.Unmarshal(body, &req) != nil || req.Model == "" || known(req.Model) {
		return clone, false
	}
	inTokens := len(body) / 4
	if inTokens < 1 {
		inTokens = 1
	}
	const text = "ok"
	message := func(content any, stop any, out int) map[string]any {
		return map[string]any{
			"id": "msg_vmodel_capture", "type": "message", "role": "assistant", "model": req.Model,
			"content": content, "stop_reason": stop, "stop_sequence": nil,
			"usage": map[string]any{"input_tokens": inTokens, "output_tokens": out},
		}
	}
	if !req.Stream {
		writeJSON(w, http.StatusOK, message([]any{map[string]any{"type": "text", "text": text}}, "end_turn", 2))
		return clone, true
	}
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)
	events := []struct {
		name string
		data any
	}{
		{"message_start", map[string]any{"type": "message_start", "message": message([]any{}, nil, 1)}},
		{"content_block_start", map[string]any{"type": "content_block_start", "index": 0, "content_block": map[string]any{"type": "text", "text": ""}}},
		{"ping", map[string]any{"type": "ping"}},
		{"content_block_delta", map[string]any{"type": "content_block_delta", "index": 0, "delta": map[string]any{"type": "text_delta", "text": text}}},
		{"content_block_stop", map[string]any{"type": "content_block_stop", "index": 0}},
		{"message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": "end_turn", "stop_sequence": nil}, "usage": map[string]any{"output_tokens": 2}}},
		{"message_stop", map[string]any{"type": "message_stop"}},
	}
	flusher, _ := w.(http.Flusher)
	for _, e := range events {
		data, _ := json.Marshal(e.data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.name, data)
		if flusher != nil {
			flusher.Flush()
		}
	}
	return clone, true
}
