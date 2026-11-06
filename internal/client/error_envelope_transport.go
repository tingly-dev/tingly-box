package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// stringErrorTransport rewrites {"error":"text"} into {"error":{"message":"text"}}
// on failed responses. The OpenAI SDK decodes the "error" value into a struct,
// so a string there ends in "json: cannot unmarshal string into Go value of
// type apierror.Error" and the HTTP status is lost. Every other body shape
// already yields a proper *openai.Error, and the Anthropic SDK tolerates all of
// them, so nothing else is touched. See .design/upstream-error-normalization.md.
type stringErrorTransport struct {
	inner http.RoundTripper
}

func wrapWithStringErrorEnvelope(inner http.RoundTripper) http.RoundTripper {
	return &stringErrorTransport{inner: inner}
}

func (t *stringErrorTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.inner.RoundTrip(req)
	if err != nil || resp.StatusCode < 400 || resp.Body == nil {
		return resp, err
	}
	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(body), errReader{readErr}))
		return resp, nil
	}

	var probe struct {
		Error any `json:"error"`
	}
	if json.Unmarshal(body, &probe) == nil {
		if msg, ok := probe.Error.(string); ok {
			body, _ = json.Marshal(map[string]any{"error": map[string]string{"message": msg, "type": "upstream_error"}})
			resp.Header.Set("Content-Type", "application/json")
			resp.Header.Del("Content-Length")
			resp.ContentLength = int64(len(body))
		}
	}
	resp.Body = io.NopCloser(bytes.NewReader(body))
	return resp, nil
}

type errReader struct{ err error }

func (r errReader) Read([]byte) (int, error) { return 0, r.err }
