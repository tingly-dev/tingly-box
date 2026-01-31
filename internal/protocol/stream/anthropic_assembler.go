package stream

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/shared/constant"
)

// AnthropicStreamAssembler assembles Anthropic streaming responses
// It is a pure assembler that doesn't depend on recording logic
type AnthropicStreamAssembler struct {
	// Message-level data
	msgID      string
	msgType    string
	msgRole    string
	stopReason string
	stopSeq    string
	usageData  *anthropic.Usage

	// Block-level tracking - store ContentBlockUnion by index
	blocks map[int]anthropic.ContentBlockUnion
}

// NewAnthropicStreamAssembler creates a new assembler for Anthropic streams
func NewAnthropicStreamAssembler() *AnthropicStreamAssembler {
	return &AnthropicStreamAssembler{
		blocks: make(map[int]anthropic.ContentBlockUnion),
	}
}

// RecordV1Event processes a v1 stream event
func (a *AnthropicStreamAssembler) RecordV1Event(event *anthropic.MessageStreamEventUnion) {

	switch event.Type {
	case "message_start":
		a.msgID = string(event.Message.ID)
		a.msgType = string(event.Message.Type)
		a.msgRole = string(event.Message.Role)

	case "content_block_start":
		a.handleContentBlockStart(int(event.Index), event.ContentBlock)

	case "content_block_delta":
		a.handleContentBlockDelta(int(event.Index), event.Delta)

	case "message_delta":
		a.stopReason = string(event.Delta.StopReason)
		a.stopSeq = event.Delta.StopSequence
		if event.Usage.InputTokens > 0 || event.Usage.OutputTokens > 0 {
			a.usageData = &anthropic.Usage{
				InputTokens:  event.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			}
		}
	}
}

// RecordV1BetaEvent processes a v1 beta stream event
func (a *AnthropicStreamAssembler) RecordV1BetaEvent(event *anthropic.BetaRawMessageStreamEventUnion) {

	switch event.Type {
	case "message_start":
		a.msgID = string(event.Message.ID)
		a.msgType = string(event.Message.Type)
		a.msgRole = string(event.Message.Role)

	case "content_block_start":
		a.handleContentBlockStartBeta(int(event.Index), event.ContentBlock)

	case "content_block_delta":
		a.handleContentBlockDeltaBeta(int(event.Index), event.Delta)

	case "message_delta":
		a.stopReason = string(event.Delta.StopReason)
		a.stopSeq = event.Delta.StopSequence
		if event.Usage.InputTokens > 0 || event.Usage.OutputTokens > 0 {
			a.usageData = &anthropic.Usage{
				InputTokens:  event.Usage.InputTokens,
				OutputTokens: event.Usage.OutputTokens,
			}
		}
	}
}

// handleContentBlockStart handles content_block_start for v1 events
func (a *AnthropicStreamAssembler) handleContentBlockStart(blockIndex int, block anthropic.ContentBlockStartEventContentBlockUnion) {
	union := anthropic.ContentBlockUnion{
		Type: block.Type,
	}

	switch block.Type {
	case "text":
		union.Text = ""
		union.Citations = block.Citations
	case "thinking":
		union.Thinking = ""
		union.Signature = block.Signature
		if union.Signature == "" {
			union.Signature = "sig_" + a.msgID
		}
	}

	a.blocks[blockIndex] = union
}

// handleContentBlockStartBeta handles content_block_start for v1beta events
func (a *AnthropicStreamAssembler) handleContentBlockStartBeta(blockIndex int, block anthropic.BetaRawContentBlockStartEventContentBlockUnion) {
	union := anthropic.ContentBlockUnion{
		Type: block.Type,
	}

	switch block.Type {
	case "text":
		union.Text = ""
	case "thinking":
		union.Thinking = ""
		union.Signature = block.Signature
		if union.Signature == "" {
			union.Signature = "sig_" + a.msgID
		}
	}

	a.blocks[blockIndex] = union
}

// handleContentBlockDelta handles content_block_delta for v1 events
func (a *AnthropicStreamAssembler) handleContentBlockDelta(blockIndex int, delta anthropic.MessageStreamEventUnionDelta) {
	block, exists := a.blocks[blockIndex]
	if !exists {
		return
	}

	switch delta.Type {
	case "text_delta":
		block.Text += delta.Text
	case "thinking_delta":
		block.Thinking += delta.Thinking
	case "signature_delta":
		block.Signature = delta.Signature
	}

	a.blocks[blockIndex] = block
}

// handleContentBlockDeltaBeta handles content_block_delta for v1beta events
func (a *AnthropicStreamAssembler) handleContentBlockDeltaBeta(blockIndex int, delta anthropic.BetaRawMessageStreamEventUnionDelta) {
	block, exists := a.blocks[blockIndex]
	if !exists {
		return
	}

	switch delta.Type {
	case "text_delta":
		block.Text += delta.Text
	case "thinking_delta":
		block.Thinking += delta.Thinking
	case "signature_delta":
		block.Signature = delta.Signature
	}

	a.blocks[blockIndex] = block
}

// SetUsage sets the usage data
func (a *AnthropicStreamAssembler) SetUsage(inputTokens, outputTokens int) {
	a.usageData = &anthropic.Usage{
		InputTokens:  int64(inputTokens),
		OutputTokens: int64(outputTokens),
	}
}

// Finish assembles the final response and returns it as anthropic.Message
func (a *AnthropicStreamAssembler) Finish(model string, inputTokens, outputTokens int) *anthropic.Message {
	if a == nil || a.msgID == "" {
		return nil
	}

	// Set defaults
	msgType := a.msgType
	if msgType == "" {
		msgType = "message"
	}
	msgRole := a.msgRole
	if msgRole == "" {
		msgRole = "assistant"
	}
	stopReason := a.stopReason
	if stopReason == "" {
		stopReason = "end_turn"
	}
	stopSeq := a.stopSeq

	// Build usage
	usage := a.usageData
	if usage == nil {
		usage = &anthropic.Usage{
			InputTokens:  int64(inputTokens),
			OutputTokens: int64(outputTokens),
		}
	}

	// Collect blocks in order
	content := make([]anthropic.ContentBlockUnion, 0, len(a.blocks))
	for i := 0; i < len(a.blocks); i++ {
		if block, exists := a.blocks[i]; exists {
			content = append(content, block)
		}
	}

	return &anthropic.Message{
		ID:           a.msgID,
		Type:         "message",
		Role:         constant.Assistant(msgRole),
		Content:      content,
		Model:        anthropic.Model(model),
		StopReason:   anthropic.StopReason(stopReason),
		StopSequence: stopSeq,
		Usage:        *usage,
	}
}
