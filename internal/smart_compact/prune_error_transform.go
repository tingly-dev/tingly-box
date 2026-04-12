package smart_compact

import (
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/tingly-dev/tingly-box/internal/protocol/transform"
)

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
