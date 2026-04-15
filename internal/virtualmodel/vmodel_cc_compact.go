package virtualmodel

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// ClaudeCodeCompactTransform conditionally applies XML compression for Claude Code.
// Only activates when:
// 1. Last user message contains "compact" (case-insensitive)
// 2. Request has tool definitions
type ClaudeCodeCompactTransform struct {
	inner *smart_compact.XMLCompactTransform
}

// NewClaudeCodeCompactTransform creates a new ClaudeCodeCompactTransform.
func NewClaudeCodeCompactTransform() transform.Transform {
	return &ClaudeCodeCompactTransform{
		inner: smart_compact.NewXMLCompactTransform().(*smart_compact.XMLCompactTransform),
	}
}

// Name returns the transform identifier.
func (t *ClaudeCodeCompactTransform) Name() string {
	return "claude_code_compact"
}

// Apply applies XML compression only if conditions are met.
func (t *ClaudeCodeCompactTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		if !t.shouldApplyV1(req) {
			logrus.Debugf("[claude_code_compact] v1: conditions not met, skipping")
			return nil
		}
		logrus.Debugf("[claude_code_compact] v1: conditions met, applying compression")
		return t.inner.Apply(ctx)

	case *anthropic.BetaMessageNewParams:
		if !t.shouldApplyV1Beta(req) {
			logrus.Debugf("[claude_code_compact] v1beta: conditions not met, skipping")
			return nil
		}
		logrus.Debugf("[claude_code_compact] v1beta: conditions met, applying compression")
		return t.inner.Apply(ctx)

	default:
		return nil
	}
}

// shouldApplyV1 checks v1 conditions for compression.
func (t *ClaudeCodeCompactTransform) shouldApplyV1(req *anthropic.MessageNewParams) bool {
	// Must have tools
	if req.Tools == nil || len(req.Tools) == 0 {
		return false
	}
	// Last user message must contain "compact"
	return lastUserMessageContainsCompact(req.Messages)
}

// shouldApplyV1Beta checks v1beta conditions for compression.
func (t *ClaudeCodeCompactTransform) shouldApplyV1Beta(req *anthropic.BetaMessageNewParams) bool {
	// Must have tools
	if req.Tools == nil || len(req.Tools) == 0 {
		return false
	}
	// Last user message must contain "compact"
	return lastUserMessageContainsCompactBeta(req.Messages)
}

// lastUserMessageContainsCompact checks if the last user message contains "compact".
func lastUserMessageContainsCompact(messages []anthropic.MessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			return extractAndCheckText(messages[i].Content)
		}
	}
	return false
}

// lastUserMessageContainsCompactBeta checks if the last user message contains "compact" for beta API.
func lastUserMessageContainsCompactBeta(messages []anthropic.BetaMessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			return extractAndCheckTextBeta(messages[i].Content)
		}
	}
	return false
}

// extractAndCheckText extracts text from v1 content blocks and checks for "compact".
func extractAndCheckText(content []anthropic.ContentBlockParamUnion) bool {
	for _, block := range content {
		if block.OfText != nil {
			if strings.Contains(strings.ToLower(block.OfText.Text), "compact") {
				return true
			}
		}
	}
	return false
}

// extractAndCheckTextBeta extracts text from beta content blocks and checks for "compact".
func extractAndCheckTextBeta(content []anthropic.BetaContentBlockParamUnion) bool {
	for _, block := range content {
		if block.OfText != nil {
			if strings.Contains(strings.ToLower(block.OfText.Text), "compact") {
				return true
			}
		}
	}
	return false
}
