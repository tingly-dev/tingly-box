package client

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/tingly-dev/tingly-box/ai"
	"github.com/tingly-dev/tingly-box/ai/oauth"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

// workbuddyRoundTripper layers the WorkBuddy client identification on an
// inner transport. The Authorization Bearer is set by the OpenAI SDK (and by
// ListModels for /v3/config). The
// values come from ai.WorkBuddySiteFor, the same table the login uses, so
// login and inference always present the same client.
type workbuddyRoundTripper struct {
	http.RoundTripper
	site   ai.WorkBuddySite
	userID string
}

func newWorkBuddyRoundTripper(inner http.RoundTripper, provider *typ.Provider) *workbuddyRoundTripper {
	t := &workbuddyRoundTripper{RoundTripper: inner}
	if provider != nil && provider.OAuthDetail != nil {
		t.site, _ = ai.WorkBuddySiteFor(provider.OAuthDetail.GetIssuer())
		if uid, ok := provider.OAuthDetail.ExtraFields[oauth.WorkBuddyMetaUserID].(string); ok {
			t.userID = uid
		}
	}
	return t
}

func (t *workbuddyRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	// A RoundTripper must not modify the caller's request.
	req = req.Clone(req.Context())
	for k, v := range t.site.ClientHeaders() {
		req.Header.Set(k, v)
	}
	// The product config answers per client: the CLI agent's model list
	// only comes back to the CLI User-Agent.
	if strings.HasSuffix(req.URL.Path, "/v3/config") {
		req.Header.Set("User-Agent", ai.WorkBuddyConfigUserAgent)
	}
	if t.userID != "" {
		req.Header.Set("X-User-Id", t.userID)
	}
	var names map[string]string
	if req.Method == http.MethodPost && strings.HasSuffix(req.URL.Path, "/chat/completions") && req.Body != nil {
		body, err := io.ReadAll(req.Body)
		_ = req.Body.Close()
		if err != nil {
			return nil, err
		}
		body, names = workbuddyFitChat(body)
		req.Body = io.NopCloser(bytes.NewReader(body))
		req.ContentLength = int64(len(body))
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	}
	resp, err := t.RoundTripper.RoundTrip(req)
	if err == nil {
		if normalizeErr := workbuddyNormalizeError(resp); normalizeErr != nil {
			return nil, normalizeErr
		}
	}
	if err != nil || len(names) == 0 || resp.Body == nil {
		return resp, err
	}
	// Tools renamed on the way out are named back on the way in, so the
	// client sees calls to the tools it declared.
	resp.Body = newWorkBuddyNameBackBody(resp.Body, names)
	resp.ContentLength = -1
	resp.Header.Del("Content-Length")
	return resp, nil
}

// workbuddyNameBackBody rewrites a chat answer as it is read: each SSE
// `data:` line, or a whole JSON body, gets workbuddyNameBack. Lines that do
// not parse pass through untouched.
type workbuddyNameBackBody struct {
	src   io.ReadCloser
	r     *bufio.Reader
	names map[string]string
	out   bytes.Buffer
	err   error
}

func newWorkBuddyNameBackBody(src io.ReadCloser, names map[string]string) io.ReadCloser {
	return &workbuddyNameBackBody{src: src, r: bufio.NewReader(src), names: names}
}

func (b *workbuddyNameBackBody) Read(p []byte) (int, error) {
	for b.out.Len() == 0 && b.err == nil {
		line, err := b.r.ReadBytes('\n')
		if len(line) > 0 {
			b.out.Write(b.nameBack(line))
		}
		b.err = err
	}
	if b.out.Len() > 0 {
		return b.out.Read(p)
	}
	return 0, b.err
}

func (b *workbuddyNameBackBody) nameBack(line []byte) []byte {
	trimmed := bytes.TrimRight(line, "\r\n")
	ending := line[len(trimmed):]
	prefix, payload := []byte(nil), trimmed
	if rest, ok := bytes.CutPrefix(trimmed, []byte("data:")); ok {
		prefix, payload = []byte("data: "), bytes.TrimLeft(rest, " ")
	}
	if len(payload) == 0 || payload[0] != '{' {
		return line
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var v map[string]any
	if dec.Decode(&v) != nil || !workbuddyNameBack(v, b.names) {
		return line
	}
	out, err := json.Marshal(v)
	if err != nil {
		return line
	}
	return append(append(prefix, out...), ending...)
}

func (b *workbuddyNameBackBody) Close() error { return b.src.Close() }
