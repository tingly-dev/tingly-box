// Package paginate parses the limit/offset query parameters shared by list
// endpoints.
package paginate

import (
	"strconv"

	"github.com/gin-gonic/gin"
)

// Limit reads the "limit" query parameter. A missing, non-numeric or
// non-positive value yields def; a value above max is clamped to max. A max
// of 0 means no upper bound.
func Limit(c *gin.Context, def, max int) int {
	n, err := strconv.Atoi(c.Query("limit"))
	if err != nil || n <= 0 {
		return def
	}
	if max > 0 && n > max {
		return max
	}
	return n
}

// Offset reads the "offset" query parameter. A missing, non-numeric or
// negative value yields 0.
func Offset(c *gin.Context) int {
	n, err := strconv.Atoi(c.Query("offset"))
	if err != nil || n < 0 {
		return 0
	}
	return n
}
