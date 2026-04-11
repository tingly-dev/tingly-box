package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

const (
	documentTitle   = "Conversation History"
	documentContext = "Compacted conversation history for reference"
)

// ConversationDocumentStrategy compresses conversation history into an Anthropic document
// content block placed in a user message.
//
// Unlike ConditionalCompactStrategy (which produces an assistant text message), this
// strategy uses the Anthropic document block type, allowing the model to reference the
// conversation history as a cited document.
//
// Only activates when:
// 1. Last user message contains "compact" (case-insensitive)
// 2. Request has tool definitions
type ConversationDocumentStrategy struct {
	pathUtil *PathUtil
}

// NewConversationDocumentStrategy creates a new document-based compact strategy.
func NewConversationDocumentStrategy() *ConversationDocumentStrategy {
	return &ConversationDocumentStrategy{
		pathUtil: NewPathUtil(),
	}
}

// Name returns the strategy identifier.
func (s *ConversationDocumentStrategy) Name() string {
	return "conversation-document"
}

// CompressV1 compresses v1 messages into a user message with a document block.
func (s *ConversationDocumentStrategy) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	xmlContent := buildConversationXML(messages, s.pathUtil)

	doc := anthropic.DocumentBlockParam{
		Source: anthropic.DocumentBlockParamSourceUnion{
			OfText: &anthropic.PlainTextSourceParam{
				Data: xmlContent,
			},
		},
		Title:   anthropic.String(documentTitle),
		Context: anthropic.String(documentContext),
	}

	return []anthropic.MessageParam{
		{
			Role: anthropic.MessageParamRoleUser,
			Content: []anthropic.ContentBlockParamUnion{
				{OfDocument: &doc},
			},
		},
	}
}

// CompressBeta compresses beta messages into a user message with a document block.
func (s *ConversationDocumentStrategy) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	xmlContent := buildBetaConversationXML(messages, s.pathUtil)

	doc := anthropic.BetaRequestDocumentBlockParam{
		Source: anthropic.BetaRequestDocumentBlockSourceUnionParam{
			OfText: &anthropic.BetaPlainTextSourceParam{
				Data: xmlContent,
			},
		},
		Title:   anthropic.String(documentTitle),
		Context: anthropic.String(documentContext),
	}

	return []anthropic.BetaMessageParam{
		{
			Role: anthropic.BetaMessageParamRoleUser,
			Content: []anthropic.BetaContentBlockParamUnion{
				{OfDocument: &doc},
			},
		},
	}
}

// ConversationDocumentTransformer applies document-based compression using transform.Transform interface.
//
// Conditions for activation (same as ConditionalCompactTransformer):
// 1. Last user message contains "compact" (case-insensitive)
// 2. Request has tool definitions
type ConversationDocumentTransformer struct {
	strategy *ConversationDocumentStrategy
}

// Compile-time interface check.
var _ transform.Transform = (*ConversationDocumentTransformer)(nil)

// NewConversationDocumentTransformer creates a new document-based compact transformer.
func NewConversationDocumentTransformer() transform.Transform {
	return &ConversationDocumentTransformer{
		strategy: NewConversationDocumentStrategy(),
	}
}

// Name returns the transform identifier.
func (t *ConversationDocumentTransformer) Name() string {
	return "conversation-document"
}

// Apply applies the document-based compression to the request.
func (t *ConversationDocumentTransformer) Apply(ctx *transform.TransformContext) error {
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
func (t *ConversationDocumentTransformer) applyV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !t.shouldCompactV1(req) {
		logrus.Debugf("[conversation-document] v1: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[conversation-document] v1: applying document compression")
	req.Messages = t.strategy.CompressV1(req.Messages)
	return nil
}

// applyBeta handles compacting for Anthropic v1beta requests.
func (t *ConversationDocumentTransformer) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !t.shouldCompactV1Beta(req) {
		logrus.Debugf("[conversation-document] v1beta: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[conversation-document] v1beta: applying document compression")
	req.Messages = t.strategy.CompressBeta(req.Messages)
	return nil
}

func (t *ConversationDocumentTransformer) shouldCompactV1(req *anthropic.MessageNewParams) bool {
	if len(req.Tools) == 0 {
		return false
	}
	return lastUserMessageContainsCompact(req.Messages)
}

func (t *ConversationDocumentTransformer) shouldCompactV1Beta(req *anthropic.BetaMessageNewParams) bool {
	if len(req.Tools) == 0 {
		return false
	}
	return lastUserMessageContainsCompactBeta(req.Messages)
}
