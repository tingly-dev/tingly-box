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

func (g *gzipResponseWriter) Write(data []byte) (int, error) {
	g.wrote = true
	return g.gz.Write(data)
}

func (g *gzipResponseWriter) WriteString(s string) (int, error) {
	g.wrote = true
	return g.gz.Write([]byte(s))
}

// GzipHandler wraps a JSON-producing handler so its response body is
// gzip-compressed when the client accepts it. Intended for endpoints that can
// return large payloads (usage stats, time series, records). Do not use it on
// streaming/SSE endpoints. The unnamed signature keeps it assignable to both
// gin.HandlerFunc and swagger.Handler.
func GzipHandler(handler func(c *gin.Context)) func(c *gin.Context) {
	return func(c *gin.Context) {
		if !strings.Contains(c.GetHeader("Accept-Encoding"), "gzip") {
			handler(c)
			return
		}

		gz := gzipWriterPool.Get().(*gzip.Writer)
		gz.Reset(c.Writer)
		defer gzipWriterPool.Put(gz)

		writer := &gzipResponseWriter{ResponseWriter: c.Writer, gz: gz}
		c.Header("Content-Encoding", "gzip")
		c.Header("Vary", "Accept-Encoding")
		c.Writer = writer

		handler(c)

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
