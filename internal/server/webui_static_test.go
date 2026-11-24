package server

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/gin-gonic/gin"
)

// A tiny stand-in for the embedded Vite build. The ".br" payload is not real
// brotli (the standard library has no encoder, and the handler never decodes
// it): the tests only check that exactly those bytes are served, labelled br.
const (
	webJS      = "export const a = 'tingly box';\n"
	webBrBytes = "BR-PAYLOAD-NOT-REAL-BROTLI"
	webIndex   = "<!doctype html><title>Tingly Box</title>"
	testChunk  = "/assets/index-AbCd1234.js"
)

func gzipBytes(t testing.TB, s string) []byte {
	t.Helper()
	var b bytes.Buffer
	w := gzip.NewWriter(&b)
	if _, err := w.Write([]byte(s)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func newWebEngine(t *testing.T, files fstest.MapFS) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	e := gin.New()
	registerWebStatic(e, files)
	return e
}

func fullBuild(t *testing.T) fstest.MapFS {
	return fstest.MapFS{
		"index.html":                       {Data: []byte(webIndex)},
		"index.html.gz":                    {Data: gzipBytes(t, webIndex)},
		"assets/index-AbCd1234.js":         {Data: []byte(webJS)},
		"assets/index-AbCd1234.js.gz":      {Data: gzipBytes(t, webJS)},
		"assets/index-AbCd1234.js.br":      {Data: []byte(webBrBytes)},
		"assets/icon.svg":                  {Data: []byte("<svg/>")},
		"assets/vendor-Zz9yXw8v.css":       {Data: []byte("a{}")},
		"assets/only-plain-Qq1wEr2t.json":  {Data: []byte(`{"a":1}`)},
		"assets/foo-barbaric.png":          {Data: []byte("png")}, // 8 letters, but not a hash
		"assets/nested/chunk-AbCd1234.mjs": {Data: []byte("export {}")},
	}
}

func get(e *gin.Engine, method, path string, hdr ...string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	for i := 0; i+1 < len(hdr); i += 2 {
		req.Header.Set(hdr[i], hdr[i+1])
	}
	w := httptest.NewRecorder()
	e.ServeHTTP(w, req)
	return w
}

func TestWebAssets_HashedIsImmutable(t *testing.T) {
	w := get(newWebEngine(t, fullBuild(t)), "GET", testChunk)
	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Errorf("Cache-Control = %q", got)
	}
	if w.Header().Get("ETag") == "" {
		t.Error("no ETag")
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Errorf("Content-Type = %q", ct)
	}
	if w.Body.String() != webJS {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestWebAssets_UnhashedRevalidates(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	for _, p := range []string{"/assets/icon.svg", "/assets/foo-barbaric.png"} {
		w := get(e, "GET", p)
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache (revalidate via ETag)", p, got)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", p)
		}
		if w2 := get(e, "GET", p, "If-None-Match", etag); w2.Code != 304 {
			t.Errorf("%s revalidation = %d, want 304", p, w2.Code)
		}
	}
	// hashed names also in subdirectories and with other extensions
	for _, p := range []string{"/assets/vendor-Zz9yXw8v.css", "/assets/nested/chunk-AbCd1234.mjs", "/assets/only-plain-Qq1wEr2t.json"} {
		if got := get(e, "GET", p).Header().Get("Cache-Control"); !strings.Contains(got, "immutable") {
			t.Errorf("%s Cache-Control = %q, want immutable", p, got)
		}
	}
}

