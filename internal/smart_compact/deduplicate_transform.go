package smart_compact

import (
	"encoding/json"
	"fmt"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

// Compile-time interface checks.
var _ transform.Transform = (*DeduplicationTransform)(nil)
var _ transform.Transform = (*PurgeErrorsTransform)(nil)

const (
	dedupPlaceholder      = "[Output removed to save context - information superseded or no longer needed]"
	purgeInputPlaceholder = `"[input removed due to failed tool call]"`
)

// DeduplicationTransform removes duplicate tool calls from messages.
// For each unique tool signature (name + params), only the most recent call's
// output is kept; earlier occurrences have their tool_result content replaced
// with a placeholder.
type DeduplicationTransform struct{}

// NewDeduplicationTransform creates a new DeduplicationTransform.
func NewDeduplicationTransform() transform.Transform {
	return &DeduplicationTransform{}
}

func (t *DeduplicationTransform) Name() string { return "deduplication" }

func (t *DeduplicationTransform) Apply(ctx *transform.TransformContext) error {
	req, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil
	}
	if len(req.Messages) == 0 {
		return nil
	}

	// Pass 1: collect tool_use blocks and their signatures, keyed by tool_call_id.
	type callEntry struct {
		sig string // "name::sortedJSON"
	}
	callsByID := map[string]*callEntry{}
	// Track the last call seen for each signature.
	lastBySignature := map[string]string{} // sig → last tool_call_id

	for _, msg := range req.Messages {
		for _, blk := range msg.Content {
			if blk.OfToolUse == nil {
				continue
			}
			id := blk.OfToolUse.ID
			sig := toolSignature(blk.OfToolUse.Name, blk.OfToolUse.Input)
			callsByID[id] = &callEntry{sig: sig}
			lastBySignature[sig] = id
		}
	}

	// Pass 2: for each tool_result, if its tool_call_id is NOT the latest for
	// its signature, replace the output with a placeholder.
	for i, msg := range req.Messages {
		for j, blk := range msg.Content {
			if blk.OfToolResult == nil {
				continue
			}
			id := blk.OfToolResult.ToolUseID
			entry, known := callsByID[id]
			if !known {
				continue
			}
			if lastBySignature[entry.sig] == id {
				continue // this is the latest — keep it
			}
			// Replace content with placeholder.
			req.Messages[i].Content[j].OfToolResult.Content = []anthropic.BetaToolResultBlockParamContentUnion{
				{OfText: &anthropic.BetaTextBlockParam{Text: dedupPlaceholder}},
			}
		}
	}

	return nil
}

// toolSignature builds a stable string key for a tool call: "name::sortedJSON".
// input is the BetaToolUseBlockParam.Input field (type any — may be []byte or
// an already-unmarshalled map). Sorting JSON keys ensures parameter order
// doesn't affect equality.
func toolSignature(name string, input any) string {
	var raw []byte
	switch v := input.(type) {
	case []byte:
		raw = v
	case json.RawMessage:
		raw = v
	default:
		// Already an interface value — re-marshal to get a canonical form.
		b, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%s::%v", name, v)
		}
		raw = b
	}

	var m interface{}
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Sprintf("%s::%s", name, string(raw))
	}
	sorted, err := json.Marshal(m)
	if err != nil {
		return fmt.Sprintf("%s::%s", name, string(raw))
	}
	return fmt.Sprintf("%s::%s", name, string(sorted))
}

// PurgeErrorsTransform removes the input of errored tool calls that are older
// than a configurable number of turns. The error output (tool_result content)
// is preserved so the LLM still has context about what went wrong.
//
// A "turn" here is counted by user-message boundaries. A value of 0 disables
// the transform entirely.
type PurgeErrorsTransform struct {
	gracePeriodTurns int
}

// NewPurgeErrorsTransform creates a PurgeErrorsTransform.
// gracePeriodTurns is the number of turns after which errored tool inputs are
// removed. 0 disables the transform.
func NewPurgeErrorsTransform(gracePeriodTurns int) transform.Transform {
	return &PurgeErrorsTransform{gracePeriodTurns: gracePeriodTurns}
}

func (t *PurgeErrorsTransform) Name() string { return "purge_errors" }

func (t *PurgeErrorsTransform) Apply(ctx *transform.TransformContext) error {
	if t.gracePeriodTurns == 0 {
		return nil
	}

	req, ok := ctx.Request.(*anthropic.BetaMessageNewParams)
	if !ok {
		return nil
	}
	if len(req.Messages) == 0 {
		return nil
	}

	// Assign a turn number to each message index.
	// A new turn starts every time we see a user message (after the first).
	turnOf := make([]int, len(req.Messages))
	currentTurn := 0
	for i, msg := range req.Messages {
		if string(msg.Role) == "user" && i > 0 {
			currentTurn++
		}
		turnOf[i] = currentTurn
	}
	totalTurns := currentTurn

	// Collect errored tool_call_ids and the turn they appeared in.
	erroredCallTurn := map[string]int{} // tool_call_id → turn
	for i, msg := range req.Messages {
		for _, blk := range msg.Content {
			if blk.OfToolResult == nil {
				continue
			}
			if blk.OfToolResult.IsError.Value {
				erroredCallTurn[blk.OfToolResult.ToolUseID] = turnOf[i]
			}
		}
	}

	// For each errored call that is old enough, replace the tool_use input.
	for i, msg := range req.Messages {
		for j, blk := range msg.Content {
			if blk.OfToolUse == nil {
				continue
			}
			errorTurn, isErrored := erroredCallTurn[blk.OfToolUse.ID]
			if !isErrored {
				continue
			}
			age := totalTurns - errorTurn
			if age < t.gracePeriodTurns {
				continue // still within grace period
			}
			req.Messages[i].Content[j].OfToolUse.Input = json.RawMessage(purgeInputPlaceholder)
		}
	}

	return nil
}
