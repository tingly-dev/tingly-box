package server

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/guardrails"
	serverguardrails "github.com/tingly-dev/tingly-box/internal/server/guardrails"
)

// applyGuardrailsToToolResultV1 evaluates tool_result content and replaces it when blocked.
func (s *Server) applyGuardrailsToToolResultV1(c *gin.Context, req *anthropic.MessageNewParams, session guardrailsSession) {
	toolResultText, toolResultBlocks, toolResultParts := serverguardrails.ExtractToolResultTextV1(req.Messages)
	logrus.Debugf("Guardrails: tool_result detected (v1) blocks=%d parts=%d len=%d", toolResultBlocks, toolResultParts, len(toolResultText))
	if toolResultText == "" {
		return
	}
	if strings.HasPrefix(toolResultText, serverguardrails.BlockPrefix) {
		return
	}

	result, ok := s.evaluateGuardrailsToolResult(
		c,
		session,
		toolResultText,
		serverguardrails.MessagesFromAnthropicV1(req.System, req.Messages),
	)
	if !ok {
		return
	}
	if result.Verdict == guardrails.VerdictBlock {
		message := serverguardrails.BlockMessageForToolResult(result)
		serverguardrails.ReplaceToolResultContentV1(req.Messages, message)
		c.Set("guardrails_block_message", message)
		c.Set("guardrails_block_index", 0)
		logrus.Debugf("Guardrails: tool_result replaced (v1) len=%d", len(message))
		return
	}
}

// applyGuardrailsToToolResultV1Beta evaluates tool_result content and replaces it when blocked.
func (s *Server) applyGuardrailsToToolResultV1Beta(c *gin.Context, req *anthropic.BetaMessageNewParams, session guardrailsSession) {
	toolResultText, toolResultBlocks, toolResultParts := serverguardrails.ExtractToolResultTextV1Beta(req.Messages)
	logrus.Debugf("Guardrails: tool_result detected (v1beta) blocks=%d parts=%d len=%d", toolResultBlocks, toolResultParts, len(toolResultText))
	if toolResultText == "" {
		return
	}
	if strings.HasPrefix(toolResultText, serverguardrails.BlockPrefix) {
		return
	}

	result, ok := s.evaluateGuardrailsToolResult(
		c,
		session,
		toolResultText,
		serverguardrails.MessagesFromAnthropicV1Beta(req.System, req.Messages),
	)
	if !ok {
		return
	}
	if result.Verdict == guardrails.VerdictBlock {
		message := serverguardrails.BlockMessageForToolResult(result)
		serverguardrails.ReplaceToolResultContentV1Beta(req.Messages, message)
		c.Set("guardrails_block_message", message)
		c.Set("guardrails_block_index", 0)
		logrus.Debugf("Guardrails: tool_result replaced (v1beta) len=%d", len(message))
		return
	}
}