func TestWebAssets_ConditionalGet304(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	first := get(e, "GET", testChunk)
	w := get(e, "GET", testChunk, "If-None-Match", first.Header().Get("ETag"))
	if w.Code != 304 {
		t.Fatalf("status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("304 carried %d body bytes", w.Body.Len())
	}
	if w := get(e, "GET", testChunk, "If-None-Match", `"stale"`); w.Code != 200 {
		t.Errorf("stale validator got %d, want 200", w.Code)
	}
}

func TestWebAssets_Negotiation(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	cases := []struct {
		name, accept, wantEnc, wantBody string
	}{
		{"brotli preferred", "gzip, deflate, br, zstd", "br", webBrBytes},
		{"gzip only", "gzip", "gzip", string(gzipBytes(t, webJS))},
		{"identity", "", "", webJS},
		{"identity explicit", "identity", "", webJS},
		{"br refused", "br;q=0, gzip", "gzip", string(gzipBytes(t, webJS))},
		{"unsupported only", "zstd", "", webJS},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var w *httptest.ResponseRecorder
			if tc.accept == "" {
				w = get(e, "GET", testChunk)
			} else {
				w = get(e, "GET", testChunk, "Accept-Encoding", tc.accept)
			}
			if got := w.Header().Get("Content-Encoding"); got != tc.wantEnc {
				t.Errorf("Content-Encoding = %q, want %q", got, tc.wantEnc)
			}
			if w.Body.String() != tc.wantBody {
				t.Errorf("body = %q", w.Body.String())
			}
			if v := w.Header().Get("Vary"); !strings.Contains(v, "Accept-Encoding") {
				t.Errorf("Vary = %q, want Accept-Encoding", v)
			}
			if tc.wantEnc != "" && w.Header().Get("Content-Length") == "" {
				t.Error("encoded response has no Content-Length")
			}
			if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "javascript") {
				t.Errorf("Content-Type = %q (must describe the decoded file, not .br/.gz)", ct)
			}
		})
	}
}

// Each representation needs its own validator, or a cache could answer a
// gzip client's revalidation with a 304 for the brotli copy it holds.
func TestWebAssets_ETagPerRepresentation(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	seen := map[string]string{}
	for _, accept := range []string{"br", "gzip", "identity"} {
		etag := get(e, "GET", testChunk, "Accept-Encoding", accept).Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", accept)
		}
		if prev, dup := seen[etag]; dup {
			t.Errorf("%s and %s share ETag %s", accept, prev, etag)
		}
		seen[etag] = accept
	}
	brTag := get(e, "GET", testChunk, "Accept-Encoding", "br").Header().Get("ETag")
	if w := get(e, "GET", testChunk, "Accept-Encoding", "gzip", "If-None-Match", brTag); w.Code != 200 {
		t.Errorf("br validator accepted for gzip representation: %d", w.Code)
	}
}

// A dev build (`pnpm build:dev`) ships no precompressed files: gzip clients
// still get a compressed body, computed once and reused.
func TestWebAssets_GzipFallbackWhenNotPrecompressed(t *testing.T) {
	big := strings.Repeat("export const a = 'tingly box';\n", 2000)
	e := newWebEngine(t, fstest.MapFS{
		"index.html":               {Data: []byte(webIndex)},
		"assets/index-AbCd1234.js": {Data: []byte(big)},
		"assets/logo-AbCd1234.png": {Data: []byte(big)}, // already-compressed type: left alone
	})
	w := get(e, "GET", testChunk, "Accept-Encoding", "br, gzip")
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", w.Header().Get("Content-Encoding"))
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	got, _ := io.ReadAll(zr)
	if string(got) != big {
		t.Error("gzip fallback does not round-trip")
	}
	if w.Body.Len() >= len(big)/4 {
		t.Errorf("compressed body %d bytes for %d raw", w.Body.Len(), len(big))
	}
	if w2 := get(e, "GET", testChunk, "Accept-Encoding", "gzip"); w2.Header().Get("ETag") != w.Header().Get("ETag") {
		t.Error("ETag not stable across requests")
	}
	if p := get(e, "GET", "/assets/logo-AbCd1234.png", "Accept-Encoding", "gzip"); p.Header().Get("Content-Encoding") != "" {
		t.Error("png must not be re-compressed")
	}
}

func TestWebAssets_MissingIs404NotIndex(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	for _, p := range []string{
		"/assets/gone-ZZZZZZZZ.js",
		"/assets/",
		"/assets/nested",
		"/assets/index-AbCd1234.js.br", // variants are not addressable
		"/assets/index-AbCd1234.js.gz",
		"/assets/../index.html",
	} {
		w := get(e, "GET", p)
		if w.Code != 404 {
			t.Errorf("%s = %d, want 404", p, w.Code)
		}
		if strings.Contains(w.Body.String(), "<!doctype") {
			t.Errorf("%s answered with index.html", p)
		}
		if cc := w.Header().Get("Cache-Control"); strings.Contains(cc, "immutable") {
			t.Errorf("%s 404 is cacheable: %q", p, cc)
		}
	}
}

