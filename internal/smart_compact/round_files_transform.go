package smart_compact

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// NewRoundFilesTransformer creates a protocol.Transformer for round-files compression.
func NewRoundFilesTransformer() protocol.Transformer {
	t := NewRoundWithFilesStrategy()
	return &roundFilesTransformerAdapter{t}
}

type roundFilesTransformerAdapter struct{ t *RoundFilesTransform }

func (a *roundFilesTransformerAdapter) HandleV1(req *anthropic.MessageNewParams) error {
	return a.t.applyV1(req)
}

func (a *roundFilesTransformerAdapter) HandleV1Beta(req *anthropic.BetaMessageNewParams) error {
	return a.t.applyBeta(req)
}

// RoundFilesTransform keeps user/assistant + file paths as virtual tool calls.
// This removes tool_use/tool_result blocks but injects virtual tool calls
// summarizing which files were used in each round.
type RoundFilesTransform struct {
	rounder  *protocol.Grouper
	pathUtil *PathUtil
}

// NewRoundFilesTransform creates a new RoundFilesTransform.
func NewRoundFilesTransform() transform.Transform {
	return &RoundFilesTransform{
		rounder:  protocol.NewGrouper(),
		pathUtil: NewPathUtil(),
	}
}

// NewRoundWithFilesStrategy creates a RoundFilesTransform (previously a separate Strategy type).
func NewRoundWithFilesStrategy() *RoundFilesTransform {
	return &RoundFilesTransform{
		rounder:  protocol.NewGrouper(),
		pathUtil: NewPathUtil(),
	}
}

// Name returns the transform identifier.
func (t *RoundFilesTransform) Name() string {
	return "round_files"
}

// CompressV1 compresses v1 messages keeping user/assistant + virtual file tool calls.
func (t *RoundFilesTransform) CompressV1(messages []anthropic.MessageParam) []anthropic.MessageParam {
	rounds := t.rounder.GroupV1(messages)
	if len(rounds) == 0 {
		return messages
	}

	var result []anthropic.MessageParam

	for roundIdx, round := range rounds {
		isCurrent := (roundIdx == len(rounds)-1)

		if isCurrent {
			result = append(result, round.Messages...)
			continue
		}

		result = append(result, t.compressRoundWithVirtualTools(round)...)
	}

	return result
}

// CompressBeta compresses beta messages keeping user/assistant + virtual file tool calls.
func (t *RoundFilesTransform) CompressBeta(messages []anthropic.BetaMessageParam) []anthropic.BetaMessageParam {
	rounds := t.rounder.GroupBeta(messages)
	if len(rounds) == 0 {
		return messages
	}

	var result []anthropic.BetaMessageParam

	for roundIdx, round := range rounds {
		isCurrent := (roundIdx == len(rounds)-1)

		if isCurrent {
			result = append(result, round.Messages...)
			continue
		}

		result = append(result, t.compressBetaRoundWithVirtualTools(round)...)
	}

	return result
}

// Apply applies the round-files compression to the request.
func (t *RoundFilesTransform) Apply(ctx *transform.TransformContext) error {
	switch req := ctx.Request.(type) {
	case *anthropic.MessageNewParams:
		return t.applyV1(req)
	case *anthropic.BetaMessageNewParams:
		return t.applyBeta(req)
	default:
		return nil
	}
}

// applyV1 applies round-files compression to v1 requests.
func (t *RoundFilesTransform) applyV1(req *anthropic.MessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	req.Messages = t.CompressV1(req.Messages)
	return nil
}

