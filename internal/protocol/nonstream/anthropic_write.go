package nonstream

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// WriteAnthropicMessage writes a non-streaming Anthropic message response.
//
// It prefers the SDK message's RawJSON, which is clean by construction — the
// upstream's original body on passthrough, and the converter's wire bytes on
// converted paths (the converters build a wire DTO, so RawJSON carries no
// zero-value noise like content[].citations: null that a plain struct marshal
// would emit and that strict clients reject). It falls back to a marshal only
// when RawJSON is empty.
//
// Callers that mutate the message after receiving it (e.g. the MCP tool loop
// filtering virtual tool_use blocks) must NOT use this helper: RawJSON would
// be stale. They marshal the struct directly so the wire reflects the mutation.
func WriteAnthropicMessage(c *gin.Context, msg any) {
	if r, ok := msg.(interface{ RawJSON() string }); ok {
		if raw := r.RawJSON(); raw != "" {
			c.Data(http.StatusOK, "application/json; charset=utf-8", []byte(raw))
			return
		}
	}
	c.JSON(http.StatusOK, msg)
}
