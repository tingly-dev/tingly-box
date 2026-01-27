package server

import (
	"github.com/anthropics/anthropic-sdk-go"
)

// AnthropicStreamAssembler assembles Anthropic streaming responses for recording
type AnthropicStreamAssembler struct {
	recorder *ScenarioRecorder

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

// NewAnthropicStreamAssembler creates a new assembler for recording Anthropic streams
func NewAnthropicStreamAssembler(recorder *ScenarioRecorder) *AnthropicStreamAssembler {
	if recorder != nil {
		recorder.EnableStreaming()
	}
	return &AnthropicStreamAssembler{
		recorder: recorder,
		blocks:   make(map[int]anthropic.ContentBlockUnion),
	}
}

// RecordV1Event records a v1 stream event
func (a *AnthropicStreamAssembler) RecordV1Event(event *anthropic.MessageStreamEventUnion) {
	if a == nil || a.recorder == nil {
		return
	}

	// Record the raw chunk
	a.recorder.RecordStreamChunk(event.Type, event)

	// Assemble the final response
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

// RecordV1BetaEvent records a v1 beta stream event
func (a *AnthropicStreamAssembler) RecordV1BetaEvent(event *anthropic.BetaRawMessageStreamEventUnion) {
	if a == nil || a.recorder == nil {
		return
	}

	// Record the raw chunk
	a.recorder.RecordStreamChunk(event.Type, event)

	// Assemble the final response
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
		// For beta events, we can't directly assign BetaTextCitationUnion to TextCitationUnion
		// but they have the same JSON structure, so we skip citations for now
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

// SetUsage sets the usage data (for cases where usage is accumulated separately)
func (a *AnthropicStreamAssembler) SetUsage(inputTokens, outputTokens int) {
	a.usageData = &anthropic.Usage{
		InputTokens:  int64(inputTokens),
		OutputTokens: int64(outputTokens),
	}
}

// Finish assembles the final response and sets it on the recorder
func (a *AnthropicStreamAssembler) Finish(model string, inputTokens, outputTokens int) {
	if a == nil || a.recorder == nil || a.msgID == "" {
		return
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

	// Build the complete Anthropic Message response using string types
	type AssembledMessage struct {
		ID           string                        `json:"id"`
		Type         string                        `json:"type"`
		Role         string                        `json:"role"`
		Content      []anthropic.ContentBlockUnion `json:"content"`
		Model        string                        `json:"model"`
		StopReason   string                        `json:"stop_reason"`
		StopSequence string                        `json:"stop_sequence"`
		Usage        anthropic.Usage               `json:"usage"`
	}

	assembled := AssembledMessage{
		ID:           a.msgID,
		Type:         msgType,
		Role:         msgRole,
		Content:      content,
		Model:        model,
		StopReason:   stopReason,
		StopSequence: stopSeq,
		Usage:        *usage,
	}

	a.recorder.SetAssembledResponse(assembled)
}