// compressRoundWithVirtualTools compresses a historical round and injects virtual tool calls.
func (t *RoundFilesTransform) compressRoundWithVirtualTools(round protocol.V1Round) []anthropic.MessageParam {
	var result []anthropic.MessageParam
	var collectedFiles []string
	var injectedVirtualTools bool

	// First pass: collect files
	for _, msg := range round.Messages {
		role := string(msg.Role)
		if role == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := t.pathUtil.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Second pass: build compressed messages
	for _, msg := range round.Messages {
		role := string(msg.Role)

		if role == "user" && t.isPureUserMessage(msg) {
			// Pure user message: keep only text
			compressed := t.keepOnlyText(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		} else if role == "assistant" {
			// Assistant message: keep only text
			compressed := t.keepOnlyText(msg)
			hasText := len(compressed.Content) > 0
			if hasText {
				result = append(result, compressed)
			}

			// Inject virtual tools after the first assistant text (or after first assistant message even if empty)
			if !injectedVirtualTools && len(collectedFiles) > 0 {
				virtualTools := CreateVirtualToolCalls(collectedFiles)
				if len(virtualTools) > 0 {
					// Add virtual assistant message
					result = append(result, virtualTools[0].(anthropic.MessageParam))
					// Add virtual user message
					result = append(result, virtualTools[1].(anthropic.MessageParam))
				}
				injectedVirtualTools = true
			}
		}
	}

	return result
}

// isPureUserMessage checks if message is a pure user message (not tool_result).
func (t *RoundFilesTransform) isPureUserMessage(msg anthropic.MessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false
		}
	}
	return true
}

// keepOnlyText keeps only text blocks from a message.
func (t *RoundFilesTransform) keepOnlyText(msg anthropic.MessageParam) anthropic.MessageParam {
	var filtered []anthropic.ContentBlockParamUnion
	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}
	msg.Content = filtered
	return msg
}

// applyBeta applies round-files compression to beta requests.
func (t *RoundFilesTransform) applyBeta(req *anthropic.BetaMessageNewParams) error {
	if len(req.Messages) == 0 {
		return nil
	}
	req.Messages = t.CompressBeta(req.Messages)
	return nil
}

func (t *RoundFilesTransform) compressBetaRoundWithVirtualTools(round protocol.BetaRound) []anthropic.BetaMessageParam {
	var result []anthropic.BetaMessageParam
	var collectedFiles []string
	var injectedVirtualTools bool

	// First pass: collect files
	for _, msg := range round.Messages {
		role := string(msg.Role)
		if role == "assistant" {
			for _, block := range msg.Content {
				if block.OfToolUse != nil {
					if inputMap, ok := block.OfToolUse.Input.(map[string]any); ok {
						files := t.pathUtil.ExtractFromMap(inputMap)
						collectedFiles = append(collectedFiles, files...)
					}
				}
			}
		}
	}

	// Deduplicate files
	collectedFiles = deduplicate(collectedFiles)

	// Second pass: build compressed messages
	for _, msg := range round.Messages {
		role := string(msg.Role)

		if role == "user" && t.isPureBetaUserMessage(msg) {
			compressed := t.keepOnlyBetaText(msg)
			if len(compressed.Content) > 0 {
				result = append(result, compressed)
			}
		} else if role == "assistant" {
			compressed := t.keepOnlyBetaText(msg)
			hasText := len(compressed.Content) > 0
			if hasText {
				result = append(result, compressed)
			}

			if !injectedVirtualTools && len(collectedFiles) > 0 {
				virtualTools := CreateBetaVirtualToolCalls(collectedFiles)
				if len(virtualTools) > 0 {
					result = append(result, virtualTools[0].(anthropic.BetaMessageParam))
					result = append(result, virtualTools[1].(anthropic.BetaMessageParam))
				}
				injectedVirtualTools = true
			}
		}
	}

	return result
}

func (t *RoundFilesTransform) isPureBetaUserMessage(msg anthropic.BetaMessageParam) bool {
	if string(msg.Role) != "user" {
		return false
	}
	for _, block := range msg.Content {
		if block.OfToolResult != nil {
			return false
		}
	}
	return true
}

func (t *RoundFilesTransform) keepOnlyBetaText(msg anthropic.BetaMessageParam) anthropic.BetaMessageParam {
	var filtered []anthropic.BetaContentBlockParamUnion
	for _, block := range msg.Content {
		if block.OfText != nil {
			filtered = append(filtered, block)
		}
	}
	msg.Content = filtered
	return msg
}
