package script

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// rendered is a reply with templates filled and wire ids given.
type rendered struct {
	Text       string
	Chunks     []string
	Calls      []renderedCall
	StopReason string
	Usage      Usage
	MessageID  string
	Error      *apiError
}

type renderedCall struct {
	Ref   string // step.call, for reports
	ID    string // wire tool_use id
	Name  string
	Input json.RawMessage
}

type apiError struct {
	Status  int
	Type    string
	Message string
}

func wireID(prefix string, parts ...string) string {
	h := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return prefix + hex.EncodeToString(h[:12])
}

func errorFor(status int, typ, msg string) *apiError {
	if status == 0 {
		status = 500
	}
	if typ == "" {
		switch status {
		case 400:
			typ = "invalid_request_error"
		case 401:
			typ = "authentication_error"
		case 403:
			typ = "permission_error"
		case 404:
			typ = "not_found_error"
		case 408:
			typ = "timeout_error"
		case 429:
			typ = "rate_limit_error"
		case 529:
			typ = "overloaded_error"
		default:
			typ = "api_error"
		}
	}
	if msg == "" {
		msg = http.StatusText(status)
	}
	return &apiError{Status: status, Type: typ, Message: msg}
}

func writeError(w http.ResponseWriter, e *apiError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "error", "error": map[string]string{"type": e.Type, "message": e.Message}})
}

func (r *rendered) content() []map[string]any {
	out := []map[string]any{}
	if r.Text != "" {
		out = append(out, map[string]any{"type": "text", "text": r.Text})
	}
	for _, c := range r.Calls {
		out = append(out, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": c.Input})
	}
	return out
}

func (r *rendered) stopReason() string {
	if r.StopReason != "" {
		return r.StopReason
	}
	if len(r.Calls) > 0 {
		return "tool_use"
	}
	return "end_turn"
}

// write answers one request the way the Anthropic Messages API does.
func (r *rendered) write(w http.ResponseWriter, model string, stream bool) {
	if r.Error != nil {
		writeError(w, r.Error)
		return
	}
	usage := r.Usage
	if usage.InputTokens == 0 {
		usage.InputTokens = 100
	}
	if usage.OutputTokens == 0 {
		usage.OutputTokens = 20
	}
	if !stream {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": r.MessageID, "type": "message", "role": "assistant", "model": model,
			"content": r.content(), "stop_reason": r.stopReason(), "stop_sequence": nil,
			"usage": map[string]int{"input_tokens": usage.InputTokens, "output_tokens": usage.OutputTokens},
		})
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	event := func(name string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", name, b)
		if flusher != nil {
			flusher.Flush()
		}
	}
	event("message_start", map[string]any{"type": "message_start", "message": map[string]any{
		"id": r.MessageID, "type": "message", "role": "assistant", "model": model, "content": []any{},
		"stop_reason": nil, "stop_sequence": nil, "usage": map[string]int{"input_tokens": usage.InputTokens, "output_tokens": 1},
	}})
	event("ping", map[string]string{"type": "ping"})
	index := 0
	if r.Text != "" {
		event("content_block_start", map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]string{"type": "text", "text": ""}})
		chunks := r.Chunks
		if len(chunks) == 0 {
			chunks = []string{r.Text}
		}
		for _, c := range chunks {
			event("content_block_delta", map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]string{"type": "text_delta", "text": c}})
		}
		event("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	for _, c := range r.Calls {
		event("content_block_start", map[string]any{"type": "content_block_start", "index": index, "content_block": map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": map[string]any{}}})
		event("content_block_delta", map[string]any{"type": "content_block_delta", "index": index, "delta": map[string]string{"type": "input_json_delta", "partial_json": string(c.Input)}})
		event("content_block_stop", map[string]any{"type": "content_block_stop", "index": index})
		index++
	}
	event("message_delta", map[string]any{"type": "message_delta", "delta": map[string]any{"stop_reason": r.stopReason(), "stop_sequence": nil}, "usage": map[string]int{"output_tokens": usage.OutputTokens}})
	event("message_stop", map[string]string{"type": "message_stop"})
}
