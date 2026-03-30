package server

import (
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	protocoltransform "github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// CleanHeaderTransform strips injected billing header blocks from system messages.
// Used by Claude Code scenarios to remove upstream-injected x-anthropic-billing-header blocks.
// Only added to the chain when CleanHeader flag is true.
type CleanHeaderTransform struct{}

// NewCleanHeaderTransform creates a CleanHeaderTransform.
func NewCleanHeaderTransform() *CleanHeaderTransform {
	return &CleanHeaderTransform{}
}

func (t *CleanHeaderTransform) Name() string { return "clean_header" }

func (t *CleanHeaderTransform) Apply(ctx *protocoltransform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		req.System = CleanSystemMessages(req.System)
	case *anthropic.BetaMessageNewParams:
		req.System = CleanBetaSystemMessages(req.System)
	}
	return nil
}

// CleanSystemMessages removes billing header messages from system blocks
// This is used for Claude Code scenario to filter out injected billing headers
func CleanSystemMessages(blocks []anthropic.TextBlockParam) []anthropic.TextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.TextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}

// CleanBetaSystemMessages removes billing header messages from beta system blocks
func CleanBetaSystemMessages(blocks []anthropic.BetaTextBlockParam) []anthropic.BetaTextBlockParam {
	if len(blocks) == 0 {
		return blocks
	}
	result := make([]anthropic.BetaTextBlockParam, 0, len(blocks))
	for _, block := range blocks {
		// Skip billing header messages
		if strings.HasPrefix(strings.TrimSpace(block.Text), "x-anthropic-billing-header:") {
			continue
		}
		result = append(result, block)
	}
	return result
}
