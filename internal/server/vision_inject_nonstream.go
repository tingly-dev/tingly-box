package server

import (
	"bytes"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"

	"github.com/tingly-dev/tingly-box/internal/server/processor"
)

// VisionInjectNonStream wraps c.Writer for non-streaming responses on
// every route that participates in the vision-proxy contract. When a
// request carries descriptions (stashed by applyVisionProxy) AND the
// handler ultimately writes a JSON body (not SSE), the wrapper splices
// the description prefix into the first text field of the response.
//
// Streaming responses are handled by the protocol-level OnStreamEvent
// hook (see vision_inject_stream.go); the wrapper detects them via
// Content-Type at first Write and passes bytes through untouched, so
// installing this middleware on a route group that serves both is
// safe and cheap.
//
// One middleware per route group beats four handler patches: every
// non-stream JSON path goes through c.Writer.Write exactly once
// (gin's c.JSON serialises the entire body and writes it in one
// call), so a single first-write interception is sufficient.
func VisionInjectNonStream() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Cheap predicate before doing any wrapping: when the request
		// has no descriptions the middleware is a no-op.
		raw, ok := c.Get(GinKeyVisionDescriptions)
		if !ok {
			c.Next()
			return
		}
		descs, _ := raw.([]string)
		prefix := processor.BuildVisionDescriptionPrefix(descs)
		if prefix == "" {
			c.Next()
			return
		}
		c.Writer = &visionInjectWriter{
			ResponseWriter: c.Writer,
			path:           c.Request.URL.Path,
			prefix:         prefix,
		}
		c.Next()
	}
}

// visionInjectWriter intercepts the first Write call on a gin
// response. SSE responses are detected by Content-Type and forwarded
// unchanged (the protocol stream hook owns those). JSON responses are
// dispatched to a path-aware splicer that uses sjson to land the
// prefix in the first model-text field, preserving every other byte.
//
// Idempotent: after the first Write, the wrapper flips a flag and
// every subsequent byte is forwarded raw. Subsequent writes from the
// same handler (e.g. a multi-chunk SSE stream that the predicate
// mis-identified as JSON) therefore never see the splicer twice.
type visionInjectWriter struct {
	gin.ResponseWriter
	path     string
	prefix   string
	handled  bool
}

func (w *visionInjectWriter) Write(b []byte) (int, error) {
	if w.handled {
		return w.ResponseWriter.Write(b)
	}
	w.handled = true

	ct := w.Header().Get("Content-Type")
	if strings.HasPrefix(ct, "text/event-stream") {
		return w.ResponseWriter.Write(b)
	}

	patched, ok := spliceVisionPrefix(w.path, b, w.prefix)
	if !ok {
		return w.ResponseWriter.Write(b)
	}
	// gin's status header may already be committed by the time JSON
	// body lands; the Content-Length header (if set) needs adjusting.
	// gin.ResponseWriter writes Content-Length lazily based on the
	// Write payload size, so writing the patched bytes through the
	// embedded writer is enough — the wrapper does not interpose any
	// length math.
	n, err := w.ResponseWriter.Write(patched)
	if n == len(patched) && err == nil {
		// Caller expects the byte count of THEIR payload, not ours.
		return len(b), nil
	}
	return n, err
}

func (w *visionInjectWriter) WriteString(s string) (int, error) {
	return w.Write([]byte(s))
}

// spliceVisionPrefix routes by request path to the right per-transport
// splicer. Returns (patched, true) on success; (nil, false) when the
// payload doesn't look like the expected shape (so the caller forwards
// the original bytes — fail-open).
func spliceVisionPrefix(path string, body []byte, prefix string) ([]byte, bool) {
	switch {
	case strings.HasSuffix(path, "/chat/completions"):
		return spliceOpenAIChatBody(body, prefix)
	case strings.HasSuffix(path, "/responses"):
		return spliceOpenAIResponsesBody(body, prefix)
	case strings.HasSuffix(path, "/messages"):
		return spliceAnthropicMessagesBody(body, prefix)
	}
	return nil, false
}

// spliceOpenAIChatBody prepends the prefix to choices.0.message.content.
func spliceOpenAIChatBody(body []byte, prefix string) ([]byte, bool) {
	got := gjson.GetBytes(body, "choices.0.message.content")
	if !got.Exists() {
		return nil, false
	}
	patched, err := sjson.SetBytes(body, "choices.0.message.content", prefix+got.String())
	if err != nil {
		return nil, false
	}
	return patched, true
}

// spliceOpenAIResponsesBody finds the first output_text content part
// across all output items and prepends to its text. Responses bodies
// nest output[*].content[*]; the first content of type output_text is
// the assistant's primary text reply.
func spliceOpenAIResponsesBody(body []byte, prefix string) ([]byte, bool) {
	outputs := gjson.GetBytes(body, "output")
	if !outputs.IsArray() {
		return nil, false
	}
	var (
		foundOutputIdx  int
		foundContentIdx int
		foundText       string
		ok              bool
	)
	outputs.ForEach(func(oKey, oVal gjson.Result) bool {
		oIdx := int(oKey.Int())
		oVal.Get("content").ForEach(func(cKey, cVal gjson.Result) bool {
			if cVal.Get("type").String() == "output_text" {
				foundOutputIdx = oIdx
				foundContentIdx = int(cKey.Int())
				foundText = cVal.Get("text").String()
				ok = true
				return false
			}
			return true
		})
		return !ok
	})
	if !ok {
		return nil, false
	}
	keyPath := joinPath("output", foundOutputIdx, "content", foundContentIdx, "text")
	patched, err := sjson.SetBytes(body, keyPath, prefix+foundText)
	if err != nil {
		return nil, false
	}
	return patched, true
}

// spliceAnthropicMessagesBody finds the first text block in content[]
// and prepends to its text field. Anthropic non-stream bodies expose
// content as an array of typed blocks; we touch the first one whose
// type is "text", leaving tool_use and other arms alone.
func spliceAnthropicMessagesBody(body []byte, prefix string) ([]byte, bool) {
	blocks := gjson.GetBytes(body, "content")
	if !blocks.IsArray() {
		return nil, false
	}
	var (
		foundIdx  int
		foundText string
		ok        bool
	)
	blocks.ForEach(func(key, val gjson.Result) bool {
		if val.Get("type").String() == "text" {
			foundIdx = int(key.Int())
			foundText = val.Get("text").String()
			ok = true
			return false
		}
		return true
	})
	if !ok {
		return nil, false
	}
	keyPath := joinPath("content", foundIdx, "text")
	patched, err := sjson.SetBytes(body, keyPath, prefix+foundText)
	if err != nil {
		return nil, false
	}
	return patched, true
}

// joinPath builds a dotted sjson path from mixed string / int segments.
// sjson's syntax uses dots for object keys and integers for array
// indices: "output.0.content.1.text".
func joinPath(segments ...any) string {
	var sb bytes.Buffer
	for i, s := range segments {
		if i > 0 {
			sb.WriteByte('.')
		}
		switch v := s.(type) {
		case string:
			sb.WriteString(v)
		case int:
			sb.WriteString(intToStr(v))
		}
	}
	return sb.String()
}

func intToStr(i int) string {
	// Strconv-free to keep the hot path zero-dep; sjson indices are
	// small positive ints so this two-line implementation is enough.
	if i == 0 {
		return "0"
	}
	neg := i < 0
	if neg {
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
