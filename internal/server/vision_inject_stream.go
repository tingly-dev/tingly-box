package server

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/openai/openai-go/v3"

	"github.com/tingly-dev/tingly-box/internal/protocol"
	"github.com/tingly-dev/tingly-box/internal/protocol/wire"
	"github.com/tingly-dev/tingly-box/internal/server/processor"
)

// visionStreamInjectFactory is a protocol.DefaultStreamEventHookFactory:
// registered once at boot, it is consulted for every streaming response.
// When the request carries vision-proxy descriptions (stashed on the gin
// context by applyVisionProxy), it returns a hook that PREPENDS a single
// synthetic description event onto the wire just before the model's first
// text fragment.
//
// Prepend (rather than mutate the existing event) is what lets one hook
// serve every transport: Anthropic forwards upstream RawJSON verbatim, so
// mutating the typed event would be lost on the wire — but a brand-new,
// well-formed event we write ourselves is honoured by every path. The
// client accumulates deltas, so a leading description delta becomes
// leading text in the assembled assistant message, preserving the
// description into the next turn's history.
func visionStreamInjectFactory(hc *protocol.HandleContext) func(event interface{}) error {
	c := hc.GinContext
	raw, ok := c.Get(GinKeyVisionDescriptions)
	if !ok {
		return nil
	}
	descs, _ := raw.([]string)
	prefix := processor.BuildVisionDescriptionPrefix(descs)
	if prefix == "" {
		return nil
	}

	var injected bool
	return func(event interface{}) error {
		if injected {
			return nil
		}
		switch ev := event.(type) {
		case *openai.ChatCompletionChunk:
			if firstOpenAIText(ev) {
				writeSyntheticOpenAIChatText(hc, ev, prefix)
				injected = true
			}
		case *anthropic.MessageStreamEventUnion:
			if idx, ok := firstAnthropicTextIndex(ev.RawJSON()); ok {
				writeSyntheticAnthropicText(hc, idx, prefix)
				injected = true
			}
		case *anthropic.BetaRawMessageStreamEventUnion:
			if idx, ok := firstAnthropicTextIndex(ev.RawJSON()); ok {
				writeSyntheticAnthropicText(hc, idx, prefix)
				injected = true
			}
		case wire.ResponsesOutputTextDeltaEvent:
			// Converter-based paths (OpenAI Responses) deliver concrete
			// wire-event values rather than a single union. The text-delta
			// arm carries item_id / output_index / content_index, the
			// minimum framing needed to land a synthetic delta in the
			// same content part the model is about to fill.
			writeSyntheticResponsesText(hc, ev, prefix)
			injected = true
		}
		return nil
	}
}

// firstOpenAIText reports whether this chunk carries the first piece of
// assistant text (not a role-only preamble, not a tool_call chunk).
func firstOpenAIText(chunk *openai.ChatCompletionChunk) bool {
	if chunk == nil || len(chunk.Choices) == 0 {
		return false
	}
	return chunk.Choices[0].Delta.Content != ""
}

// writeSyntheticOpenAIChatText emits one extra chat.completion.chunk whose
// only delta is the description prefix, reusing the real chunk's id /
// created / model so the synthetic event is indistinguishable from a
// normal content chunk.
func writeSyntheticOpenAIChatText(hc *protocol.HandleContext, chunk *openai.ChatCompletionChunk, prefix string) {
	model := chunk.Model
	if model == "" {
		model = hc.ResponseModel
	}
	synthetic := map[string]interface{}{
		"id":      chunk.ID,
		"object":  "chat.completion.chunk",
		"created": chunk.Created,
		"model":   model,
		"choices": []map[string]interface{}{
			{
				"index": 0,
				"delta": map[string]interface{}{"content": prefix},
			},
		},
	}
	data, err := json.Marshal(synthetic)
	if err != nil {
		return
	}
	hc.GinContext.Writer.WriteString(fmt.Sprintf("data: %s\n\n", data))
}

// firstAnthropicTextIndex parses a raw Anthropic stream event and, if it is
// a content_block_delta carrying a text_delta, returns its block index.
// Detection uses raw JSON so it works for both v1 and beta unions without
// fighting the SDK's typed accessors.
func firstAnthropicTextIndex(rawJSON string) (int, bool) {
	if rawJSON == "" {
		return 0, false
	}
	var ev struct {
		Type  string `json:"type"`
		Index int    `json:"index"`
		Delta struct {
			Type string `json:"type"`
		} `json:"delta"`
	}
	if err := json.Unmarshal([]byte(rawJSON), &ev); err != nil {
		return 0, false
	}
	if ev.Type == "content_block_delta" && ev.Delta.Type == "text_delta" {
		return ev.Index, true
	}
	return 0, false
}

// writeSyntheticAnthropicText emits one extra content_block_delta(text_delta)
// at the same block index as the model's first text, so the prefix
// accumulates ahead of the real text inside the first text block.
func writeSyntheticAnthropicText(hc *protocol.HandleContext, index int, prefix string) {
	synthetic := map[string]interface{}{
		"type":  "content_block_delta",
		"index": index,
		"delta": map[string]interface{}{
			"type": "text_delta",
			"text": prefix,
		},
	}
	data, err := json.Marshal(synthetic)
	if err != nil {
		return
	}
	hc.GinContext.Writer.WriteString(fmt.Sprintf("event: content_block_delta\ndata: %s\n\n", data))
}

// writeSyntheticResponsesText emits one extra response.output_text.delta
// event that lands in the same content part the model's first text was
// about to fill. The triple (item_id, output_index, content_index) is
// what binds the delta to a specific content part; the sequence_number
// echoes the real event's so clients that expect monotonic numbering
// see a stable progression. The synthetic event runs BEFORE the model's
// real delta (hooks fire before handleFunc) so the prefix appears at
// the start of the assembled text.
func writeSyntheticResponsesText(hc *protocol.HandleContext, real wire.ResponsesOutputTextDeltaEvent, prefix string) {
	synthetic := wire.ResponsesOutputTextDeltaEvent{
		Type:           "response.output_text.delta",
		SequenceNumber: real.SequenceNumber,
		ItemID:         real.ItemID,
		OutputIndex:    real.OutputIndex,
		ContentIndex:   real.ContentIndex,
		Delta:          prefix,
	}
	data, err := json.Marshal(synthetic)
	if err != nil {
		return
	}
	hc.GinContext.Writer.WriteString(fmt.Sprintf("event: %s\ndata: %s\n\n", synthetic.Type, data))
}
