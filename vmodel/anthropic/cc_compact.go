package anthropic

import (
	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
	"github.com/tingly-dev/tingly-box/internal/smart_compact"
)

// ClaudeCodeCompactTransform applies XML compaction for Claude Code requests
// routed to the rapid-compact virtual model.
//
// Gating note: the wake-keyword decision (whether a request is a rapid-compact
// request at all) lives upfront in Server.applyCompactWake — it checks the
// configurable compact_keyword flag against the latest user message and, on a
// match, forces service selection straight to this virtual model (see
// internal/server/compact_wake.go). By the time a request reaches this
// transform it has already been selected for compaction, so this layer only
// keeps a minimal structural guard: it compacts when the request carries tool
// definitions (the shape of a real Claude Code agent turn) and otherwise
// passes through untouched. The keyword is intentionally NOT re-checked here —
// re-checking a literal word would break custom keywords and duplicate the
// gating that already happened upfront.
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

// Apply applies XML compression when the request carries tool definitions.
func (t *ClaudeCodeCompactTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *sdk.MessageNewParams:
		if len(req.Tools) == 0 {
			logrus.Debugf("[claude_code_compact] v1: no tools, skipping")
			return nil
		}
		logrus.Debugf("[claude_code_compact] v1: applying compression")
		return t.inner.Apply(ctx)

	case *sdk.BetaMessageNewParams:
		if len(req.Tools) == 0 {
			logrus.Debugf("[claude_code_compact] v1beta: no tools, skipping")
			return nil
		}
		logrus.Debugf("[claude_code_compact] v1beta: applying compression")
		return t.inner.Apply(ctx)

	default:
		return nil
	}
}
