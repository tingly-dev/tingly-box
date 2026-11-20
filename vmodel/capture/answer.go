package capture

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

// DefaultModel is the id of the built-in model that answers requests for
// model ids nobody defined while capturing. The caller registers it (see
// internal/command) so explicit definitions keep priority and a user-supplied
// definition with this id wins.
const DefaultModel = "capture-default"

// Answer wraps next so a real client gets a normal-looking answer while it is
// being captured: the point is to observe the client, so it must neither retry
// nor abort.
//
//   - A request for a model id for which known returns false is answered by
//     DefaultModel instead of a 404. Only the copy next sees is rewritten; the
//     capture keeps the client's bytes. Anthropic Messages requests are
//     answered here, in the exact shape of the real API (see anthropic.go):
//     the shared mock's stream (content:null, stop_reason:"stop") makes Claude
//     Code give up on the stream and retry without streaming, which would put
//     a second, unrepresentative request into the capture.
//   - POST …/v1/messages/count_tokens and a few telemetry endpoints get a small
//     fixed answer instead of the protocol-shaped 501.
//
// Everything else, including every unknown path, is next's business.
func Answer(next http.Handler, known func(id string) bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/v1/messages/count_tokens") {
			body, _ := io.ReadAll(r.Body)
			n := len(body) / 4
			if n < 1 {
				n = 1
			}
			writeJSON(w, http.StatusOK, map[string]int{"input_tokens": n})
			return
		}
		if isTelemetry(r.URL.Path) {
			_, _ = io.Copy(io.Discard, r.Body)
			writeJSON(w, http.StatusOK, map[string]any{})
			return
		}
		if r.Method == http.MethodPost {
			var done bool
			if r, done = answerAnthropic(w, r, known); done {
				return
			}
			r = withFallbackModel(r, known)
		}
		next.ServeHTTP(w, r)
	})
}

// isTelemetry lists non-inference traffic Claude Code sends that a capture
// should absorb quietly. /api/oauth/* is deliberately absent: it is recorded
// but not simulated.
func isTelemetry(path string) bool {
	for _, p := range []string{"/api/event_logging/", "/api/claude_code/metrics", "/api/eval/", "/v1/messages/cache_touch"} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// inferencePath reports whether path carries a model id this handler may
// rewrite.
func inferencePath(path string) bool {
	switch {
	case strings.HasSuffix(path, "/v1/messages"),
		strings.HasSuffix(path, "/chat/completions"),
		strings.HasSuffix(path, "/responses"),
		strings.HasPrefix(path, "/v1beta/models/"):
		return true
	}
	return false
}

func withFallbackModel(r *http.Request, known func(string) bool) *http.Request {
	if !inferencePath(r.URL.Path) {
		return r
	}
	body, err := io.ReadAll(r.Body)
	_ = r.Body.Close()
	clone := r.Clone(r.Context())
	restore := func(b []byte) {
		clone.Body = io.NopCloser(bytes.NewReader(b))
		clone.ContentLength = int64(len(b))
	}
	restore(body)
	if err != nil {
		return clone
	}

	if rest, ok := strings.CutPrefix(r.URL.Path, "/v1beta/models/"); ok {
		id, action, _ := strings.Cut(rest, ":")
		if id != "" && !known(id) {
			u := *r.URL
			u.Path = "/v1beta/models/" + DefaultModel
			if action != "" {
				u.Path += ":" + action
			}
			clone.URL = &u
		}
		return clone
	}

	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || obj == nil {
		return clone
	}
	var id string
	if json.Unmarshal(obj["model"], &id) != nil || id == "" || known(id) {
		return clone
	}
	obj["model"], _ = json.Marshal(DefaultModel)
	if rewritten, err := json.Marshal(obj); err == nil {
		restore(rewritten)
	}
	return clone
}