func TestWebAssets_HeadAndRange(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	h := get(e, "HEAD", testChunk, "Accept-Encoding", "gzip")
	if h.Code != 200 || h.Body.Len() != 0 || h.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("HEAD: code=%d body=%d enc=%q", h.Code, h.Body.Len(), h.Header().Get("Content-Encoding"))
	}
	r := get(e, "GET", testChunk, "Range", "bytes=0-5")
	if r.Code != 206 || r.Body.String() != webJS[:6] {
		t.Errorf("range: code=%d body=%q", r.Code, r.Body.String())
	}
}

func TestWebAssets_RangeIgnoresEncoding(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	r := get(e, "GET", testChunk, "Range", "bytes=0-5", "Accept-Encoding", "br, gzip")
	if r.Code != 206 || r.Body.String() != webJS[:6] || r.Header().Get("Content-Encoding") != "" {
		t.Errorf("code=%d body=%q enc=%q", r.Code, r.Body.String(), r.Header().Get("Content-Encoding"))
	}
}

func TestWebIndex_RevalidatesAndCompresses(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	for _, p := range []string{"/", "/provider/settings/detail/123", "/login/tb-user-x"} {
		w := get(e, "GET", p)
		if w.Code != 200 || w.Body.String() != webIndex {
			t.Fatalf("%s: code=%d body=%q", p, w.Code, w.Body.String())
		}
		// index.html names the current chunks: it must be revalidated, but a
		// validator makes that a 304 instead of a full download.
		if got := w.Header().Get("Cache-Control"); got != "no-cache" {
			t.Errorf("%s Cache-Control = %q, want no-cache", p, got)
		}
		etag := w.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("%s: no ETag", p)
		}
		if w2 := get(e, "GET", p, "If-None-Match", etag); w2.Code != 304 {
			t.Errorf("%s revalidation = %d, want 304", p, w2.Code)
		}
		if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
			t.Errorf("%s Content-Type = %q", p, ct)
		}
	}
	w := get(e, "GET", "/", "Accept-Encoding", "gzip")
	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Errorf("index not gzip-encoded: %q", w.Header().Get("Content-Encoding"))
	}
}

func TestWebIndex_APIPathsStay404JSON(t *testing.T) {
	e := newWebEngine(t, fullBuild(t))
	for _, p := range []string{"/api/v1/nope", "/v1/nope", "/openai/x", "/anthropic/x", "/tingly/x"} {
		w := get(e, "GET", p)
		if w.Code != 404 || !strings.Contains(w.Header().Get("Content-Type"), "json") {
			t.Errorf("%s: code=%d type=%q", p, w.Code, w.Header().Get("Content-Type"))
		}
	}
}

func TestWebIndex_MissingBuildIs500(t *testing.T) {
	// the repo ships only a placeholder in internal/web/dist
	e := newWebEngine(t, fstest.MapFS{"PLACEHOLDER.md": {Data: []byte("x")}})
	if w := get(e, "GET", "/"); w.Code != http.StatusInternalServerError {
		t.Errorf("code = %d, want 500", w.Code)
	}
}

// The embedded files are served straight from the binary: a request costs a
// few allocations, not a read-and-compress of the file.
func BenchmarkWebAssets_Precompressed(b *testing.B) {
	big := strings.Repeat("export const a = 'tingly box';\n", 30000) // ~1 MB, like the largest chunk
	gz := gzipBytes(b, big)
	gin.SetMode(gin.TestMode)
	e := gin.New()
	registerWebStatic(e, fstest.MapFS{
		"index.html":                  {Data: []byte(webIndex)},
		"assets/index-AbCd1234.js":    {Data: []byte(big)},
		"assets/index-AbCd1234.js.gz": {Data: gz},
	})
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest("GET", testChunk, nil)
		req.Header.Set("Accept-Encoding", "gzip")
		e.ServeHTTP(httptest.NewRecorder(), req)
	}
}
