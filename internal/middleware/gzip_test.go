package middleware

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGzipHandlerCompressesWhenAccepted(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	payload := strings.Repeat(`{"key":"value"},`, 500)
	engine.GET("/data", Gzip(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": payload})
	})

	req, _ := http.NewRequest("GET", "/data", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if w.Body.Len() >= len(payload) {
		t.Fatalf("compressed body (%d bytes) not smaller than payload (%d bytes)", w.Body.Len(), len(payload))
	}

	gz, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader failed: %v", err)
	}
	decoded, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}
	var body struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(decoded, &body); err != nil {
		t.Fatalf("decompressed body is not valid JSON: %v", err)
	}
	if body.Data != payload {
		t.Fatal("decompressed payload does not match original")
	}
}

func TestGzipHandlerRemovesUncompressedContentLength(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	payload := []byte(strings.Repeat("cloudflare origin response ", 200))
	engine.GET("/data", Gzip(), func(c *gin.Context) {
		// render.Data sets Content-Length before writing. Once Gzip transforms
		// the body, that uncompressed length must not reach the network.
		c.Data(http.StatusOK, "text/plain; charset=utf-8", payload)
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("Accept-Encoding", "br, gzip")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Header().Get("Content-Length"); got != "" {
		t.Fatalf("Content-Length = %q, want empty for a streamed gzip response", got)
	}
	if got := w.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}

	gz, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatalf("gzip.NewReader failed: %v", err)
	}
	decoded, err := io.ReadAll(gz)
	if err != nil {
		t.Fatalf("decompression failed: %v", err)
	}
	if string(decoded) != string(payload) {
		t.Fatal("decompressed payload does not match original")
	}
}

func TestGzipHandlerRemovesContentLengthBeforeExplicitHeaderCommit(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	payload := strings.Repeat("explicit header commit ", 100)
	engine.GET("/data", Gzip(), func(c *gin.Context) {
		c.Header("Content-Length", fmt.Sprintf("%d", len(payload)))
		c.Writer.WriteHeaderNow()
		_, _ = c.Writer.WriteString(payload)
	})

	req := httptest.NewRequest(http.MethodGet, "/data", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if got := w.Result().Header.Get("Content-Length"); got != "" {
		t.Fatalf("committed Content-Length = %q, want empty", got)
	}
}

func TestGzipHandlerPassthroughWithoutAcceptEncoding(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/data", Gzip(), func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	req, _ := http.NewRequest("GET", "/data", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("Content-Encoding = %q, want empty", got)
	}
	if !strings.Contains(w.Body.String(), `"ok":true`) {
		t.Fatalf("unexpected body: %s", w.Body.String())
	}
}

func TestGzipHandlerEmptyBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.GET("/empty", Gzip(), func(c *gin.Context) {
		c.Status(http.StatusNoContent)
	})

	req, _ := http.NewRequest("GET", "/empty", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("expected empty body, got %d bytes", w.Body.Len())
	}
}
