package middleware

import (
	"bytes"

	"github.com/gin-gonic/gin"
)

// responseBod yWriter is a wrapper around gin.ResponseWriter that captures the response body
type responseBodyWriter struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (r *responseBodyWriter) Write(b []byte) (int, error) {
	r.body.Write(b)
	return r.ResponseWriter.Write(b)
}
