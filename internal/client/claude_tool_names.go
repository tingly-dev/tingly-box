package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"unicode"

	anthropicOption "github.com/anthropics/anthropic-sdk-go/v2/option"
)

// mcpNamespaceSeparator separates the segments of an MCP tool name.
//
// Claude Code passes MCP tools through verbatim as mcp__<server>__<tool> —
// lowercase, unfolded — so a name already using this separator is left alone.
// Folding it would move the request away from real Claude Code traffic rather
// than towards it, and would flatten the namespace the client uses to route the
// call. Tingly-Box's own server tools follow the same convention
// (tingly_box_mcp__<source>__<tool>, see internal/tool.NormalizeToolName), so
// one rule covers both.
//
// A single-underscore "mcp_" prefix is deliberately *not* treated as a
// namespace: it is not a shape any first-party client emits, so it folds like
// any other snake_case name.
const mcpNamespaceSeparator = "__"

// claudeCodeToolName returns the name Claude Code would send for a tool.
//
// Anthropic classifies some OAuth traffic as third-party and bills it to Extra
// Usage rather than the subscription, which surfaces as a 400 "You're out of
// extra usage" once that bucket is spent; tool naming appears to be one of the
// signals. An earlier report measured a hard cutoff (~16 snake_case names);
// later probes — five arms up to 48 lowercase names at 100% density — did not
// reproduce it, so the exact trigger is unconfirmed (see PR #1634). The fold
// is kept regardless because it only moves names towards the shape Claude Code
// itself sends, and at worst is a no-op.
//
// MCP-namespaced names are exempt — see mcpNamespaceSeparator.
// oauthToolRenameMap then wins for the well-known Claude Code tools so those
// keep their exact official spelling (e.g. "ls" -> "LS", not "Ls"). Everything
// else falls back to a mechanical TitleCase.
func claudeCodeToolName(name string) string {
	if strings.Contains(name, mcpNamespaceSeparator) {
		return name
	}
	if mapped, ok := oauthToolRenameMap[name]; ok {
		return mapped
	}
	return titleCaseToolName(name)
}

// titleCaseToolName folds a snake_case tool name into TitleCase.
// Names that are already TitleCase come back unchanged.
func titleCaseToolName(name string) string {
	if name == "" {
		return name
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, part := range strings.Split(name, "_") {
		if part == "" {
			continue
		}
		b.WriteString(upperFirst(part))
	}
	if b.Len() == 0 {
		return name
	}
	return b.String()
}

func upperFirst(s string) string {
	if s == "" {
		return s
	}
	r := []rune(s)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// planToolRenames maps original tool name -> outbound name for a request.
//
// A rename is skipped when the target collides with another tool in the same
// request (either one already named that, or another rename targeting it).
// Two distinct tools sharing one outbound name would make the reverse mapping
// ambiguous and could dispatch a tool result to the wrong tool, so leaving the
// colliding pair untouched is the safe failure mode.
func planToolRenames(names []string) map[string]string {
	taken := make(map[string]struct{}, len(names))
	for _, n := range names {
		taken[n] = struct{}{}
	}
	plan := make(map[string]string, len(names))
	used := make(map[string]struct{}, len(names))
	for _, n := range names {
		newName := claudeCodeToolName(n)
		if newName == n {
			continue
		}
		if _, clash := taken[newName]; clash {
			continue
		}
		if _, clash := used[newName]; clash {
			continue
		}
		used[newName] = struct{}{}
		plan[n] = newName
	}
	return plan
}

// restoreToolNamesMiddleware rewrites renamed tool names back to the client's
// originals in a streaming response.
//
// The non-streaming paths restore names on the decoded message
// (restoreToolNamesInMessage), but a streamed response is handed to the caller
// as an SSE stream that never passes through that step. Without this the
// renamed name reaches the client, which then cannot match it against its own
// tool registry. Rewriting at the HTTP layer keeps the client interface
// unchanged and covers both the v1 and beta streaming paths.
func restoreToolNamesMiddleware(reverse map[string]string) anthropicOption.Middleware {
	return func(req *http.Request, next anthropicOption.MiddlewareNext) (*http.Response, error) {
		resp, err := next(req)
		if err != nil || resp == nil || resp.Body == nil {
			return resp, err
		}
		if !strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream") {
			return resp, nil
		}
		resp.Body = newSSEToolNameRewriter(resp.Body, reverse)
		return resp, nil
	}
}

// sseToolNameRewriter rewrites tool names in an SSE body as it streams.
//
// A tool name appears in exactly one event type: content_block_start, whose
// content_block is {"type":"tool_use","id":...,"name":...,"input":{}}. The
// arguments arrive separately as input_json_delta and never repeat the name,
// and message_delta/message_stop do not carry it — so this is a single
// interception point rather than a general rewrite of the stream.
//
// Rewriting is line-at-a-time so streaming is preserved: each SSE "data:" line
// is a complete JSON object, and lines that are not a tool_use
// content_block_start are passed through byte-for-byte.
type sseToolNameRewriter struct {
	inner   io.ReadCloser
	reader  *bufio.Reader
	reverse map[string]string
	pending []byte
	err     error
}

func newSSEToolNameRewriter(inner io.ReadCloser, reverse map[string]string) *sseToolNameRewriter {
	return &sseToolNameRewriter{
		inner:   inner,
		reader:  bufio.NewReader(inner),
		reverse: reverse,
	}
}

func (r *sseToolNameRewriter) Read(p []byte) (int, error) {
	for len(r.pending) == 0 {
		if r.err != nil {
			return 0, r.err
		}
		line, err := r.reader.ReadBytes('\n')
		if len(line) > 0 {
			r.pending = r.rewriteLine(line)
		}
		if err != nil {
			r.err = err
			if len(r.pending) == 0 {
				return 0, err
			}
		}
	}
	n := copy(p, r.pending)
	r.pending = r.pending[n:]
	return n, nil
}

func (r *sseToolNameRewriter) Close() error { return r.inner.Close() }

func (r *sseToolNameRewriter) rewriteLine(line []byte) []byte {
	trimmed := bytes.TrimRight(line, "\r\n")
	if !bytes.HasPrefix(trimmed, []byte("data:")) {
		return line
	}
	payload := bytes.TrimSpace(trimmed[len("data:"):])
	// Cheap reject before paying for a JSON decode: every rewritable line
	// carries both markers.
	if !bytes.Contains(payload, []byte(`"content_block_start"`)) ||
		!bytes.Contains(payload, []byte(`"tool_use"`)) {
		return line
	}
	var evt map[string]any
	if err := json.Unmarshal(payload, &evt); err != nil {
		return line
	}
	block, ok := evt["content_block"].(map[string]any)
	if !ok {
		return line
	}
	if blockType, _ := block["type"].(string); blockType != "tool_use" {
		return line
	}
	name, _ := block["name"].(string)
	orig, ok := r.reverse[name]
	if !ok {
		return line
	}
	block["name"] = orig
	out, err := json.Marshal(evt)
	if err != nil {
		return line
	}
	// Preserve the original line ending so SSE framing is untouched.
	ending := line[len(trimmed):]
	rewritten := make([]byte, 0, len("data: ")+len(out)+len(ending))
	rewritten = append(rewritten, "data: "...)
	rewritten = append(rewritten, out...)
	rewritten = append(rewritten, ending...)
	return rewritten
}
