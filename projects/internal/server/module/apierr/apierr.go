// Package apierr is the one JSON error shape every tingly-project endpoint
// uses: {"error": "<message>"}. tingly-box ended up with three shapes and had
// to keep all of them as wire contract; starting with one avoids that.
package apierr

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/projects/internal/db"
)

// ErrorResponse is the body of every non-2xx JSON response.
type ErrorResponse struct {
	Error string `json:"error" example:"project not found"`
}

// Message writes {"error": msg} with status.
func Message(c *gin.Context, status int, msg string) {
	c.AbortWithStatusJSON(status, ErrorResponse{Error: msg})
}

// BadRequest writes a 400 with msg.
func BadRequest(c *gin.Context, msg string) { Message(c, http.StatusBadRequest, msg) }

// Store maps a store error to a response: db.ErrNotFound becomes 404 with
// "<what> not found", anything else a 500.
func Store(c *gin.Context, err error, what string) {
	if errors.Is(err, db.ErrNotFound) {
		Message(c, http.StatusNotFound, what+" not found")
		return
	}
	Message(c, http.StatusInternalServerError, err.Error())
}

// Bind decodes the JSON body into req, writing a 400 on failure. It reports
// whether the handler should continue.
func Bind(c *gin.Context, req any) bool {
	if err := c.ShouldBindJSON(req); err != nil {
		BadRequest(c, err.Error())
		return false
	}
	return true
}
