package request

import (
	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"
)

// The cache-shape invariant
//
// A converted input item's serialized shape must depend only on its content —
// never on whether a prompt-cache breakpoint happens to sit on it this turn.
//
// Anthropic clients carry a small, fixed number of ephemeral breakpoints and
// roll them forward as the conversation grows, so a block that owned one on
// turn N usually does not own it on turn N+1. When the compact string form was
// used for "no breakpoint" and the content-part list form for "has breakpoint",
// that rotation silently rewrote history the upstream prompt cache had already
// been keyed on: the prefix matched up to the oldest moved breakpoint and
// missed from there on, burning the whole tail of a long conversation. The
// list form is therefore emitted unconditionally, and a breakpoint only ever
// adds a field to a part that would have been there anyway.
//
// See .design/protocol-responses.md §3.

// responsesInputTextPart builds an input_text content part, marking a
// prompt-cache breakpoint on it when the source block carried one. The part is
// byte-identical with and without the breakpoint apart from that one field.
func responsesInputTextPart(text string, breakpoint bool) *responses.ResponseInputTextParam {
	part := &responses.ResponseInputTextParam{Text: text}
	if breakpoint {
		part.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
	}
	return part
}

// responsesTextParts converts text blocks to input_text content parts, one per
// block, skipping empty ones. Used by the system-prefix path.
func responsesTextParts(blocks []anthropic.TextBlockParam) responses.ResponseInputMessageContentListParam {
	parts := make(responses.ResponseInputMessageContentListParam, 0, len(blocks))
	for _, block := range blocks {
		if block.Text == "" {
			continue
		}
		parts = append(parts, responses.ResponseInputContentUnionParam{
			OfInputText: responsesInputTextPart(block.Text, !param.IsOmitted(block.CacheControl)),
		})
	}
	return parts
}

// responsesOutputTextParts converts text blocks to output_text content parts,
// one per block, skipping empty ones. The Responses API requires
// assistant-authored content items to use output_text (or refusal); input_text
// is only valid on user/system input, so this is the assistant-message
// counterpart to responsesTextParts. Prompt-cache breakpoints have no
// output_text equivalent (ResponseOutputTextParam carries no such field), so
// block.CacheControl is ignored here.
func responsesOutputTextParts(blocks []anthropic.TextBlockParam) []responses.ResponseOutputMessageContentUnionParam {
	parts := make([]responses.ResponseOutputMessageContentUnionParam, 0, len(blocks))
	for _, block := range blocks {
		if block.Text == "" {
			continue
		}
		parts = append(parts, responses.ResponseOutputMessageContentUnionParam{
			OfOutputText: &responses.ResponseOutputTextParam{Text: block.Text},
		})
	}
	return parts
}

// applyFirstResponsesCacheBreakpoint carries an Anthropic cache boundary that
// Responses cannot attach directly (for example, on a tool definition or tool
// call). The boundary advances to the first cacheable content block.
//
// Every user/system message.OfMessage item this package builds is already the
// content-part array form (the cache-shape invariant, above), never the
// plain-string form, so the loop below only ever has to add a breakpoint to
// an existing part.
func applyFirstResponsesCacheBreakpoint(req *responses.ResponseNewParams) {
	if req.Instructions.Valid() && req.Instructions.Value != "" {
		text := &responses.ResponseInputTextParam{
			Text:                  req.Instructions.Value,
			PromptCacheBreakpoint: responses.NewResponseInputTextPromptCacheBreakpointParam(),
		}
		req.Instructions = param.Opt[string]{}
		system := responseMessageWithContent("system", responses.ResponseInputMessageContentListParam{
			{OfInputText: text},
		})
		req.Input.OfInputItemList = append(responses.ResponseInputParam{system}, req.Input.OfInputItemList...)
		return
	}

	for i := range req.Input.OfInputItemList {
		item := &req.Input.OfInputItemList[i]
		if item.OfMessage == nil {
			continue
		}
		for j := range item.OfMessage.Content.OfInputItemContentList {
			part := &item.OfMessage.Content.OfInputItemContentList[j]
			if part.OfInputText != nil {
				part.OfInputText.PromptCacheBreakpoint = responses.NewResponseInputTextPromptCacheBreakpointParam()
				return
			}
			if part.OfInputImage != nil {
				part.OfInputImage.PromptCacheBreakpoint = responses.NewResponseInputImagePromptCacheBreakpointParam()
				return
			}
		}
	}
}
