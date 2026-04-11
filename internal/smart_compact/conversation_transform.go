package smart_compact

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/sirupsen/logrus"

	"github.com/tingly-dev/tingly-box/internal/protocol"
)

// ConversationReplayStrategy compresses conversation history by reconstructing historical
// message blocks into a single assistant message.
//
// Each conversation turn is represented as:
// - A text block with "[User]" or "[Assistant]" role prefix
// - Followed by the original tool_use and tool_result blocks (for assistant turns)
//
// This preserves the structural information (tool calls, results) while fitting
// everything into a single assistant message that the model can reference.
//
// Beta API: produces assistant message with reconstructed blocks.
// V1 API:   falls back to XML text message (v1 lacks rich block types for this pattern).
type ConversationReplayStrategy struct {
	pathUtil *PathUtil
}

// NewConversationReplayStrategy creates a new replay-based strategy.
func NewConversationReplayStrategy() *ConversationReplayStrategy {
	return &ConversationReplayStrategy{
		pathUtil: NewPathUtil(),
	}
}

// Name returns the strategy identifier.
func (s *ConversationReplayStrategy) Name() string {
	return "conversation-replay"
}

// CompressV1 falls back to XML text message since v1 lacks the block variety needed.
func (s *ConversationReplayStrategy) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	xmlContent := buildConversationXML(messages, s.pathUtil)
	return []anthropic.MessageParam{
		{
			Role:    anthropic.MessageParamRoleAssistant,
			Content: []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(xmlContent)},
		},
	}
}

// CompressBeta reconstructs historical beta messages into a single assistant message
// with typed blocks preserving tool_use/tool_result structure.
func (s *ConversationReplayStrategy) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	var blocks []anthropic.BetaContentBlockParamUnion

	for _, msg := range messages {
		role := string(msg.Role)
		blocks = append(blocks, s.replayBetaMessage(role, msg.Content)...)
	}

	return []anthropic.BetaMessageParam{
		{
			Role:    anthropic.BetaMessageParamRoleAssistant,
			Content: blocks,
		},
	}
}

// replayBetaMessage converts a single message's blocks into replay blocks.
// Text content gets a role prefix; tool_use and tool_result blocks are preserved as-is.
func (s *ConversationReplayStrategy) replayBetaMessage(role string, content []anthropic.BetaContentBlockParamUnion) []anthropic.BetaContentBlockParamUnion {
	var blocks []anthropic.BetaContentBlockParamUnion

	roleLabel := "[User]"
	if role == "assistant" {
		roleLabel = "[Assistant]"
	}

	// Collect text from this message's text blocks
	var textParts []string
	for _, block := range content {
		if block.OfText != nil {
			textParts = append(textParts, block.OfText.Text)
		}
	}

	// Emit role-prefixed text block if there's any text
	if len(textParts) > 0 {
		combined := fmt.Sprintf("%s\n", roleLabel)
		for _, t := range textParts {
			combined += t + "\n"
		}
		blocks = append(blocks, anthropic.NewBetaTextBlock(combined))
	} else {
		// Emit role marker even if no text (e.g. pure tool_result message)
		blocks = append(blocks, anthropic.NewBetaTextBlock(fmt.Sprintf("%s\n", roleLabel)))
	}

	// Preserve tool_use blocks
	for _, block := range content {
		if block.OfToolUse != nil {
			blocks = append(blocks, block)
		}
	}

	// Preserve tool_result blocks
	for _, block := range content {
		if block.OfToolResult != nil {
			blocks = append(blocks, block)
		}
	}

	return blocks
}

// ConversationReplayTransformer applies replay-based compression.
//
// Same trigger conditions as other compact transformers:
// 1. Last user message contains "compact" (case-insensitive)
// 2. Request has tool definitions
type ConversationReplayTransformer struct {
	strategy *ConversationReplayStrategy
}

// NewConversationReplayTransformer creates a new replay transformer.
func NewConversationReplayTransformer() protocol.Transformer {
	return &ConversationReplayTransformer{
		strategy: NewConversationReplayStrategy(),
	}
}

// HandleV1 handles compacting for Anthropic v1 requests (XML fallback).
func (t *ConversationReplayTransformer) HandleV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !lastUserMessageContainsCompact(req.Messages) || len(req.Tools) == 0 {
		logrus.Debugf("[conversation-replay] v1: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[conversation-replay] v1: applying replay (XML fallback)")
	req.Messages = t.strategy.CompressV1(req.Messages)
	return nil
}

// HandleV1Beta handles compacting for Anthropic v1beta requests (native block replay).
func (t *ConversationReplayTransformer) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	if !lastUserMessageContainsCompactBeta(req.Messages) || len(req.Tools) == 0 {
		logrus.Debugf("[conversation-replay] v1beta: conditions not met, passing through")
		return nil
	}
	logrus.Infof("[conversation-replay] v1beta: applying block replay")
	req.Messages = t.strategy.CompressBeta(req.Messages)
	return nil
}
