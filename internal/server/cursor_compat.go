package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/tingly-dev/tingly-box/internal/typ"
)

const cursorCompatExtraField = "__tb_cursor_compat"

// resolveCursorCompat determines whether Cursor compatibility handling should be enabled.
// Primary mechanism is rule flags; optional auto-detection is enabled via rule flags.
func resolveCursorCompat(c *gin.Context, rule *typ.Rule) bool {
	if rule == nil {
		return false
	}

	flags := rule.Flags
	if flags.CursorCompat {
		return true
	}
	if flags.CursorCompatAuto && isCursorRequest(c) {
		return true
	}
	return false
}

func isCursorRequest(c *gin.Context) bool {
	if c == nil {
		return false
	}
	userAgent := strings.ToLower(c.GetHeader("User-Agent"))
	if strings.Contains(userAgent, "cursor") {
		return true
	}
	clientName := strings.ToLower(c.GetHeader("X-Client-Name"))
	if clientName == "cursor" {
		return true
	}
	clientApp := strings.ToLower(c.GetHeader("X-Client-App"))
	if strings.Contains(clientApp, "cursor") {
		return true
	}
	return false
}

// applyCursorCompatFlag embeds a transient flag in request extra fields for downstream transforms.
// The flag is stripped before sending to the provider.
func applyCursorCompatFlag(req *openai.ChatCompletionNewParams, enabled bool) {
	if req == nil || !enabled {
		return
	}
	extra := req.ExtraFields()
	if extra == nil {
		extra = map[string]interface{}{}
	}
	extra[cursorCompatExtraField] = true
	req.SetExtraFields(extra)
}
