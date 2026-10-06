package capture

import (
	"bytes"
	"io"
	"net/http"
	"strings"
	"sync"
)

// WrapTransport wraps a wire-level RoundTripper so every request made under
// an enabled Trace becomes an Exchange. Mount it directly on the wire base —
// inside every header- or body-rewriting round tripper — so it sees the
// request that actually leaves the gateway (.design/recording.md §4.1). It
// only reads, so vendor chains may mount it too.
func WrapTransport(base http.RoundTripper, p ProviderInfo) http.RoundTripper {
	if base == nil {
		return nil
	}
	return &transport{base: base, provider: p}
}

type transport struct {
	base     http.RoundTripper
	provider ProviderInfo
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	ex := FromContext(req.Context()).beginExchange(t.provider)
	if ex == nil {
		return t.base.RoundTrip(req)
	}

	reqMsg := &Message{
		Method:   req.Method,
		URL:      req.URL.String(),
		Headers:  redactHeaders(req.Header),
		Complete: true,
	}
	if ex.withBody.request {
		body, err := requestBody(req)
		if err != nil {
			ex.fail(err)
			return nil, err
		}
		reqMsg.Size = int64(len(body))
		if len(body) > maxBodyBytes {
			body = body[:maxBodyBytes]
			reqMsg.Truncated = true
			reqMsg.Complete = false
		}
		reqMsg.Body = body
	}
	reqMsg.ContentType = req.Header.Get("Content-Type")
	ex.setRequest(reqMsg)

	resp, err := t.base.RoundTrip(req)
	if err != nil {
		ex.fail(err)
		return resp, err
	}
	ct := resp.Header.Get("Content-Type")
	ex.setResponseHead(&Message{
		Status:      resp.StatusCode,
		Headers:     redactHeaders(resp.Header),
		ContentType: ct,
		Stream:      strings.HasPrefix(strings.ToLower(ct), "text/event-stream"),
	})
	if resp.Body == nil || resp.Body == http.NoBody {
		ex.closeBody(true)
		return resp, nil
	}
	resp.Body = &teeBody{rc: resp.Body, ex: ex}
	return resp, nil
}

// requestBody returns the request body bytes without consuming req.Body:
// GetBody (set by the SDKs for in-memory bodies) yields an independent copy;
// otherwise the body is read once and replaced by an in-memory reader.
func requestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil || req.Body == http.NoBody {
		return nil, nil
	}
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err == nil {
			defer rc.Close()
			return io.ReadAll(rc)
		}
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	return body, nil
}

// teeBody mirrors a response body into its Exchange as the SDK reads it, so
// a streamed (SSE) response is captured verbatim without extra buffering on
// the read path. EOF marks the body complete; Close before EOF (client gone,
// failover abandoning the attempt) leaves it incomplete.
type teeBody struct {
	rc   io.ReadCloser
	ex   *Exchange
	once sync.Once
	eof  bool
}

func (b *teeBody) Read(p []byte) (int, error) {
	n, err := b.rc.Read(p)
	if n > 0 {
		b.ex.appendBody(p[:n])
	}
	if err == io.EOF {
		b.eof = true
		b.finish()
	} else if err != nil {
		b.ex.fail(err)
	}
	return n, err
}

func (b *teeBody) Close() error {
	err := b.rc.Close()
	b.finish()
	return err
}

func (b *teeBody) finish() {
	b.once.Do(func() { b.ex.closeBody(b.eof) })
}
