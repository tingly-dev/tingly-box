// Package bind holds the request-binding helper shared by module handlers.
package bind

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/tingly-dev/tingly-box/internal/server/module/apierr"
)

// JSON binds the request body into dst. On failure it writes a 400 using w
// (apierr.Message or apierr.Failure, whichever shape the endpoint already
// answers with) carrying the binder's error text, and returns false; the
// caller just returns.
func JSON(c *gin.Context, dst any, w apierr.Writer) bool {
	if err := c.ShouldBindJSON(dst); err != nil {
		w(c, http.StatusBadRequest, err.Error())
		return false
	}
	return true
}
