package server

import (
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"

	"github.com/tingly-dev/tingly-box/internal/mcp/runtime"
	"github.com/tingly-dev/tingly-box/internal/protocol/stream"
)

func hasOnlyMCPToolUsesV1(content []anthropic.ContentBlockUnion) ([]anthropic.ToolUseBlock, bool) {
	toolUses := make([]anthropic.ToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.ToolUseBlock:
			if !runtime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

func hasOnlyMCPToolUsesBeta(content []anthropic.BetaContentBlockUnion) ([]anthropic.BetaToolUseBlock, bool) {
	toolUses := make([]anthropic.BetaToolUseBlock, 0, len(content))
	for _, block := range content {
		switch v := block.AsAny().(type) {
		case anthropic.BetaToolUseBlock:
			if !runtime.IsMCPToolName(v.Name) {
				return nil, false
			}
			toolUses = append(toolUses, v)
		}
	}
	if len(toolUses) == 0 {
		return nil, false
	}
	return toolUses, true
}

// respondMCPError writes a JSON error response for non-streaming MCP tool call failures.
// This consolidates the ~10-line error block repeated across dispatch paths.
func respondMCPError(s *Server, c *gin.Context, recorder *ProtocolRecorder, err error, msg string) {
	s.trackUsageFromContext(c, 0, 0, err)
	c.JSON(http.StatusInternalServerError, ErrorResponse{
		Error: ErrorDetail{
			Message: msg + ": " + err.Error(),
			Type:    "api_error",
		},
	})
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// recordMCPError sends a streaming error response for streaming MCP tool call failures.
func recordMCPError(s *Server, c *gin.Context, err error, recorder *ProtocolRecorder) {
	s.trackUsageFromContext(c, 0, 0, err)
	stream.SendStreamingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}

// recordMCPForwardingError handles MCP errors in non-streaming forward paths.
func recordMCPForwardingError(s *Server, c *gin.Context, err error, recorder *ProtocolRecorder) {
	s.trackUsageFromContext(c, 0, 0, err)
	stream.SendForwardingError(c, err)
	if recorder != nil {
		recorder.RecordError(err)
	}
}
