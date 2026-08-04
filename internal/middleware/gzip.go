package middleware

import (
	"compress/gzip"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
)

// gzipWriterPool recycles gzip writers across requests to avoid the
// per-request allocation cost of gzip.NewWriter.
var gzipWriterPool = sync.Pool{
	New: func() interface{} {
		// BestSpeed keeps CPU cost negligible while still shrinking JSON
		// payloads roughly 10x.
		w, _ := gzip.NewWriterLevel(nil, gzip.BestSpeed)
		return w
	},
}

type gzipResponseWriter struct {
	gin.ResponseWriter
	gz    *gzip.Writer
	wrote bool
}

func (g *gzipResponseWriter) clearContentLength() {
	// A handler may have set Content-Length for the uncompressed body (Gin's
	// render.Data does this). Gzip changes the representation length, so the
	// stale value would make clients and reverse proxies treat the response as
	// truncated. Let net/http select the correct framing for the encoded body.
	g.Header().Del("Content-Length")
}

func (g *gzipResponseWriter) WriteHeader(code int) {
	g.clearContentLength()
	g.ResponseWriter.WriteHeader(code)
}

func (g *gzipResponseWriter) WriteHeaderNow() {
	g.clearContentLength()
	g.ResponseWriter.WriteHeaderNow()
}

func (g *gzipResponseWriter) prepareWrite() {
	g.clearContentLength()
	g.wrote = true
}

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	g.prepareWrite()
	return g.gz.Write(data)
}

func (g *gzipResponseWriter) WriteString(s string) (int, error) {
	g.prepareWrite()
	return g.gz.Write([]byte(s))
}

// Gzip returns gin middleware that gzip-compresses the response body when
// the client accepts it. Intended for endpoints that can return large JSON
// payloads (usage stats, time series, records) — register it per-route via
// swagger.WithMiddleware(middleware.Gzip()) rather than wrapping the handler
// directly, so it composes through the normal auth/CORS middleware chain
// instead of bypassing it. Do not use it on streaming/SSE endpoints.
func Gzip() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			c.Next()
			return
		}

		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(c.Writer)
		defer gzipWriterPool.Put(gz)

		writer := &gzipResponseWriter{ResponseWriter: c.Writer, gz: gz}
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = writer

		c.Next()

		c.Writer = writer.ResponseWriter
		if writer.wrote {
			_ = gz.Close()
		} else {
			// Nothing was written (e.g. 204); drop the compression headers
			// instead of emitting an empty gzip stream.
			header := c.Writer.Header()
			header.Del("Content-Encoding")
		}
	}
}
