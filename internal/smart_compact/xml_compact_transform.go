package smart_compact

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// XMLCompactTransform compresses conversation history into XML format.
//
// This is a pure compression strategy that always compresses the conversation
// into an XML-formatted assistant message. The conditional logic should be
// handled at the Virtual Model layer using ConditionalWrapper.
type XMLCompactTransform struct {
	pathUtil *PathUtil
}

// NewXMLCompactTransform creates a new XMLCompactTransform.
func NewXMLCompactTransform() transform.Transform {
	return &XMLCompactTransform{
		pathUtil: NewPathUtil(),
	}
}

// Name returns the transform identifier.
func (t *XMLCompactTransform) Name() string {
	return "xml_compact"
}

// Apply applies XML compression to the request.
func (t *XMLCompactTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return t.applyV1(req)
	case *anthropic.BetaMessageNewParams:
		return t.applyBeta(req)
	default:
		return nil
	}
}

// applyV1 handles compacting for Anthropic v1 requests.
func (t *XMLCompactTransform) applyV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}

	xmlSummary := buildConversationXML(req.Messages, t.pathUtil)
	xmlSummary = fmt.Sprintf("<analysis>\nCompact conversation into short\n</analysis>\n\n<summary>\n%s\n</summary>\n", xmlSummary)

	req.Messages = []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock("Here is the conversation summary:\n\n" + xmlSummary)},
		},
	}
	return nil
}

// applyBeta handles compacting for Anthropic v1beta requests.
func (t *XMLCompactTransform) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}

	xmlSummary := buildBetaConversationXML(req.Messages, t.pathUtil)
	xmlSummary = fmt.Sprintf("<analysis>\nCompact conversation into short</analysis>\n\n<summary>\n%s\n</summary>", xmlSummary)

	req.Messages = []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaTextBlock(xmlSummary)},
		},
	}
	return nil
}

// XMLCompactionStrategy compresses conversation history using Anthropic's native
// compaction block type (BetaCompactionBlockParam).
//
// Beta API: produces assistant message with compaction block.
// V1 API:   compaction block is unsupported; falls back to assistant text message with XML.
type XMLCompactionStrategy struct {
	pathUtil *PathUtil
}

// NewXMLCompactionStrategy creates a new compaction-based strategy.
func NewXMLCompactionStrategy() *XMLCompactionStrategy {
	return &XMLCompactionStrategy{
		pathUtil: NewPathUtil(),
	}
}

// CompressV1 falls back to XML text message since compaction block is beta-only.
func (s *XMLCompactionStrategy) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	xmlContent := buildConversationXML(messages, s.pathUtil)
	return []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(xmlContent)},
		},
	}
}

// CompressBeta compresses beta messages into a single assistant message with a compaction block.
func (s *XMLCompactionStrategy) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	xmlContent := buildBetaConversationXML(messages, s.pathUtil)
	return []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: []anthropic.BetaContentBlockParamUnion{anthropic.NewBetaCompactionBlock(xmlContent)},
		},
	}
}

// XMLCompactionTransformer applies compaction block compression using transform.Transform interface.
//
// Same trigger conditions as other compact transformers:
// 1. Last user message contains "compact" (case-insensitive)
// 2. Request has tool definitions
type XMLCompactionTransformer struct {
	strategy *XMLCompactionStrategy
}

// Compile-time interface check.
var _ transform.Transform = (*XMLCompactionTransformer)(nil)

// NewXMLCompactionTransformer creates a new compaction transformer.
func NewXMLCompactionTransformer() transform.Transform {
	return &XMLCompactionTransformer{
		strategy: NewXMLCompactionStrategy(),
	}
}

// Name returns the transform identifier.
func (t *XMLCompactionTransformer) Name() string {
	return "xml-compaction"
}

// Apply applies the compaction block compression to the request.
func (t *XMLCompactionTransformer) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return t.applyV1(req)
	case *anthropic.BetaMessageNewParams:
		return t.applyBeta(req)
	default:
		return nil
	}
}

// applyV1 handles compacting for Anthropic v1 requests (fallback to XML text).
func (t *XMLCompactionTransformer) applyV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !lastUserMessageContainsCompact(req.Messages) || len(req.Tools) == 0 {
		logrus.Debugf("[xml-compaction] v1: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[xml-compaction] v1: applying compaction (XML fallback)")
	req.Messages = t.strategy.CompressV1(req.Messages)
	return nil
}

// applyBeta handles compacting for Anthropic v1beta requests (native compaction block).
func (t *XMLCompactionTransformer) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !lastUserMessageContainsCompactBeta(req.Messages) || len(req.Tools) == 0 {
		logrus.Debugf("[xml-compaction] v1beta: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[xml-compaction] v1beta: applying native compaction block")
	req.Messages = t.strategy.CompressBeta(req.Messages)
	return nil
}
