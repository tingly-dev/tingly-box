package anthropic

import (
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// ClaudeCodeCompactTransform conditionally applies XML compression for Claude Code.
// Only activates when the last user message contains "compact" (case-insensitive).
//
// Note: there is intentionally NO "request must carry tool definitions" gate —
// a long text-only conversation that issues /compact should still be compressed.
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
	case *sdk.MessageNewParams:
		if !t.shouldApplyV1(req) {
			logrus.Debugf("[claude_code_compact] v1: conditions not met, skipping")
			return nil
		}
		logrus.Debugf("[claude_code_compact] v1: conditions met, applying compression")
		return t.inner.Apply(ctx)

	case *sdk.BetaMessageNewParams:
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

func (t *ClaudeCodeCompactTransform) shouldApplyV1(req *sdk.MessageNewParams) bool {
	return lastUserMessageContainsCompact(req.Messages)
}

func (t *ClaudeCodeCompactTransform) shouldApplyV1Beta(req *sdk.BetaMessageNewParams) bool {
	return lastUserMessageContainsCompactBeta(req.Messages)
}

func lastUserMessageContainsCompact(messages []sdk.MessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			return extractAndCheckText(messages[i].Content)
		}
	}
	return false
}

func lastUserMessageContainsCompactBeta(messages []sdk.BetaMessageParam) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		if string(messages[i].Role) == "user" {
			return extractAndCheckTextBeta(messages[i].Content)
		}
	}
	return false
}

func extractAndCheckText(content []sdk.ContentBlockParamUnion) bool {
	for _, block := range content {
		if block.OfText != nil {
			if strings.Contains(strings.ToLower(block.OfText.Text), "compact") {
				return true
			}
		}
	}
	return false
}

func extractAndCheckTextBeta(content []sdk.BetaContentBlockParamUnion) bool {
	for _, block := range content {
		if block.OfText != nil {
			if strings.Contains(strings.ToLower(block.OfText.Text), "compact") {
				return true
			}
		}
	}
	return false
}
